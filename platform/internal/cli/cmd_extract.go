package cli

import (
	"context"
	"fmt"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/codegen"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/extract"
)

type location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type unknownJSON struct {
	Key       string     `json:"key"`
	Locations []location `json:"locations"`
}

type extractJSON struct {
	Schema string          `json:"schema"`
	Files  int             `json:"files"`
	Usages []extract.Usage `json:"usages"`
	// Unknown are IDs used in code that the source catalog lacks.
	Unknown []unknownJSON `json:"unknown"`
	// Unused are catalog messages no code uses (dynamic IDs can't be seen).
	Unused []string `json:"unused"`
}

func runExtract(_ context.Context, inv *invocation, args []string) error {
	fs := inv.flags("extract [--strict]")
	strict := fs.Bool("strict", false, "exit 1 when code uses message IDs the catalog doesn't have")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	cfg, err := inv.loadConfig()
	if err != nil {
		return err
	}
	local, err := loadLocal(cfg)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(local.Messages))
	known := map[string]bool{}
	for _, m := range local.Messages {
		keys = append(keys, m.Key)
		known[m.Key] = true
	}
	opts := extract.Options{Include: cfg.Extract.Include, Exclude: cfg.Extract.Exclude,
		Accessors: extract.Accessors{TS: codegen.TSNames(keys), Go: codegen.GoNames(keys)}}
	if len(opts.Include) == 0 {
		opts.Include, opts.Exclude = defaultExtract.Include, append(opts.Exclude, defaultExtract.Exclude...)
	}
	usages, files, err := extract.Scan(cfg.Dir(), opts)
	if err != nil {
		return &Error{Exit: ExitUsage, Code: "scan_failed", What: "can't scan the source tree", Where: cfg.Dir(), Why: err.Error(),
			Fix: "check extract.include / extract.exclude in glossa.yaml"}
	}
	out := extractJSON{Schema: "glossa.cli.extract/v1", Files: files, Usages: usages, Unknown: []unknownJSON{}, Unused: []string{}}
	if out.Usages == nil {
		out.Usages = []extract.Usage{}
	}
	used := map[string]bool{}
	for _, u := range usages {
		used[u.Key] = true
		if known[u.Key] {
			continue
		}
		if n := len(out.Unknown); n == 0 || out.Unknown[n-1].Key != u.Key {
			out.Unknown = append(out.Unknown, unknownJSON{Key: u.Key})
		}
		last := &out.Unknown[len(out.Unknown)-1]
		last.Locations = append(last.Locations, location{File: u.File, Line: u.Line})
	}
	for _, k := range keys {
		if !used[k] {
			out.Unused = append(out.Unused, k)
		}
	}
	if err := inv.emit(out, func(p *printer) { printExtract(p, out, len(keys)) }); err != nil {
		return err
	}
	if *strict && len(out.Unknown) > 0 {
		return silentExit(ExitCheckFailed, "unknown_messages")
	}
	return nil
}

func printExtract(p *printer, out extractJSON, catalogSize int) {
	distinct := map[string]bool{}
	for _, u := range out.Usages {
		distinct[u.Key] = true
	}
	p.line("%s %s of %s in %s", p.pass(), plural(len(out.Usages), "usage", "usages"),
		plural(len(distinct), "message", "messages"), plural(out.Files, "file", "files"))
	if len(out.Unknown) == 0 {
		p.line("%s every used ID is in the catalog", p.pass())
	} else {
		p.line("%s %s not in the catalog:", p.fail(), plural(len(out.Unknown), "ID", "IDs"))
		for _, u := range out.Unknown {
			l := u.Locations[0]
			more := ""
			if len(u.Locations) > 1 {
				more = p.dim(fmt.Sprintf(" (+%d more)", len(u.Locations)-1))
			}
			p.line("  %s  %s:%d%s", u.Key, l.File, l.Line, more)
		}
	}
	if len(out.Unused) > 0 {
		p.line("%s %d of %d catalog messages look unused (dynamic IDs aren't detected):", p.caution(), len(out.Unused), catalogSize)
		for i, k := range out.Unused {
			if i == maxFindingsPerGroup {
				p.line("  %s", p.dim(fmt.Sprintf("… and %d more (--json lists all)", len(out.Unused)-i)))
				break
			}
			p.line("  %s", k)
		}
	}
}
