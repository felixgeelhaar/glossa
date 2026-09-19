// Package formatstest generates random exchange-model values for the
// converters' round-trip property tests, and compares them.
package formatstest

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/formats"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
)

// Gen draws random values from a seeded source, so failures reproduce.
type Gen struct {
	R *rand.Rand
	// Control allows characters XML can't carry (U+0001, U+FFFE) in text.
	Control bool
	// Complex allows select messages and declarations.
	Complex bool
}

// New returns a generator seeded with seed.
func New(seed uint64) *Gen {
	return &Gen{R: rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)), Complex: true}
}

var textAtoms = []string{
	"Hello", "world", " ", "  ", "\n", "\t", "{", "}", "\\", "|", ".", ".input", "@", "#", "'", "''",
	"&", "<b>", "</b>", "\"", "ü", "日本語", "🙂", "\u202e", "\r\n", "%s", "%d", "$", "*",
}

// Text returns random text built from syntax-heavy atoms.
func (g *Gen) Text() string {
	var b strings.Builder
	for range g.R.IntN(5) {
		b.WriteString(textAtoms[g.R.IntN(len(textAtoms))])
	}
	if g.Control && g.R.IntN(8) == 0 {
		b.WriteString([]string{"\x01", "\x1f", "\uFFFE", "\x0b"}[g.R.IntN(4)])
	}
	return b.String()
}

// Word returns a short identifier-like word.
func (g *Gen) Word() string {
	return []string{"name", "count", "total", "user", "b", "i", "link", "br", "x"}[g.R.IntN(9)]
}

func (g *Gen) element() mf.PatternElement {
	switch g.R.IntN(7) {
	case 0:
		return mf.Expression{Arg: mf.VariableRef{Name: g.Word()}, Function: &mf.FunctionRef{Name: "number"}}
	case 1:
		return mf.Expression{Arg: mf.Literal{Value: g.Text()}}
	case 2:
		return mf.Markup{Kind: mf.MarkupOpen, Name: g.Word()}
	case 3:
		return mf.Markup{Kind: mf.MarkupClose, Name: g.Word()}
	case 4:
		return mf.Markup{Kind: mf.MarkupStandalone, Name: "br"}
	default:
		return mf.Expression{Arg: mf.VariableRef{Name: g.Word()}}
	}
}

// Pattern returns a random simple pattern.
func (g *Gen) Pattern() mf.Pattern {
	var p mf.Pattern
	for range g.R.IntN(6) {
		if g.R.IntN(2) == 0 {
			p = append(p, mf.Text(g.Text()))
		} else {
			p = append(p, g.element())
		}
	}
	return p
}

// Message returns a random valid message.
func (g *Gen) Message() mf.Message {
	if !g.Complex || g.R.IntN(4) != 0 {
		return formats.PatternMessage(g.Pattern())
	}
	n := mf.Declaration{Type: mf.InputDeclaration, Name: "n",
		Value: mf.Expression{Arg: mf.VariableRef{Name: "n"}, Function: &mf.FunctionRef{Name: "number"}}}
	if g.R.IntN(3) == 0 {
		return mf.Message{Type: mf.PatternMessageType, Declarations: []mf.Declaration{n}, Pattern: g.Pattern()}
	}
	return mf.Message{
		Type: mf.SelectMessageType, Declarations: []mf.Declaration{n},
		Selectors: []mf.VariableRef{{Name: "n"}},
		Variants: []mf.Variant{
			{Keys: []mf.VariantKey{{Value: "one"}}, Value: g.Pattern()},
			{Keys: []mf.VariantKey{{Catchall: true}}, Value: g.Pattern()},
		},
	}
}

// Content returns random content in canonical form.
func (g *Gen) Content(t testing.TB) mfcontent.Content {
	t.Helper()
	c, err := formats.FromModel(g.Message())
	if err != nil {
		t.Fatalf("generated message: %v", err)
	}
	return c
}

// PlainContent returns literal text content.
func (g *Gen) PlainContent(t testing.TB) mfcontent.Content {
	t.Helper()
	c, err := formats.PlainText(g.Text())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Pick returns one of opts.
func Pick[T any](g *Gen, opts ...T) T { return opts[g.R.IntN(len(opts))] }

// Key returns a message key unique by i.
func (g *Gen) Key(i int) string {
	return fmt.Sprintf("%s.%s_%d", g.Word(), g.Word(), i)
}

// SameContent fails t when a and b are different messages.
func SameContent(t testing.TB, what string, a, b mfcontent.Content) {
	t.Helper()
	switch {
	case a.IsZero() != b.IsZero():
		t.Errorf("%s: zero = %v, want %v", what, b.IsZero(), a.IsZero())
	case !a.IsZero() && !a.SameModel(b):
		t.Errorf("%s differs:\n got %s\nwant %s", what, b.ModelJSON(), a.ModelJSON())
	}
}
