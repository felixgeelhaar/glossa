// Package config reads and writes glossa.yaml, the CLI's project file.
//
// The file is found by walking up from the working directory (like
// .git), so commands work from any subdirectory. Relative paths in it are
// relative to the file. GLOSSA_SERVER, GLOSSA_TENANT and GLOSSA_PROJECT
// override the file for CI.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
)

// FileName is the project file the CLI looks for.
const FileName = "glossa.yaml"

// LocalePlaceholder is replaced by a locale code in catalog paths.
const LocalePlaceholder = "{locale}"

// Config is glossa.yaml.
type Config struct {
	Version int `yaml:"version"`
	// Server is the glossa-server base URL (no /v1).
	Server string `yaml:"server"`
	// Tenant is the tenant ID. Empty: the API token's own tenant.
	Tenant string `yaml:"tenant,omitempty"`
	// Project is the project's slug or ID.
	Project string `yaml:"project"`
	// SourceLocale is the project's source locale; offline commands need it.
	SourceLocale string `yaml:"source_locale"`
	// Syntax is the authoring syntax of local catalogs: mf1 (default) or mf2.
	Syntax   string   `yaml:"syntax,omitempty"`
	Catalogs Catalogs `yaml:"catalogs"`
	Extract  Extract  `yaml:"extract,omitempty"`
	Generate Generate `yaml:"generate,omitempty"`
	Check    Check    `yaml:"check,omitempty"`
	Pull     Pull     `yaml:"pull,omitempty"`

	// Path is the file this config was read from ("" for a new one).
	Path string `yaml:"-"`
}

// Catalogs says where the local message catalogs are.
type Catalogs struct {
	// Path is a file pattern with {locale}: locales/{locale}.json. The
	// source locale's file is what `push` sends; the others hold
	// translations (`pull` writes them, `check --offline` reads them).
	Path string `yaml:"path"`
	// Style is how `pull` writes catalogs: flat (default) or nested.
	Style string `yaml:"style,omitempty"`
}

// Extract says which source files `extract` scans.
type Extract struct {
	Include []string `yaml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`
}

// Generate says where `generate` writes typed accessors. Empty outputs
// are skipped.
type Generate struct {
	// TypeScript is the typed module (messages.ts).
	TypeScript string `yaml:"typescript,omitempty"`
	// Vue is the @glossa/vue registration module; needs TypeScript.
	Vue string `yaml:"vue,omitempty"`
	// Go is the typed Go file.
	Go string `yaml:"go,omitempty"`
	// GoPackage is its package name (default: the directory's name).
	GoPackage string `yaml:"go_package,omitempty"`
	// GoRuntime is the Go runtime's import path.
	GoRuntime string `yaml:"go_runtime,omitempty"`
}

// Check is the default policy of `check`; flags override it.
type Check struct {
	// RequireComplete lists the locales that must be complete. Empty:
	// every locale of the project.
	RequireComplete []string `yaml:"require_complete,omitempty"`
	// FailOn is error (default) or warning.
	FailOn string `yaml:"fail_on,omitempty"`
}

// Pull configures `pull`.
type Pull struct {
	// Path is where pulled catalogs go (default: catalogs.path).
	Path string `yaml:"path,omitempty"`
	// States are the review states pulled (default: approved).
	States []string `yaml:"states,omitempty"`
}

// DefaultGoRuntime is the import path of the Go runtime.
const DefaultGoRuntime = "github.com/felixgeelhaar/glossa/runtimes/go"

// ErrNotFound means no glossa.yaml was found.
var ErrNotFound = errors.New("config: no " + FileName + " found")

// Find returns the path of the nearest glossa.yaml at or above dir.
func Find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load reads and validates the file at path, then applies the
// environment overrides (getenv may be nil).
func Load(path string, getenv func(string) string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		return nil, &InvalidError{Path: path, Problem: strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	c.Path = path
	c.applyEnv(getenv)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) applyEnv(getenv func(string) string) {
	if getenv == nil {
		return
	}
	for _, o := range []struct {
		env string
		dst *string
	}{{"GLOSSA_SERVER", &c.Server}, {"GLOSSA_TENANT", &c.Tenant}, {"GLOSSA_PROJECT", &c.Project}} {
		if v := getenv(o.env); v != "" {
			*o.dst = v
		}
	}
}

// InvalidError is a glossa.yaml that can't be used.
type InvalidError struct {
	Path    string
	Field   string
	Problem string
}

func (e *InvalidError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s: %s", e.Path, e.Field, e.Problem)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Problem)
}

// Validate checks the fields every command relies on and canonicalizes
// the locales.
func (c *Config) Validate() error {
	bad := func(field, format string, args ...any) error {
		return &InvalidError{Path: c.Path, Field: field, Problem: fmt.Sprintf(format, args...)}
	}
	if c.Version != 1 {
		return bad("version", "must be 1 (got %d)", c.Version)
	}
	if c.SourceLocale == "" {
		return bad("source_locale", "is required (the project's source locale, e.g. en)")
	}
	tag, err := bcp47.Parse(c.SourceLocale)
	if err != nil {
		return bad("source_locale", "%q is not a BCP 47 locale (e.g. en, de-CH)", c.SourceLocale)
	}
	c.SourceLocale = tag.String()
	switch c.Syntax {
	case "", "mf1", "mf2":
	default:
		return bad("syntax", "must be mf1 or mf2 (got %q)", c.Syntax)
	}
	if !strings.Contains(c.Catalogs.Path, LocalePlaceholder) {
		return bad("catalogs.path", "must contain %s, e.g. locales/%s.json", LocalePlaceholder, LocalePlaceholder)
	}
	switch c.Catalogs.Style {
	case "", "flat", "nested":
	default:
		return bad("catalogs.style", "must be flat or nested (got %q)", c.Catalogs.Style)
	}
	switch c.Check.FailOn {
	case "", "error", "warning":
	default:
		return bad("check.fail_on", "must be error or warning (got %q)", c.Check.FailOn)
	}
	if c.Generate.Vue != "" && c.Generate.TypeScript == "" {
		return bad("generate.vue", "needs generate.typescript (the Vue registration imports the typed module)")
	}
	return nil
}

// SyntaxOrDefault is the authoring syntax of local catalogs.
func (c *Config) SyntaxOrDefault() string {
	if c.Syntax == "" {
		return "mf1"
	}
	return c.Syntax
}

// Dir is the directory relative paths resolve against.
func (c *Config) Dir() string {
	if c.Path == "" {
		return "."
	}
	return filepath.Dir(c.Path)
}

// Resolve makes a path from the file absolute.
func (c *Config) Resolve(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.Dir(), filepath.FromSlash(p))
}

// CatalogPath is the catalog file of locale.
func (c *Config) CatalogPath(locale string) string {
	return c.Resolve(strings.ReplaceAll(c.Catalogs.Path, LocalePlaceholder, locale))
}

// PullPath is where `pull` writes locale's catalog.
func (c *Config) PullPath(locale string) string {
	pattern := c.Pull.Path
	if pattern == "" {
		pattern = c.Catalogs.Path
	}
	return c.Resolve(strings.ReplaceAll(pattern, LocalePlaceholder, locale))
}

// Marshal renders the config as YAML with a short header.
func (c *Config) Marshal() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("# Glossa project file: https://github.com/felixgeelhaar/glossa (platform/cmd/glossa)\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Write saves the config to path, refusing to overwrite unless force.
func (c *Config) Write(path string, force bool) error {
	data, err := c.Marshal()
	if err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flags |= os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	c.Path = path
	return f.Close()
}
