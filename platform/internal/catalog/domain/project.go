// Package domain is the Catalog context's model: what a product says
// (RFC 0002 §4). A Project owns Applications (its runtime surfaces) and
// Messages; the Message is the aggregate root of the whole domain
// (intent §7–8), with its source content as a canonical MessageFormat 2
// model and an append-only log of source revisions.
package domain

import (
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Slug is a project's or application's URL-safe handle, unique within
// its parent: a lowercase DNS label.
type Slug string

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// ParseSlug validates s.
func ParseSlug(s string) (Slug, error) {
	if !slugPattern.MatchString(s) {
		return "", fmt.Errorf("%w: %q", ErrInvalidSlug, s)
	}
	return Slug(s), nil
}

// ParseName validates a display name.
func ParseName(s string) (string, error) {
	if n := utf8.RuneCountInString(s); n < 1 || n > 200 {
		return "", ErrInvalidName
	}
	return s, nil
}

// Settings are a project's tunables. Unset members take their defaults.
type Settings struct {
	// DefaultSyntax is the authoring syntax assumed when a write doesn't
	// name one. ICU MF1 unless the project says otherwise (RFC 0002 §5).
	DefaultSyntax mfcontent.Syntax `json:"default_syntax"`
	// ReviewRequired sends new translations to review instead of
	// approving them on write. It is the whole review policy until the
	// workflow engine (Phase 4) owns it.
	ReviewRequired bool `json:"review_required"`
}

// DefaultSettings are a new project's settings.
func DefaultSettings() Settings {
	return Settings{DefaultSyntax: mfcontent.MF1, ReviewRequired: true}
}

// Validate checks every member.
func (s Settings) Validate() error {
	_, err := mfcontent.ParseSyntax(string(s.DefaultSyntax), "")
	if err != nil || s.DefaultSyntax == "" {
		return fmt.Errorf("%w: default_syntax", mfcontent.ErrInvalidSyntax)
	}
	return nil
}

// Project is a product's localizable surface as a whole: its source
// locale, its applications and its messages. The source locale is fixed
// at creation — every source revision and translation is relative to it.
type Project struct {
	ID           ProjectID
	TenantID     tenancy.ID
	Slug         Slug
	Name         string
	SourceLocale bcp47.Tag
	Settings     Settings
	// Version increments with every change; it is the project's ETag.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewProject creates a project.
func NewProject(tenant tenancy.ID, slug Slug, name string, source bcp47.Tag, settings Settings, now time.Time) (Project, error) {
	if _, err := ParseName(name); err != nil {
		return Project{}, err
	}
	if source.IsZero() {
		return Project{}, bcp47.ErrInvalid
	}
	if err := settings.Validate(); err != nil {
		return Project{}, err
	}
	return Project{
		ID: NewProjectID(), TenantID: tenant, Slug: slug, Name: name, SourceLocale: source,
		Settings: settings, Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// ProjectChange is a partial update; nil members keep their value.
type ProjectChange struct {
	Slug     *Slug
	Name     *string
	Settings *Settings
}

// Change applies c and reports whether anything changed.
func (p *Project) Change(c ProjectChange, now time.Time) (bool, error) {
	next := *p
	if c.Slug != nil {
		next.Slug = *c.Slug
	}
	if c.Name != nil {
		if _, err := ParseName(*c.Name); err != nil {
			return false, err
		}
		next.Name = *c.Name
	}
	if c.Settings != nil {
		if err := c.Settings.Validate(); err != nil {
			return false, err
		}
		next.Settings = *c.Settings
	}
	if next.Slug == p.Slug && next.Name == p.Name && next.Settings == p.Settings {
		return false, nil
	}
	next.Version++
	next.UpdatedAt = now
	*p = next
	return true, nil
}

// Platform is the kind of runtime surface an application is.
type Platform string

// Platforms.
const (
	PlatformWeb     Platform = "web"
	PlatformAPI     Platform = "api"
	PlatformIOS     Platform = "ios"
	PlatformAndroid Platform = "android"
	PlatformOther   Platform = "other"
)

// ParsePlatform validates p.
func ParsePlatform(p string) (Platform, error) {
	switch Platform(p) {
	case PlatformWeb, PlatformAPI, PlatformIOS, PlatformAndroid, PlatformOther:
		return Platform(p), nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidPlatform, p)
}

// Application is a runtime surface of a project — a web app, an API's
// emails, an iOS app (RFC 0002 §6). Delivery keys, usage context and
// per-application releases will hang off it.
type Application struct {
	ID        ApplicationID
	ProjectID ProjectID
	Slug      Slug
	Name      string
	Platform  Platform
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewApplication creates an application of project.
func NewApplication(project ProjectID, slug Slug, name string, platform Platform, now time.Time) (Application, error) {
	if _, err := ParseName(name); err != nil {
		return Application{}, err
	}
	if _, err := ParsePlatform(string(platform)); err != nil {
		return Application{}, err
	}
	return Application{
		ID: NewApplicationID(), ProjectID: project, Slug: slug, Name: name, Platform: platform,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// ApplicationChange is a partial update; nil members keep their value.
type ApplicationChange struct {
	Slug     *Slug
	Name     *string
	Platform *Platform
}

// Change applies c and reports whether anything changed.
func (a *Application) Change(c ApplicationChange, now time.Time) (bool, error) {
	next := *a
	if c.Slug != nil {
		next.Slug = *c.Slug
	}
	if c.Name != nil {
		if _, err := ParseName(*c.Name); err != nil {
			return false, err
		}
		next.Name = *c.Name
	}
	if c.Platform != nil {
		if _, err := ParsePlatform(string(*c.Platform)); err != nil {
			return false, err
		}
		next.Platform = *c.Platform
	}
	if next == *a {
		return false, nil
	}
	next.Version++
	next.UpdatedAt = now
	*a = next
	return true, nil
}
