package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
)

var (
	now         = time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	project     = uuid.MustParse("0192a1b2-0000-7000-8000-0000000000aa")
	application = uuid.MustParse("0192a1b2-0000-7000-8000-0000000000bb")
)

// rfcDocument is the example of RFC 0004 §2.2.
const rfcDocument = `{
  "schema": "glossa.usages/v1",
  "application": "web",
  "commit": "9f2c1e7ab4", "branch": "feat/checkout-copy",
  "tool": { "name": "@glossa/unplugin", "version": "0.1.0" },
  "usages": [
    { "key": "checkout.pay", "file": "src/checkout/PaymentFooter.vue", "line": 42, "column": 9,
      "component": "PaymentFooter", "route": "/checkout/payment", "kind": "t" }
  ]
}`

func TestParseUploadReadsTheRFCDocument(t *testing.T) {
	up, err := domain.ParseUpload([]byte(rfcDocument))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(rfcDocument))
	if got, want := up.Digest.String(), hex.EncodeToString(sum[:]); got != want {
		t.Errorf("digest = %s, want the SHA-256 of the bytes %s", got, want)
	}
	if up.Application != "web" || up.Commit.String() != "9f2c1e7ab4" || up.Branch.String() != "feat/checkout-copy" {
		t.Errorf("header = %q %q %q", up.Application, up.Commit, up.Branch)
	}
	if up.Tool != (domain.Tool{Name: "@glossa/unplugin", Version: "0.1.0"}) {
		t.Errorf("tool = %+v", up.Tool)
	}
	want := domain.Usage{
		Key: "checkout.pay", File: "src/checkout/PaymentFooter.vue", Line: 42, Column: 9,
		Component: "PaymentFooter", Route: "/checkout/payment", Kind: "t",
	}
	if len(up.Usages) != 1 || up.Usages[0] != want {
		t.Errorf("usages = %+v, want [%+v]", up.Usages, want)
	}
}

func TestParseUploadCanonicalizesTheCommit(t *testing.T) {
	up, err := domain.ParseUpload([]byte(strings.Replace(rfcDocument, "9f2c1e7ab4", "9F2C1E7AB4", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if up.Commit.String() != "9f2c1e7ab4" {
		t.Errorf("commit = %s, want lower case", up.Commit)
	}
}

func TestParseUploadRejectsInvalidDocuments(t *testing.T) {
	cases := map[string]string{
		"not json":           `{`,
		"wrong schema":       strings.Replace(rfcDocument, "glossa.usages/v1", "glossa.usages/v2", 1),
		"no application":     strings.Replace(rfcDocument, `"web"`, `""`, 1),
		"bad commit":         strings.Replace(rfcDocument, "9f2c1e7ab4", "HEAD", 1),
		"short commit":       strings.Replace(rfcDocument, "9f2c1e7ab4", "9f2c", 1),
		"no branch":          strings.Replace(rfcDocument, "feat/checkout-copy", "", 1),
		"branch with space":  strings.Replace(rfcDocument, "feat/checkout-copy", "feat checkout", 1),
		"branch with dotdot": strings.Replace(rfcDocument, "feat/checkout-copy", "feat/../main", 1),
		"no tool name":       strings.Replace(rfcDocument, "@glossa/unplugin", "", 1),
		"no key":             strings.Replace(rfcDocument, `"checkout.pay"`, `""`, 1),
		"no file":            strings.Replace(rfcDocument, `"src/checkout/PaymentFooter.vue"`, `""`, 1),
		"line zero":          strings.Replace(rfcDocument, `"line": 42`, `"line": 0`, 1),
		"negative column":    strings.Replace(rfcDocument, `"column": 9`, `"column": -1`, 1),
		"bad kind":           strings.Replace(rfcDocument, `"kind": "t"`, `"kind": "T()"`, 1),
		"no kind":            strings.Replace(rfcDocument, `, "kind": "t"`, ``, 1),
		"control in file":    strings.Replace(rfcDocument, "PaymentFooter.vue", `Payment\u0000Footer.vue`, 1),
		"long component":     strings.Replace(rfcDocument, `"PaymentFooter",`, `"`+strings.Repeat("C", 201)+`",`, 1),
		"trailing data":      rfcDocument + `{}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.ParseUpload([]byte(doc))
			if !errors.Is(err, domain.ErrInvalidUpload) {
				t.Errorf("err = %v, want ErrInvalidUpload", err)
			}
		})
	}
}

func TestParseUploadAllowsOptionalLocationFields(t *testing.T) {
	doc := `{"schema":"glossa.usages/v1","application":"api","commit":"0123456789abcdef0123456789abcdef01234567",
	  "branch":"main","tool":{"name":"glossa extract"},
	  "usages":[{"key":"mail.welcome.subject","file":"internal/mail/welcome.go","line":7,"kind":"go"}]}`
	up, err := domain.ParseUpload([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	u := up.Usages[0]
	if u.Column != 0 || u.Component != "" || u.Route != "" || up.Tool.Version != "" {
		t.Errorf("optional fields = %+v, tool %+v", u, up.Tool)
	}
}

func TestParseUploadEnforcesTheUsageLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"schema":"glossa.usages/v1","application":"web","commit":"abcdef1","branch":"main","tool":{"name":"x"},"usages":[`)
	for i := range domain.MaxUsagesPerBuild + 1 {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"key":"k","file":"f","line":%d,"kind":"t"}`, i+1)
	}
	b.WriteString(`]}`)
	_, err := domain.ParseUpload([]byte(b.String()))
	if !errors.Is(err, domain.ErrTooManyUsages) {
		t.Errorf("err = %v, want ErrTooManyUsages", err)
	}
}

func TestParseUploadEnforcesTheSizeLimit(t *testing.T) {
	doc := make([]byte, domain.MaxUploadBytes+1)
	_, err := domain.ParseUpload(doc)
	if !errors.Is(err, domain.ErrUploadTooLarge) {
		t.Errorf("err = %v, want ErrUploadTooLarge", err)
	}
}

func TestParseUploadAcceptsAnEmptyUsageList(t *testing.T) {
	doc := `{"schema":"glossa.usages/v1","application":"web","commit":"abcdef1","branch":"main","tool":{"name":"x"},"usages":[]}`
	up, err := domain.ParseUpload([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Usages) != 0 {
		t.Errorf("usages = %v", up.Usages)
	}
}

func TestNewBuild(t *testing.T) {
	up, err := domain.ParseUpload([]byte(rfcDocument))
	if err != nil {
		t.Fatal(err)
	}
	b, err := domain.NewBuild(project, application, up, domain.SourcePlugin, false, "token:ci", now)
	if err != nil {
		t.Fatal(err)
	}
	if b.ID == uuid.Nil || b.ID.Version() != 7 {
		t.Errorf("id = %s, want a UUIDv7", b.ID)
	}
	if b.ProjectID != project || b.ApplicationID != application || b.Source != domain.SourcePlugin ||
		b.OnDefaultBranch || b.UsageCount != 1 || b.Digest != up.Digest || b.CreatedBy != "token:ci" || !b.CreatedAt.Equal(now) {
		t.Errorf("build = %+v", b)
	}
	if _, err := domain.NewBuild(project, application, up, domain.Source("ci"), false, "token:ci", now); !errors.Is(err, domain.ErrInvalidSource) {
		t.Errorf("unknown source: err = %v", err)
	}
}

func TestParseSource(t *testing.T) {
	for _, s := range []string{"plugin", "extract", "runtime", "capture"} {
		if _, err := domain.ParseSource(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	if _, err := domain.ParseSource("bundler"); !errors.Is(err, domain.ErrInvalidSource) {
		t.Errorf("bundler: err = %v", err)
	}
}

func TestParseBranch(t *testing.T) {
	for _, ok := range []string{"main", "feat/checkout-copy", "release/2026.09", "renovate/@vue-3.x", "pr_7"} {
		if _, err := domain.ParseBranch(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/main", "main/", "-x", "a..b", "a b", "a~1", "a^", "a:b", "a?", "a*", "a[", `a\b`,
		"x.lock", "a//b", "@", strings.Repeat("b", 256), "a\tb"} {
		if _, err := domain.ParseBranch(bad); !errors.Is(err, domain.ErrInvalidBranch) {
			t.Errorf("%q: err = %v, want ErrInvalidBranch", bad, err)
		}
	}
}
