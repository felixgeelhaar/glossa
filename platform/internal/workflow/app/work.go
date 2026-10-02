package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/domain"
)

// WorkService implements assignments and approvals (RFC 0006 §3.1–3.2):
// by hand, for the API (wave 3), and as the effects of the assign and
// request_approval actions, for the instance runner.
//
// The runner calls AssignForInstance, RequestApprovalForInstance and
// Approvers with the triggering actor's context, inside its step's
// transaction: WorkTransactor joins it, so an action's assignment or
// approval commits — and its events are published — with the transition
// that asked for it, or not at all.
type WorkService struct {
	tx      WorkTransactor
	dir     Directory
	authors Authors
	// releases says where a release request is and who asked for it
	// (RFC 0006 §5.1); nil refuses every decision on one.
	releases ReleaseRequests
	now      func() time.Time
}

// WorkOption configures a WorkService.
type WorkOption func(*WorkService)

// WithWorkClock sets the time source.
func WithWorkClock(now func() time.Time) WorkOption { return func(s *WorkService) { s.now = now } }

// WithReleaseRequests lets the service decide approvals of release
// requests: Release says which environment a request is for — where
// approvals.decide is checked — and who requested it, whom four-eyes
// is held against.
func WithReleaseRequests(r ReleaseRequests) WorkOption {
	return func(s *WorkService) { s.releases = r }
}

// NewWorkService returns a WorkService.
func NewWorkService(tx WorkTransactor, dir Directory, authors Authors, opts ...WorkOption) *WorkService {
	s := &WorkService{tx: tx, dir: dir, authors: authors, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ── assignments ─────────────────────────────────────────────────────

// AssignInput is an assignment made by hand.
type AssignInput struct {
	ProjectID uuid.UUID
	Units     []domain.Unit
	To        domain.Party
	// Permission is what doing the work takes; translations.write when
	// empty.
	Permission string
	DueAt      *time.Time
}

// Assign gives units to a party. It takes assignments.manage.
func (s *WorkService) Assign(ctx context.Context, in AssignInput) (domain.Assignment, error) {
	if err := authz.RequireIn(ctx, PermAssignmentsManage, in.ProjectID); err != nil {
		return domain.Assignment{}, err
	}
	perm := in.Permission
	if perm == "" {
		perm = string(authz.TranslationsWrite)
	}
	return s.assign(ctx, uuid.Nil, in.ProjectID, in.Units, in.To, perm, in.DueAt)
}

// WorkflowAssign is an assign action's effect: the units of the
// instance's subject and the action's parameters.
type WorkflowAssign struct {
	InstanceID uuid.UUID
	ProjectID  uuid.UUID
	Subject    domain.SubjectKind
	Units      []domain.Unit
	Params     domain.Assign
}

// AssignForInstance carries out an assign action as the actor whose
// event moved the instance. The authority to assign is the definition
// a workflow manager saved and bound, so it takes no permission of the
// actor beyond being one in this tenant; the actor is who the
// assignment and its event name. The work it assigns is translating
// (translations.write).
func (s *WorkService) AssignForInstance(ctx context.Context, in WorkflowAssign) (domain.Assignment, error) {
	if _, err := actorInTenant(ctx); err != nil {
		return domain.Assignment{}, err
	}
	if err := authz.InProject(ctx, in.ProjectID); err != nil {
		return domain.Assignment{}, err
	}
	if in.Subject != domain.SubjectTranslation {
		return domain.Assignment{}, ErrUnsupportedSubject
	}
	if in.InstanceID == uuid.Nil {
		return domain.Assignment{}, fmt.Errorf("%w: no instance", domain.ErrInvalidAssignment)
	}
	var due *time.Time
	if !in.Params.Due.IsZero() {
		t := s.now().UTC().Add(in.Params.Due.Std())
		due = &t
	}
	return s.assign(ctx, in.InstanceID, in.ProjectID, in.Units, in.Params.To, string(authz.TranslationsWrite), due)
}

func (s *WorkService) assign(ctx context.Context, instance, project uuid.UUID, units []domain.Unit, to domain.Party, perm string, due *time.Time) (domain.Assignment, error) {
	actor, err := authz.EventActor(ctx)
	if err != nil {
		return domain.Assignment{}, err
	}
	var out domain.Assignment
	err = s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) error {
		assignee, err := s.resolve(ctx, to)
		if err != nil {
			return err
		}
		a, err := domain.NewAssignment(project, units, assignee, perm, due, actor.String(), s.now().UTC())
		if err != nil {
			return err
		}
		a.InstanceID = instance
		if err := st.InsertAssignment(ctx, a); err != nil {
			return err
		}
		out = a
		return st.Publish(ctx, assignmentEvent(domain.EventTypeAssignmentCreated, a, actor))
	})
	return out, err
}

// resolve turns a definition's or a request's party into Identity's
// ids.
func (s *WorkService) resolve(ctx context.Context, p domain.Party) (domain.Assignee, error) {
	var (
		kind domain.AssigneeKind
		ref  string
		n    int
	)
	for k, v := range map[domain.AssigneeKind]string{
		domain.AssigneeMember: p.Member, domain.AssigneeRole: p.Role,
		domain.AssigneeGroup: p.Group, domain.AssigneeVendor: p.Vendor,
	} {
		if v != "" {
			kind, ref, n = k, v, n+1
		}
	}
	if n != 1 {
		return domain.Assignee{}, fmt.Errorf("%w: name exactly one member, role, group or vendor", domain.ErrInvalidAssignment)
	}
	if kind == domain.AssigneeRole {
		a := domain.RoleAssignee(ref)
		return a, a.Validate()
	}
	id, err := s.dir.Resolve(ctx, kind, ref)
	if err != nil {
		return domain.Assignee{}, err
	}
	return domain.Assignee{Kind: kind, ID: id}, nil
}

// Accept takes an open assignment on. Only its assignee can, holding
// the assignment's permission for every unit's locale.
func (s *WorkService) Accept(ctx context.Context, id uuid.UUID) (domain.Assignment, error) {
	return s.work(ctx, id, domain.EventTypeAssignmentAccepted, func(a *domain.Assignment, _ string, now time.Time) error {
		return a.Accept(now)
	})
}

// Complete claims an assignment done, raising assignment.completed. It
// changes no review state: the definition decides what follows. Only
// its assignee can, holding the assignment's permission for every
// unit's locale.
func (s *WorkService) Complete(ctx context.Context, id uuid.UUID) (domain.Assignment, error) {
	return s.work(ctx, id, domain.EventTypeAssignmentCompleted, func(a *domain.Assignment, by string, now time.Time) error {
		return a.Complete(by, now)
	})
}

// Decline hands an assignment back, raising assignment.declined: by
// its assignee, or by someone holding assignments.manage taking it
// back.
func (s *WorkService) Decline(ctx context.Context, id uuid.UUID, reason string) (domain.Assignment, error) {
	if authz.Require(ctx, PermAssignmentsManage) == nil {
		return s.change(ctx, id, domain.EventTypeAssignmentDeclined, inScope, func(a *domain.Assignment, by string, now time.Time) error {
			return a.Decline(by, reason, now)
		})
	}
	return s.work(ctx, id, domain.EventTypeAssignmentDeclined, func(a *domain.Assignment, by string, now time.Time) error {
		return a.Decline(by, reason, now)
	})
}

// Expire ends a live assignment past its due date. It takes
// assignments.manage: the timer sweep's background principal holds it.
func (s *WorkService) Expire(ctx context.Context, id uuid.UUID) (domain.Assignment, error) {
	if err := authz.Require(ctx, PermAssignmentsManage); err != nil {
		return domain.Assignment{}, err
	}
	return s.change(ctx, id, domain.EventTypeAssignmentExpired, inScope, func(a *domain.Assignment, by string, now time.Time) error {
		return a.Expire(by, now)
	})
}

// inScope is the check on an assignment a manager changes: one outside
// their project scope doesn't exist to them (RFC 0006 §4.1).
func inScope(ctx context.Context, a domain.Assignment) error {
	return authz.InProject(ctx, a.ProjectID)
}

// ownWork checks perm for reading the caller's own work. An `assigned`
// member (a vendor's) holds no tenant-wide permission, so authz.Require
// refuses them everything; their own assignments are the one thing that
// is theirs without any unit to name, so for them the grant decides and
// the affiliation filter that follows keeps it to their own rows.
func ownWork(ctx context.Context, perm authz.Permission) error {
	p, ok := authz.From(ctx)
	if !ok {
		return authz.ErrUnauthenticated
	}
	if p.Assigned() {
		if !p.Grant.Allows(perm) {
			return &authz.DeniedError{Permission: perm}
		}
		return nil
	}
	return authz.Require(ctx, perm)
}

// work is a change only the assignee may make, holding the
// assignment's permission for every unit's locale.
func (s *WorkService) work(ctx context.Context, id uuid.UUID, event string, fn func(*domain.Assignment, string, time.Time) error) (domain.Assignment, error) {
	p, err := actorInTenant(ctx)
	if err != nil {
		return domain.Assignment{}, err
	}
	check := func(ctx context.Context, a domain.Assignment) error {
		if p.Member.IsZero() {
			return ErrNotAssignee
		}
		aff, err := s.dir.Affiliation(ctx, p.Member.UUID())
		if errors.Is(err, ErrNotFound) {
			return ErrNotAssignee
		} else if err != nil {
			return err
		}
		if !aff.Includes(a.Assignee) {
			return ErrNotAssignee
		}
		// Per unit and through RequireUnit, not RequireFor per locale: a
		// vendor member (visibility `assigned`) holds the permission only
		// for units an assignment of theirs covers, which this one does,
		// and a project-scoped member only inside their projects.
		for _, u := range a.Units {
			locale, err := authz.ParseLocale(u.Locale)
			if err != nil {
				return err
			}
			if err := authz.RequireUnit(ctx, authz.Permission(a.Permission), a.ProjectID, u.Message, locale); err != nil {
				return err
			}
		}
		return nil
	}
	return s.change(ctx, id, event, check, fn)
}

func (s *WorkService) change(ctx context.Context, id uuid.UUID, event string, check func(context.Context, domain.Assignment) error, fn func(*domain.Assignment, string, time.Time) error) (domain.Assignment, error) {
	actor, err := authz.EventActor(ctx)
	if err != nil {
		return domain.Assignment{}, err
	}
	var out domain.Assignment
	err = s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) error {
		a, err := st.LockAssignment(ctx, id)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(ctx, a); err != nil {
				return err
			}
		}
		if err := fn(&a, actor.String(), s.now().UTC()); err != nil {
			return err
		}
		if err := st.UpdateAssignment(ctx, a); err != nil {
			return err
		}
		out = a
		return st.Publish(ctx, assignmentEvent(event, a, actor))
	})
	return out, err
}

// Assignment reads one assignment: with assignments.manage, or as its
// assignee with assignments.read.
func (s *WorkService) Assignment(ctx context.Context, id uuid.UUID) (domain.Assignment, error) {
	manager := authz.Require(ctx, PermAssignmentsManage) == nil
	if !manager {
		if err := ownWork(ctx, PermAssignmentsRead); err != nil {
			return domain.Assignment{}, err
		}
	}
	p, _ := authz.From(ctx)
	var out domain.Assignment
	err := s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) error {
		a, err := st.GetAssignment(ctx, id)
		if err != nil {
			return err
		}
		if err := authz.InProject(ctx, a.ProjectID); err != nil {
			return err
		}
		if !manager {
			aff, err := s.affiliation(ctx, p)
			if err != nil {
				return err
			}
			// Someone else's assignment does not exist to them.
			if !aff.Includes(a.Assignee) {
				return ErrNotFound
			}
		}
		out = a
		return nil
	})
	return out, err
}

// Assignments lists a tenant's assignments. It takes
// assignments.manage; people see their own with MyAssignments.
func (s *WorkService) Assignments(ctx context.Context, f AssignmentFilter) ([]domain.Assignment, error) {
	scope, err := authz.Projects(ctx, PermAssignmentsManage)
	if err != nil {
		return nil, err
	}
	f.Within = within(scope)
	var out []domain.Assignment
	err = s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) (err error) {
		out, err = st.ListAssignments(ctx, f)
		return err
	})
	return out, err
}

// MyAssignments lists the assignments given to the caller — directly,
// by a role they hold, through a group or through their vendor — what
// Studio's "My work" shows. f's Assignees is replaced by the caller's.
func (s *WorkService) MyAssignments(ctx context.Context, f AssignmentFilter) ([]domain.Assignment, error) {
	if err := ownWork(ctx, PermAssignmentsRead); err != nil {
		return nil, err
	}
	p, _ := authz.From(ctx)
	f.Within = nil
	if !p.Projects.All() {
		ids := p.Projects.UUIDs()
		f.Within = &ids
	}
	var out []domain.Assignment
	err := s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) error {
		aff, err := s.affiliation(ctx, p)
		if err != nil {
			return err
		}
		if aff.Member == uuid.Nil {
			return nil
		}
		f.Assignees = aff.workKeys()
		out, err = st.ListAssignments(ctx, f)
		return err
	})
	return out, err
}

// affiliation is the caller's; a token or an unknown member has none.
func (s *WorkService) affiliation(ctx context.Context, p authz.Principal) (Affiliation, error) {
	if p.Member.IsZero() {
		return Affiliation{}, nil
	}
	aff, err := s.dir.Affiliation(ctx, p.Member.UUID())
	if errors.Is(err, ErrNotFound) {
		return Affiliation{}, nil
	}
	return aff, err
}

// ── approvals ───────────────────────────────────────────────────────

// ApprovalInput is an approval requested by hand.
type ApprovalInput struct {
	ProjectID uuid.UUID
	Subject   domain.ApprovalSubject
	N         int
	From      domain.Party
	DueAt     *time.Time
}

// RequestApproval asks for n approvals of a subject. It takes
// assignments.manage. Four-eyes always applies.
func (s *WorkService) RequestApproval(ctx context.Context, in ApprovalInput) (domain.Approval, error) {
	if err := authz.RequireIn(ctx, PermAssignmentsManage, in.ProjectID); err != nil {
		return domain.Approval{}, err
	}
	return s.requestApproval(ctx, uuid.Nil, in.ProjectID, in.Subject, in.N, in.From, in.DueAt)
}

// WorkflowApproval is a request_approval action's effect.
type WorkflowApproval struct {
	InstanceID uuid.UUID
	ProjectID  uuid.UUID
	Subject    domain.ApprovalSubject
	Params     domain.RequestApproval
}

// RequestApprovalForInstance carries out a request_approval action as
// the actor whose event moved the instance; like AssignForInstance it
// takes no permission of theirs. Four-eyes (distinct_from_author)
// always applies to the approval itself; the definition's
// approvals_at_least guard applies its own setting to its count. A
// pending approval for the same subject asking the same of the same
// party is returned as it is rather than asked twice.
func (s *WorkService) RequestApprovalForInstance(ctx context.Context, in WorkflowApproval) (domain.Approval, error) {
	if _, err := actorInTenant(ctx); err != nil {
		return domain.Approval{}, err
	}
	if err := authz.InProject(ctx, in.ProjectID); err != nil {
		return domain.Approval{}, err
	}
	if in.InstanceID == uuid.Nil {
		return domain.Approval{}, fmt.Errorf("%w: no instance", domain.ErrInvalidApproval)
	}
	var due *time.Time
	if !in.Params.Due.IsZero() {
		t := s.now().UTC().Add(in.Params.Due.Std())
		due = &t
	}
	return s.requestApproval(ctx, in.InstanceID, in.ProjectID, in.Subject, in.Params.N, in.Params.From, due)
}

func (s *WorkService) requestApproval(ctx context.Context, instance, project uuid.UUID, subject domain.ApprovalSubject, n int, from domain.Party, due *time.Time) (domain.Approval, error) {
	actor, err := authz.EventActor(ctx)
	if err != nil {
		return domain.Approval{}, err
	}
	if from.Vendor != "" {
		return domain.Approval{}, fmt.Errorf("%w: a vendor cannot be asked to approve", domain.ErrInvalidApproval)
	}
	var out domain.Approval
	err = s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) error {
		eligible, err := s.resolve(ctx, from)
		if err != nil {
			return err
		}
		a, err := domain.NewApproval(project, subject, n, eligible, true, due, actor.String(), s.now().UTC())
		if err != nil {
			return err
		}
		a.InstanceID = instance
		prev, err := st.LatestApproval(ctx, project, a.Subject)
		switch {
		case err == nil && prev.State == domain.ApprovalPending && prev.Required == a.Required &&
			prev.Eligible == a.Eligible && prev.InstanceID == a.InstanceID:
			out = prev
			return nil
		case err != nil && !errors.Is(err, ErrNotFound):
			return err
		}
		if err := st.InsertApproval(ctx, a); err != nil {
			return err
		}
		out = a
		return st.Publish(ctx, approvalEvent(domain.EventTypeApprovalRequested, a, nil, "", actor))
	})
	return out, err
}

// Decide records the caller's decision on an approval and raises
// approval.granted or approval.denied.
//
// Deciding is human-only (approvals.decide, §3.2): a token, an MCP
// agent or a background process is refused before anything else is
// looked at. The caller must hold approvals.decide (for a translation,
// in its locale), be in the approval's eligible party, and — four-eyes
// — not have written the text under approval. A repeated grant by the
// same person records nothing. Only the newest approval of a subject
// takes decisions.
func (s *WorkService) Decide(ctx context.Context, id uuid.UUID, verdict domain.Verdict, reason string) (domain.Approval, error) {
	p, err := actorInTenant(ctx)
	if err != nil {
		return domain.Approval{}, err
	}
	if PermApprovalsDecide.HumanOnly() && (p.Person.IsZero() || p.Member.IsZero()) {
		return domain.Approval{}, fmt.Errorf("%w: %w", authz.ErrForbidden, domain.ErrNotHuman)
	}
	actor, err := authz.EventActor(ctx)
	if err != nil {
		return domain.Approval{}, err
	}
	// Who wrote the text is another context's to say, read before this
	// transaction so that its port runs in its own.
	var subject domain.Approval
	if err := s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) (err error) {
		subject, err = st.GetApproval(ctx, id)
		return err
	}); err != nil {
		return domain.Approval{}, err
	}
	author, err := s.decidable(ctx, subject.ProjectID, subject.Subject)
	if err != nil {
		return domain.Approval{}, err
	}

	var out domain.Approval
	err = s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) error {
		a, err := st.LockApproval(ctx, id)
		if err != nil {
			return err
		}
		if latest, err := st.LatestApproval(ctx, a.ProjectID, a.Subject); err != nil {
			return err
		} else if latest.ID != a.ID {
			return ErrSuperseded
		}
		aff, err := s.dir.Affiliation(ctx, p.Member.UUID())
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		eligible := err == nil && aff.Includes(a.Eligible)
		recorded, err := a.Decide(domain.Ballot{
			Principal: actor.String(), Verdict: verdict, Reason: reason,
			Eligible: eligible, Author: author, At: s.now().UTC(),
		})
		if err != nil {
			return refusal(err)
		}
		out = a
		if !recorded {
			return nil
		}
		d := a.Decisions[len(a.Decisions)-1]
		if err := st.AppendDecision(ctx, a.ID, len(a.Decisions), d); err != nil {
			return err
		}
		if err := st.UpdateApproval(ctx, a); err != nil {
			return err
		}
		event := domain.EventTypeApprovalGranted
		if d.Verdict == domain.VerdictDenied {
			event = domain.EventTypeApprovalDenied
		}
		return st.Publish(ctx, approvalEvent(event, a, &d, author, actor))
	})
	return out, err
}

// decidable checks that the caller may decide an approval of s, and
// returns the author four-eyes is held against.
//
// approvals.decide is checked where the subject is (RFC 0006 §4.2): in
// a translation's locale, and in a release request's environment —
// environment-scoped, so a reviewer limited to de may decide a release
// into production unless their environment scope leaves it out. The
// author of a translation is the actor of its latest content revision;
// of a release request, its requester. Both are another context's to
// say, read before the decision's transaction so their ports run in
// their own.
func (s *WorkService) decidable(ctx context.Context, project uuid.UUID, subject domain.ApprovalSubject) (string, error) {
	if subject.Kind == domain.SubjectReleaseRequest {
		if s.releases == nil {
			return "", fmt.Errorf("%w: release requests are not wired in this deployment", authz.ErrForbidden)
		}
		f, err := s.releases.Request(ctx, project, subject.ID)
		if err != nil {
			return "", err
		}
		if err := authz.RequireInEnvironment(ctx, PermApprovalsDecide, project, f.Environment); err != nil {
			return "", err
		}
		return f.Requester, nil
	}
	locale, err := authz.ParseLocale(subject.Locale)
	if err != nil {
		return "", err
	}
	if err := authz.RequireForIn(ctx, PermApprovalsDecide, locale, project); err != nil {
		return "", err
	}
	return s.authors.Author(ctx, project, subject)
}

// refusal makes the domain's refusals of who is deciding match
// authz.ErrForbidden as well, so every adapter answers them as one.
func refusal(err error) error {
	for _, e := range []error{domain.ErrNotHuman, domain.ErrNotEligible, domain.ErrOwnText} {
		if errors.Is(err, e) {
			return fmt.Errorf("%w: %w", authz.ErrForbidden, err)
		}
	}
	return err
}

// Approval reads one approval with its decisions. It takes
// workflows.read.
func (s *WorkService) Approval(ctx context.Context, id uuid.UUID) (domain.Approval, error) {
	if err := authz.Require(ctx, PermWorkflowsRead); err != nil {
		return domain.Approval{}, err
	}
	var out domain.Approval
	err := s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) (err error) {
		if out, err = st.GetApproval(ctx, id); err != nil {
			return err
		}
		return authz.InProject(ctx, out.ProjectID)
	})
	return out, err
}

// Approvers is the approvals_at_least guard's input for a subject
// (domain.Subject.Approvers): every grant on its newest approval, one
// entry per grant, or none when that approval was denied or none was
// asked. The runner calls it while loading a step's subject snapshot.
func (s *WorkService) Approvers(ctx context.Context, project uuid.UUID, subject domain.ApprovalSubject) ([]string, error) {
	if _, err := actorInTenant(ctx); err != nil {
		return nil, err
	}
	if err := authz.InProject(ctx, project); err != nil {
		return nil, err
	}
	subject, err := subject.Canonical()
	if err != nil {
		return nil, err
	}
	var out []string
	err = s.tx.InTenant(ctx, func(ctx context.Context, st WorkStore) error {
		a, err := st.LatestApproval(ctx, project, subject)
		if errors.Is(err, ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		if a.State != domain.ApprovalDenied {
			out = a.Approvers()
		}
		return nil
	})
	return out, err
}

// ── shared ──────────────────────────────────────────────────────────

// actorInTenant is the caller, who must be acting in the tenant the
// context (and so the transaction) is scoped to.
func actorInTenant(ctx context.Context) (authz.Principal, error) {
	p, err := authz.Authenticated(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if t, ok := tenancy.FromContext(ctx); !ok || t != p.Tenant {
		return authz.Principal{}, fmt.Errorf("%w: principal is not scoped to this tenant", authz.ErrForbidden)
	}
	return p, nil
}

func assignmentEvent(typ string, a domain.Assignment, actor outbox.Actor) outbox.Event {
	return outbox.Event{
		Type: typ, AggregateType: domain.AggregateAssignment, AggregateID: a.ID.String(),
		Actor: actor, Payload: domain.AssignmentPayload(a, actor.String()), OccurredAt: a.UpdatedAt,
	}
}

func approvalEvent(typ string, a domain.Approval, d *domain.Decision, author string, actor outbox.Actor) outbox.Event {
	return outbox.Event{
		Type: typ, AggregateType: domain.AggregateApproval, AggregateID: a.ID.String(),
		Actor: actor, Payload: domain.ApprovalPayload(a, d, author, actor.String()),
	}
}

// within is the project set a list is cut to: nil when every project is
// visible.
func within(f authz.ProjectFilter) *[]uuid.UUID {
	if f.All() {
		return nil
	}
	ids := f.IDs()
	return &ids
}
