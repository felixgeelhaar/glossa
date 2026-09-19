package jsoncat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/formats"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
)

// ReadOptions configure Read.
type ReadOptions struct {
	// Locale is the file's locale (required).
	Locale bcp47.Tag
	// SourceLocale is the catalog's source locale. When it is zero or
	// equal to Locale the file holds source messages; otherwise it holds
	// translations into Locale.
	SourceLocale bcp47.Tag
	// Namespace is given to every entry.
	Namespace string
	// Syntax of the messages; "" means MF1.
	Syntax mfcontent.Syntax
	// State of translations read; "" means needs_review (a JSON file
	// says nothing about review).
	State  formats.State
	Limits formats.Limits
}

// Read reads a JSON catalog.
func Read(r io.Reader, opts ReadOptions) (formats.Catalog, error) {
	if opts.Locale.IsZero() {
		return formats.Catalog{}, &formats.Error{Format: format, Err: formats.Invalidf("ReadOptions.Locale is required")}
	}
	limits := opts.Limits.WithDefaults(formats.DefaultLimits)
	data, err := formats.ReadAll(r, limits.MaxBytes)
	if err != nil {
		return formats.Catalog{}, &formats.Error{Format: format, Err: err}
	}
	p := newParser(data, opts, limits)
	if err := p.document(); err != nil {
		return formats.Catalog{}, err
	}
	return p.cat, nil
}

type parser struct {
	data   []byte
	dec    *json.Decoder
	opts   ReadOptions
	limits formats.Limits
	cat    formats.Catalog
	keys   map[string]bool
	target bool
}

func newParser(data []byte, opts ReadOptions, limits formats.Limits) *parser {
	if opts.Syntax == "" {
		opts.Syntax = mfcontent.MF1
	}
	if opts.State == "" {
		opts.State = formats.StateNeedsReview
	}
	source := opts.SourceLocale
	if source.IsZero() {
		source = opts.Locale
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return &parser{
		data: data, dec: dec, opts: opts, limits: limits, keys: map[string]bool{},
		cat: formats.Catalog{SourceLocale: source}, target: source != opts.Locale,
	}
}

func (p *parser) document() error {
	tok, err := p.dec.Token()
	if err != nil {
		return p.syntaxErr(err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return p.errAt(0, "", formats.Invalidf("a catalog is a JSON object"))
	}
	if err := p.object("", 1); err != nil {
		return err
	}
	end := p.dec.InputOffset()
	if _, err := p.dec.Token(); err != io.EOF {
		return p.errAt(end, "", formats.Invalidf("data after the catalog object"))
	}
	return nil
}

// object reads the members of an object whose '{' was consumed.
func (p *parser) object(prefix string, depth int) error {
	if depth > formats.MaxDepth {
		return p.errAt(p.dec.InputOffset(), prefix, formats.Invalidf("objects nest deeper than %d levels", formats.MaxDepth))
	}
	members := map[string]bool{}
	for p.dec.More() {
		at := p.dec.InputOffset()
		tok, err := p.dec.Token()
		if err != nil {
			return p.syntaxErr(err)
		}
		name := tok.(string)
		key := join(prefix, name)
		if name == "" || members[name] {
			return p.errAt(at, key, formats.Invalidf("empty or duplicate key"))
		}
		members[name] = true
		if err := p.member(key, depth); err != nil {
			return err
		}
	}
	_, err := p.dec.Token()
	return p.syntaxErr(err)
}

func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func (p *parser) member(key string, depth int) error {
	at := p.dec.InputOffset()
	tok, err := p.dec.Token()
	if err != nil {
		return p.syntaxErr(err)
	}
	switch v := tok.(type) {
	case string:
		return p.message(key, v, at)
	case json.Delim:
		if v == '{' {
			return p.object(key, depth+1)
		}
	}
	return p.errAt(at, key, formats.Invalidf("a value must be a message string or an object"))
}

func (p *parser) message(key, text string, at int64) error {
	if p.keys[key] {
		return p.errAt(at, key, formats.Invalidf("key defined twice (flat and nested)"))
	}
	if len(p.cat.Entries) >= p.limits.MaxItems {
		return p.errAt(at, key, fmt.Errorf("%w: more than %d messages", formats.ErrTooLarge, p.limits.MaxItems))
	}
	p.keys[key] = true
	c, err := formats.ParseContent(p.opts.Syntax, text, p.opts.Locale)
	if err != nil {
		return p.errAt(at, key, err)
	}
	e := formats.Entry{ID: key, Namespace: p.opts.Namespace}
	if p.target {
		e.Targets = []formats.Target{{Locale: p.opts.Locale, Content: c, State: p.opts.State}}
	} else {
		e.Source = c
	}
	p.cat.Entries = append(p.cat.Entries, e)
	return nil
}

func (p *parser) syntaxErr(err error) error {
	if err == nil {
		return nil
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return p.errAt(se.Offset, "", formats.Invalidf("JSON: %v", err))
	}
	return p.errAt(p.dec.InputOffset(), "", formats.Invalidf("JSON: %v", err))
}

// errAt reports err at a byte offset, skipping the whitespace and
// separators before the value it points at.
func (p *parser) errAt(off int64, key string, err error) error {
	off = min(max(off, 0), int64(len(p.data)))
	for off < int64(len(p.data)) && bytes.IndexByte([]byte(" \t\r\n:,"), p.data[off]) >= 0 {
		off++
	}
	line, col := position(p.data[:off])
	item := ""
	if key != "" {
		item = fmt.Sprintf("key %q", key)
	}
	return &formats.Error{Format: format, Line: line, Column: col, Item: item, Err: err}
}

func position(before []byte) (line, col int) {
	line = 1 + bytes.Count(before, []byte("\n"))
	col = 1 + len(before) - (bytes.LastIndexByte(before, '\n') + 1)
	return line, col
}
