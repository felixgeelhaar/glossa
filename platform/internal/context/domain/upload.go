package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

// UsagesSchema is the document type every collector writes.
const UsagesSchema = "glossa.usages/v1"

// Limits of a build (RFC 0004 §10).
const (
	// MaxUploadBytes bounds a usages document.
	MaxUploadBytes = 20 << 20
	// MaxUsagesPerBuild bounds the usages of one build.
	MaxUsagesPerBuild = 100_000
)

// Usage limits, in characters.
const (
	MaxKeyLen       = 200
	MaxFileLen      = 1024
	MaxComponentLen = 200
	MaxRouteLen     = 500
)

// UsagesDocument is a glossa.usages/v1 document as collectors write it
// (RFC 0004 §2.2; the JSON Schema is runtimes/testdata/usages). Field
// names are the published contract.
type UsagesDocument struct {
	Schema      string          `json:"schema"`
	Application string          `json:"application"`
	Commit      string          `json:"commit"`
	Branch      string          `json:"branch"`
	Tool        DocumentTool    `json:"tool"`
	Usages      []DocumentUsage `json:"usages"`
}

// DocumentTool is the document's tool.
type DocumentTool struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// DocumentUsage is one entry of the document's usages. Column,
// component and route are optional.
type DocumentUsage struct {
	Key       string `json:"key"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Column    int    `json:"column,omitempty"`
	Component string `json:"component,omitempty"`
	Route     string `json:"route,omitempty"`
	Kind      string `json:"kind"`
}

// Usage is a message's key at a location in a build (RFC 0004 §2.2).
// The message ID is resolved at ingest and kept apart (a key the
// catalog doesn't know has none).
type Usage struct {
	Key  string
	File string
	// Line is 1-based; Column is 1-based, 0 when unknown.
	Line   int
	Column int
	// Component is the SFC, .astro file or enclosing component or
	// function that makes the call; Route the route pattern where the
	// collector knows it. Either may be empty.
	Component string
	Route     string
	// Kind is the call shape the collector recognized (t, element, go,
	// template, typed …).
	Kind string
}

var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func (u Usage) validate() error {
	switch {
	case !textWithin(u.Key, 1, MaxKeyLen):
		return fmt.Errorf("key must be 1–%d characters", MaxKeyLen)
	case !textWithin(u.File, 1, MaxFileLen):
		return fmt.Errorf("file must be 1–%d characters", MaxFileLen)
	case u.Line < 1:
		return errors.New("line must be at least 1")
	case u.Column < 0:
		return errors.New("column must be at least 1")
	case !textWithin(u.Component, 0, MaxComponentLen):
		return fmt.Errorf("component must be at most %d characters", MaxComponentLen)
	case !textWithin(u.Route, 0, MaxRouteLen):
		return fmt.Errorf("route must be at most %d characters", MaxRouteLen)
	case !kindPattern.MatchString(u.Kind):
		return errors.New("kind must be a lowercase word (t, element, go, template, typed …)")
	}
	return nil
}

// Upload is a validated usages document.
type Upload struct {
	// Application is the application's slug in the project.
	Application string
	Commit      Commit
	Branch      Branch
	Tool        Tool
	Usages      []Usage
	// Digest is the SHA-256 of the document's bytes.
	Digest Digest
}

// ParseUpload reads and validates a glossa.usages/v1 document. Unknown
// fields are ignored so collectors can add optional ones within v1.
func ParseUpload(raw []byte) (Upload, error) {
	if len(raw) > MaxUploadBytes {
		return Upload{}, ErrUploadTooLarge
	}
	var doc UsagesDocument
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&doc); err != nil {
		return Upload{}, fmt.Errorf("%w: %v", ErrInvalidUpload, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Upload{}, fmt.Errorf("%w: data after the document", ErrInvalidUpload)
	}
	up, err := doc.validate()
	if err != nil {
		return Upload{}, err
	}
	up.Digest = DigestOf(raw)
	return up, nil
}

func (doc UsagesDocument) validate() (Upload, error) {
	if doc.Schema != UsagesSchema {
		return Upload{}, fmt.Errorf("%w: schema must be %q", ErrInvalidUpload, UsagesSchema)
	}
	if !textWithin(doc.Application, 1, 64) {
		return Upload{}, fmt.Errorf("%w: application must name an application", ErrInvalidUpload)
	}
	commit, err := ParseCommit(doc.Commit)
	if err != nil {
		return Upload{}, fmt.Errorf("%w: %w", ErrInvalidUpload, err)
	}
	branch, err := ParseBranch(doc.Branch)
	if err != nil {
		return Upload{}, fmt.Errorf("%w: %w", ErrInvalidUpload, err)
	}
	tool := Tool{Name: doc.Tool.Name, Version: doc.Tool.Version}
	if err := tool.validate(); err != nil {
		return Upload{}, err
	}
	if len(doc.Usages) > MaxUsagesPerBuild {
		return Upload{}, ErrTooManyUsages
	}
	usages := make([]Usage, len(doc.Usages))
	for i, d := range doc.Usages {
		u := Usage(d)
		if err := u.validate(); err != nil {
			return Upload{}, fmt.Errorf("%w: usages[%d]: %v", ErrInvalidUpload, i, err)
		}
		usages[i] = u
	}
	return Upload{Application: doc.Application, Commit: commit, Branch: branch, Tool: tool, Usages: usages}, nil
}
