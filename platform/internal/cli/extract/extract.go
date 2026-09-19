// Package extract finds message usages in source code (product intent
// §32): the web components (<glossa-text key="…">), Vue
// (<GlossaText id="…">, t("…"), $t("…")), the Go runtime
// (client.T(ctx, "…"), l.T("…"), {{t "…"}} in templates) and the typed
// accessors `glossa generate` writes (messages.checkout.pay(…),
// m.CheckoutPay(…)). Each usage records file and line; the Context
// context will receive them once it exists (M3).
//
// Extraction is lexical, not a full parse: it sees literal IDs only, which
// is what a message ID should be.
package extract

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Usage is one place a message is used.
type Usage struct {
	Key    string `json:"key"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	// Kind is element, t, go, template or typed.
	Kind string `json:"kind"`
}

// Accessors map the typed accessor names `generate` emits back to IDs.
type Accessors struct {
	// TS maps an accessor path (checkout.paymentFailed) to its ID.
	TS map[string]string
	// Go maps a method name (CheckoutPaymentFailed) to its ID.
	Go map[string]string
}

// Options configure a scan.
type Options struct {
	Include   []string
	Exclude   []string
	Accessors Accessors
}

const keyRE = `([a-z0-9_-]+(?:\.[a-z0-9_-]+)*)`

type pattern struct {
	kind string
	re   *regexp.Regexp
	// goOnly patterns run on .go files, webOnly on everything else.
	goOnly, webOnly bool
}

var patterns = []pattern{
	{kind: "element", webOnly: true, re: regexp.MustCompile(`<glossa-(?:text|rich|plural|select)\b[^>]*?\b(?:key|message|id)\s*=\s*["']` + keyRE + `["']`)},
	{kind: "element", webOnly: true, re: regexp.MustCompile(`<GlossaText\b[^>]*?\bid\s*=\s*["']` + keyRE + `["']`)},
	{kind: "t", webOnly: true, re: regexp.MustCompile(`(?:^|[^A-Za-z0-9_$])\$?t\(\s*["'` + "`" + `]` + keyRE + `["'` + "`" + `]`)},
	{kind: "go", goOnly: true, re: regexp.MustCompile(`\.T\(\s*[^,()"]+,\s*"` + keyRE + `"`)},
	{kind: "go", goOnly: true, re: regexp.MustCompile(`\.T\(\s*"` + keyRE + `"`)},
	{kind: "template", re: regexp.MustCompile(`\{\{-?\s*td?\s+"` + keyRE + `"`)},
}

var (
	tsAccessor = regexp.MustCompile(`([A-Za-z_$][\w$]*)((?:\.[A-Za-z_$][\w$]*)+)\s*\(`)
	goAccessor = regexp.MustCompile(`\.([A-Z][A-Za-z0-9_]*)\(`)
)

// skipDirs are never scanned.
var skipDirs = map[string]bool{"node_modules": true, "dist": true, "vendor": true, "build": true, "coverage": true}

// Scan walks root and returns the usages in files that match an include
// glob and no exclude glob, sorted by key, file and line, and the number
// of files scanned.
func Scan(root string, opts Options) ([]Usage, int, error) {
	include, err := compileAll(opts.Include)
	if err != nil {
		return nil, 0, err
	}
	exclude, err := compileAll(opts.Exclude)
	if err != nil {
		return nil, 0, err
	}
	var (
		usages []Usage
		files  int
	)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()]) {
				return filepath.SkipDir
			}
			return nil
		}
		if !matchAny(include, rel) || matchAny(exclude, rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files++
		usages = append(usages, scanFile(rel, data, opts.Accessors)...)
		return nil
	})
	sort.Slice(usages, func(i, j int) bool {
		a, b := usages[i], usages[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return usages, files, err
}

func compileAll(patterns []string) ([]*Glob, error) {
	var out []*Glob
	for _, p := range patterns {
		g, err := CompileGlob(p)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func matchAny(gs []*Glob, path string) bool {
	for _, g := range gs {
		if g.Match(path) {
			return true
		}
	}
	return false
}

// scanFile finds usages line by line.
func scanFile(file string, data []byte, acc Accessors) []Usage {
	isGo := strings.HasSuffix(file, ".go")
	var out []Usage
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	seen := map[[2]int]bool{} // (line, column) already reported
	add := func(key, kind string, col int) {
		pos := [2]int{line, col}
		if seen[pos] {
			return
		}
		seen[pos] = true
		out = append(out, Usage{Key: key, File: file, Line: line, Column: col, Kind: kind})
	}
	for sc.Scan() {
		line++
		text := sc.Text()
		for _, p := range patterns {
			if (p.goOnly && !isGo) || (p.webOnly && isGo) {
				continue
			}
			for _, m := range p.re.FindAllStringSubmatchIndex(text, -1) {
				add(text[m[2]:m[3]], p.kind, m[2]+1)
			}
		}
		if isGo {
			for _, m := range goAccessor.FindAllStringSubmatchIndex(text, -1) {
				if key, ok := acc.Go[text[m[2]:m[3]]]; ok {
					add(key, "typed", m[2]+1)
				}
			}
			continue
		}
		for _, m := range tsAccessor.FindAllStringSubmatchIndex(text, -1) {
			receiver, path := text[m[2]:m[3]], strings.TrimPrefix(text[m[4]:m[5]], ".")
			key, ok := acc.TS[path]
			// A one-segment path (title) is too common a method name to
			// trust unless it's called on `messages`.
			if !ok || (!strings.Contains(path, ".") && receiver != "messages") {
				continue
			}
			add(key, "typed", m[4]+2)
		}
	}
	return out
}
