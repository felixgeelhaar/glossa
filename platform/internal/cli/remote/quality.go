package remote

import (
	"context"
	"net/http"

	"github.com/felixgeelhaar/glossa/platform/internal/apiclient"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// CheckPolicy reads the project's check-policy document (RFC 0005 §4.2).
//
// This is the source `glossa check` grades against. The document the
// server returns carries its rules, environments and version, so a run
// can say which version decided it — which the three fields of
// settings.check_policy could not.
func (c *Client) CheckPolicy(ctx context.Context, s Scope) (checkpolicy.Policy, error) {
	r, err := c.api.GetCheckPolicyWithResponse(ctx, s.Tenant, s.Project)
	if err := check(r, err, http.MethodGet, c.path("/v1/tenants/%s/projects/%s/check-policy", s.Tenant, s.Project)); err != nil {
		return checkpolicy.Policy{}, err
	}
	return policyFromWire(*r.JSON200), nil
}

// policyFromWire turns the API's document into the kernel's policy.
//
// The two disagree on one field, deliberately: the wire says
// require_complete as `all | listed | none` beside a `locales` list,
// because an enum is what an API can validate and document, while the
// kernel says it as nil (every locale), an empty slice (none) or the
// named ones. Everything else is the same shape under the same names,
// but this one cannot round-trip through JSON, so the whole conversion
// is written out rather than half-inferred.
func policyFromWire(st apiclient.CheckPolicyState) checkpolicy.Policy {
	doc := st.Document
	p := checkpolicy.Policy{
		Version:             st.Version,
		FailOn:              checkpolicy.Severity(doc.FailOn),
		MissingTranslations: checkpolicy.Severity(doc.MissingTranslations),
	}
	// nil is every locale; an empty slice is none. Required says the
	// first as All, so it is the one case that maps to leaving the
	// field alone.
	if req := requiredFromWire(doc.RequireComplete, doc.Locales); !req.All {
		p.RequireComplete = req.Locales
	}
	if doc.Schema != nil {
		p.Schema = string(*doc.Schema)
	}
	if st.EffectiveFrom != nil {
		t := *st.EffectiveFrom
		p.EffectiveFrom = &t
	}
	if st.GraceUntil != nil {
		t := *st.GraceUntil
		p.GraceUntil = &t
	}
	if doc.Rules != nil {
		p.Rules = make([]checkpolicy.Rule, 0, len(*doc.Rules))
		for _, r := range *doc.Rules {
			p.Rules = append(p.Rules, ruleFromWire(r))
		}
	}
	if doc.Environments != nil {
		p.Environments = make(map[string]checkpolicy.Environment, len(*doc.Environments))
		for name, e := range *doc.Environments {
			p.Environments[name] = environmentFromWire(e)
		}
	}
	return p
}

// requiredFromWire reads the enum and its list as one answer.
func requiredFromWire(req apiclient.CheckPolicyLocaleRequirement, locales *[]apiclient.Locale) checkpolicy.Required {
	switch req {
	case apiclient.CheckPolicyLocaleRequirementAll:
		return checkpolicy.AllLocales()
	case apiclient.CheckPolicyLocaleRequirementListed:
		named := []string{}
		if locales != nil {
			for _, l := range *locales {
				named = append(named, string(l))
			}
		}
		return checkpolicy.RequiredLocales(named...)
	default: // none
		return checkpolicy.RequiredLocales()
	}
}

func ruleFromWire(r apiclient.CheckPolicyRule) checkpolicy.Rule {
	out := checkpolicy.Rule{Severity: checkpolicy.Severity(r.Severity)}
	if r.Mode != nil {
		out.Mode = checkpolicy.Mode(*r.Mode)
	}
	if r.Layer != nil {
		out.Selector.Layer = string(*r.Layer)
	}
	if r.Code != nil {
		out.Selector.Code = *r.Code
	}
	if r.Locale != nil {
		out.Selector.Locale = string(*r.Locale)
	}
	if r.Namespace != nil {
		out.Selector.Namespace = string(*r.Namespace)
	}
	if r.Environment != nil {
		out.Selector.Environment = *r.Environment
	}
	return out
}

// environmentFromWire leaves what the block did not say unset, so
// "I did not name a require_complete" keeps inheriting the document's
// rather than becoming "every locale".
func environmentFromWire(e apiclient.CheckPolicyEnvironment) checkpolicy.Environment {
	out := checkpolicy.Environment{}
	if e.RequireComplete != nil {
		out.RequireComplete = requiredFromWire(*e.RequireComplete, e.Locales)
	}
	if e.RequireReview != nil {
		out.RequireReview = string(*e.RequireReview)
	}
	return out
}
