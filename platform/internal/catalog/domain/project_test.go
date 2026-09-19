package domain_test

import (
	"errors"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

func TestNewProject(t *testing.T) {
	p, err := domain.NewProject(tenancy.NewID(), "brotwerk", "Brotwerk", bcp47.MustParse("de"), domain.DefaultSettings(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.SourceLocale.String() != "de" || p.Settings.DefaultSyntax != mfcontent.MF1 || !p.Settings.ReviewRequired {
		t.Errorf("project = %+v", p)
	}
	if _, err := domain.NewProject(tenancy.NewID(), "x", "", en, domain.DefaultSettings(), t0); !errors.Is(err, domain.ErrInvalidName) {
		t.Errorf("empty name: %v", err)
	}
	if _, err := domain.NewProject(tenancy.NewID(), "x", "X", bcp47.Tag{}, domain.DefaultSettings(), t0); !errors.Is(err, bcp47.ErrInvalid) {
		t.Errorf("no source locale: %v", err)
	}
	if _, err := domain.NewProject(tenancy.NewID(), "x", "X", en, domain.Settings{DefaultSyntax: "icu"}, t0); !errors.Is(err, mfcontent.ErrInvalidSyntax) {
		t.Errorf("bad settings: %v", err)
	}
}

func TestProjectChange(t *testing.T) {
	p, _ := domain.NewProject(tenancy.NewID(), "brotwerk", "Brotwerk", en, domain.DefaultSettings(), t0)
	name := "Brotwerk Web"
	settings := domain.Settings{DefaultSyntax: mfcontent.MF2}
	changed, err := p.Change(domain.ProjectChange{Name: &name, Settings: &settings}, t0)
	if err != nil || !changed || p.Version != 2 || p.Name != name || p.Settings.ReviewRequired {
		t.Fatalf("change: %v %v %+v", changed, err, p)
	}
	if changed, _ := p.Change(domain.ProjectChange{Name: &name}, t0); changed || p.Version != 2 {
		t.Error("no-op change bumped the version")
	}
}

func TestProjectDefaultBranch(t *testing.T) {
	p, err := domain.NewProject(tenancy.NewID(), "brotwerk", "Brotwerk", en, domain.Settings{DefaultSyntax: mfcontent.MF1}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Settings.DefaultBranch != domain.DefaultBranch {
		t.Errorf("default branch of a new project = %q, want %q", p.Settings.DefaultBranch, domain.DefaultBranch)
	}
	// Settings without a default branch keep the current one.
	trunk := domain.Settings{DefaultSyntax: mfcontent.MF1, DefaultBranch: "trunk"}
	if changed, err := p.Change(domain.ProjectChange{Settings: &trunk}, t0); err != nil || !changed || p.Settings.DefaultBranch != "trunk" {
		t.Fatalf("set trunk: %v %v %+v", changed, err, p.Settings)
	}
	keep := domain.Settings{DefaultSyntax: mfcontent.MF1}
	if changed, err := p.Change(domain.ProjectChange{Settings: &keep}, t0); err != nil || changed || p.Settings.DefaultBranch != "trunk" {
		t.Errorf("settings without default_branch: %v %v %+v", changed, err, p.Settings)
	}
	for _, bad := range []string{"feat/.hidden", "a b", "x..y", "@", "main.lock"} {
		s := domain.Settings{DefaultSyntax: mfcontent.MF1, DefaultBranch: domain.BranchName(bad)}
		if _, err := p.Change(domain.ProjectChange{Settings: &s}, t0); !errors.Is(err, domain.ErrInvalidBranchName) {
			t.Errorf("default branch %q: %v, want ErrInvalidBranchName", bad, err)
		}
		if _, err := domain.NewProject(tenancy.NewID(), "x", "X", en, s, t0); !errors.Is(err, domain.ErrInvalidBranchName) {
			t.Errorf("new project with default branch %q: %v", bad, err)
		}
	}
}

func TestSlugs(t *testing.T) {
	for _, bad := range []string{"", "-a", "a-", "A", "a_b"} {
		if _, err := domain.ParseSlug(bad); !errors.Is(err, domain.ErrInvalidSlug) {
			t.Errorf("ParseSlug(%q) = %v", bad, err)
		}
	}
}

func TestApplications(t *testing.T) {
	a, err := domain.NewApplication(domain.NewProjectID(), "web", "Web shop", domain.PlatformWeb, t0)
	if err != nil || a.Version != 1 {
		t.Fatalf("new application: %v %+v", err, a)
	}
	if _, err := domain.NewApplication(domain.NewProjectID(), "tv", "TV", "tv", t0); !errors.Is(err, domain.ErrInvalidPlatform) {
		t.Errorf("bad platform: %v", err)
	}
	p := domain.PlatformAPI
	if changed, err := a.Change(domain.ApplicationChange{Platform: &p}, t0); err != nil || !changed || a.Version != 2 {
		t.Errorf("change platform: %v %v", changed, err)
	}
	if changed, _ := a.Change(domain.ApplicationChange{Platform: &p}, t0); changed {
		t.Error("no-op change reported")
	}
}
