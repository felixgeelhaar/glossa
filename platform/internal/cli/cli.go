// Package cli is the glossa command: the developer and CI interface to
// Glossa (product intent §46, RFC 0002 §9). platform/cmd/glossa is only
// its entry point.
//
// Every command:
//
//   - reads glossa.yaml (found by walking up from the working directory),
//   - talks to glossa-server through the generated /v1 client with an
//     API token (GLOSSA_TOKEN, else the one `glossa login` stored),
//   - prints human output, or exactly one JSON document with --json
//     (shapes in cmd/glossa/README.md, each tagged with a "schema"),
//   - exits with a documented code (ExitCode).
//
// The MessageFormat kernel (messageformat) runs locally, so `check
// --offline`, `extract` and `generate` work without a server.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/credentials"
)

// Env is everything the CLI touches outside itself, so tests can run
// every command in-process.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	// Dir is the working directory.
	Dir string
	// Interactive is true when stdin is a terminal: prompts are allowed.
	Interactive bool
	// ColorOutput is true when stdout is a terminal that takes color.
	ColorOutput bool
	// HTTP is the transport for glossa-server and v0.3 (nil: default).
	HTTP *http.Client
	// Credentials stores API tokens (nil: the OS keychain or the file).
	Credentials credentials.Store
	// ReadSecret reads a secret from the terminal without echo (nil:
	// read a line from Stdin).
	ReadSecret func(prompt string) (string, error)
	// Version is the CLI's version, for --version and the User-Agent.
	Version string
}

func (e *Env) getenv(k string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(k)
}

// command is one subcommand.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, inv *invocation, args []string) error
}

func commands() []command {
	return []command{
		{"init", "Create glossa.yaml for this project", runInit},
		{"login", "Store an API token for the server", runLogin},
		{"logout", "Forget the stored API token", runLogout},
		{"whoami", "Show the server, token and tenant in use", runWhoami},
		{"push", "Send the source catalog's messages to the server", runPush},
		{"pull", "Write translations to local catalogs", runPull},
		{"extract", "Find message usages in source code", runExtract},
		{"generate", "Generate typed message accessors (TypeScript, Vue, Go)", runGenerate},
		{"check", "Check the project: structure, arguments, completeness", runCheck},
		{"status", "Show translation coverage per locale", runStatus},
		{"diff", "Compare local catalogs with the server", runDiff},
		{"locales", "List the project's locales", runLocales},
		{"messages", "List the project's messages", runMessages},
		{"import", "Import from another system (--from v0: Glossa v0.3)", runImport},
		{"release", "Publish, promote and roll back releases", runRelease},
		{"tm", "Search the translation memory; list and retire units", runTM},
	}
}

// Main runs the CLI with args (without the program name) and returns the
// process exit code.
func Main(ctx context.Context, args []string, env Env) int {
	if env.Stdout == nil {
		env.Stdout = io.Discard
	}
	if env.Stderr == nil {
		env.Stderr = io.Discard
	}
	if env.Stdin == nil {
		env.Stdin = strings.NewReader("")
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if len(args) > 1 {
			return Main(ctx, []string{args[1], "--help"}, env)
		}
		printHelp(env.Stdout)
		return int(ExitOK)
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Fprintf(env.Stdout, "glossa %s\n", versionOr(env.Version))
		return int(ExitOK)
	}
	for _, c := range commands() {
		if c.name != args[0] {
			continue
		}
		inv := &invocation{env: env, name: c.name}
		err := c.run(ctx, inv, args[1:])
		return inv.finish(err)
	}
	inv := &invocation{env: env, name: "glossa"}
	inv.setupOutput()
	return inv.finish(&Error{Exit: ExitUsage, Code: "unknown_command",
		What: fmt.Sprintf("unknown command %q", args[0]),
		Fix:  "run `glossa help` for the list of commands"})
}

func versionOr(v string) string {
	if v == "" {
		return "dev"
	}
	return v
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `glossa — localization infrastructure for developers and CI

Usage: glossa <command> [flags]

Commands:
`)
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprint(w, `
Flags every command takes:
  --json       print one JSON document (stable shapes; see the README)
  --quiet      print nothing but errors
  --no-color   never color output (also NO_COLOR=1)
  --config     path to glossa.yaml (default: nearest one up from here)

Exit codes: 0 ok · 1 check failed · 2 usage or config · 3 network or auth · 4 partial failure
Run "glossa <command> --help" for a command's flags.
`)
}

// invocation is one command run: its flags, output and lazily loaded
// config and client.
type invocation struct {
	env  Env
	name string

	json, quiet, noColor bool
	configPath           string

	out *printer
}

// flags returns a FlagSet with the global flags registered.
func (inv *invocation) flags(usage string) *flag.FlagSet {
	fs := flag.NewFlagSet("glossa "+inv.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&inv.json, "json", false, "print one JSON document")
	fs.BoolVar(&inv.quiet, "quiet", false, "print nothing but errors")
	fs.BoolVar(&inv.noColor, "no-color", false, "never color output")
	fs.StringVar(&inv.configPath, "config", "", "path to glossa.yaml")
	fs.Usage = func() {
		fmt.Fprintf(inv.env.Stdout, "Usage: glossa %s\n\n", usage)
		fs.SetOutput(inv.env.Stdout)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
	}
	return fs
}

// errHelp ends a run after --help was printed.
var errHelp = errors.New("help requested")

// parse parses args (flags may follow positional arguments) and sets up
// output. It returns the positional arguments.
func (inv *invocation) parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			inv.setupOutput()
			if errors.Is(err, flag.ErrHelp) {
				// flag already printed the usage.
				return nil, errHelp
			}
			return nil, usageError(inv.name, "%v", err)
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
	inv.setupOutput()
	return positional, nil
}

func (inv *invocation) setupOutput() {
	color := inv.env.ColorOutput && !inv.noColor && !inv.json && inv.env.getenv("NO_COLOR") == ""
	inv.out = &printer{w: inv.env.Stdout, color: color, quiet: inv.quiet || inv.json}
}

// emit prints the result: v as JSON with --json, else human(out).
func (inv *invocation) emit(v any, human func(p *printer)) error {
	if inv.json {
		return writeJSON(inv.env.Stdout, v)
	}
	human(inv.out)
	return nil
}

// finish turns the command's error into output and an exit code.
func (inv *invocation) finish(err error) int {
	if err == nil || errors.Is(err, errHelp) {
		return int(ExitOK)
	}
	if inv.out == nil {
		inv.setupOutput()
	}
	e := asError(err)
	if e.Silent {
		return int(e.Exit)
	}
	if inv.json {
		_ = writeJSON(inv.env.Stdout, map[string]any{"schema": "glossa.cli.error/v1", "error": e.json()})
		return int(e.Exit)
	}
	p := &printer{w: inv.env.Stderr, color: inv.out.color}
	printError(inv.env.Stderr, p, e)
	return int(e.Exit)
}
