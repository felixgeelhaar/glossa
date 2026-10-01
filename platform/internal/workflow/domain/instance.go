package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.klarlabs.de/statekit"
)

// Instance statuses (RFC 0006 §2.5).
const (
	StatusActive   = "active"
	StatusFinished = "finished"
)

// NotStarted is the state of an instance whose initial state's entry
// actions were refused: it never entered the chart, and the next event
// tries again.
const NotStarted = ""

// Instance is one workflow instance: a subject running on one version
// of one definition. It exists only while work is in flight; a finished
// instance keeps its transition log and drops its snapshot.
type Instance struct {
	ID         uuid.UUID
	Project    uuid.UUID
	Definition uuid.UUID
	Version    int
	Kind       SubjectKind
	// SubjectID is the message of a translation unit, the release
	// request otherwise.
	SubjectID uuid.UUID
	// Locale is a translation unit's canonical BCP 47 tag; empty for a
	// release request.
	Locale string
	State  string
	Status string
	// Timer is the instance's pending due date, if an action set one.
	Timer      *Timer
	CreatedAt  time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time
}

// Started reports whether the instance has entered its chart.
func (i Instance) Started() bool { return i.State != NotStarted }

// Timer is when timer.due and timer.overdue fall due for an instance,
// and the state that set them (RFC 0006 §2.3: a stored due_at, never a
// timer in the chart). Leaving that state cancels them.
type Timer struct {
	State string
	// DueAt and OverdueAt are nil once raised.
	DueAt     *time.Time
	OverdueAt *time.Time
}

// NewTimer is the timer an action with a due period sets in state at
// now: timer.due when the period has passed, and timer.overdue when it
// has passed again — the escalation a definition can react to.
func NewTimer(state string, now time.Time, due Duration) *Timer {
	d, o := now.Add(due.Std()), now.Add(2*due.Std())
	return &Timer{State: state, DueAt: &d, OverdueAt: &o}
}

// Due returns the timer event that has fallen due at now, if any, and
// the timer that remains once it is raised (nil when nothing remains).
// timer.due is raised before timer.overdue.
func (t *Timer) Due(now time.Time) (EventName, *Timer, bool) {
	if t == nil {
		return "", nil, false
	}
	rest := *t
	switch {
	case t.DueAt != nil && !t.DueAt.After(now):
		rest.DueAt = nil
		return EventTimerDue, rest.orNil(), true
	case t.DueAt == nil && t.OverdueAt != nil && !t.OverdueAt.After(now):
		rest.OverdueAt = nil
		return EventTimerOverdue, rest.orNil(), true
	}
	return "", t, false
}

func (t Timer) orNil() *Timer {
	if t.DueAt == nil && t.OverdueAt == nil {
		return nil
	}
	return &t
}

// snapshot is what an instance stores to Restore from: the machine and
// the state. A definition's chart has no history or parallel states and
// no delayed transitions (§2.3), so the state is the whole of
// statekit's snapshot; the step context is loaded afresh for every
// transition and never stored, so no subject data is kept with it.
type snapshot struct {
	MachineID string `json:"machine_id"`
	State     string `json:"state"`
}

// Snapshot is what an instance in state stores to Restore from.
func (d *Definition) Snapshot(state string) json.RawMessage {
	b, _ := json.Marshal(snapshot{MachineID: d.machine.ID, State: state})
	return b
}

// RestoreState reads a stored snapshot back into the state to Advance
// from, refusing one taken on another chart.
func (d *Definition) RestoreState(raw []byte) (string, error) {
	var s snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("workflow: snapshot: %w", err)
	}
	if s.MachineID != d.machine.ID {
		return "", fmt.Errorf("workflow: snapshot of %q restored on %q", s.MachineID, d.machine.ID)
	}
	if d.machine.GetState(statekit.StateID(s.State)) == nil {
		return "", fmt.Errorf("workflow: snapshot state %q is not in %q", s.State, d.machine.ID)
	}
	return s.State, nil
}

// GuardOutcome is one guard evaluated for a transition.
type GuardOutcome struct {
	Guard  string
	Passed bool
}

// GuardOutcomes evaluates every guard on a transition out of state (or
// out of one of its ancestors) for step's event, in the order statekit
// would consider them — what the transition log records beside the
// transition statekit took, so "why didn't it move?" has an answer.
func (d *Definition) GuardOutcomes(state string, step Step) []GuardOutcome {
	var out []GuardOutcome
	for id := statekit.StateID(state); id != ""; {
		s := d.machine.GetState(id)
		if s == nil {
			break
		}
		for _, t := range s.Transitions {
			if string(t.Event) != string(step.Trigger.Event) || t.Guard == "" {
				continue
			}
			fn, ok := d.guardFns[string(t.Guard)]
			out = append(out, GuardOutcome{Guard: string(t.Guard), Passed: ok && fn(step)})
		}
		id = s.Parent
	}
	return out
}
