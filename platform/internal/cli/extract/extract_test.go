package extract_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/extract"
)

func TestGlob(t *testing.T) {
	for _, tc := range []struct {
		pattern, path string
		want          bool
	}{
		{"**/*.{ts,vue}", "src/a/b.vue", true},
		{"**/*.{ts,vue}", "b.ts", true},
		{"**/*.{ts,vue}", "src/b.tsx", false},
		{"src/*.go", "src/a.go", true},
		{"src/*.go", "src/x/a.go", false},
		{"**/*_test.go", "a/b_test.go", true},
		{"**/*.test.*", "src/x.test.ts", true},
		{"src/**", "src/a/b/c.ts", true},
		{"file?.go", "file1.go", true},
		{"[ab].ts", "a.ts", true},
		{"[!ab].ts", "a.ts", false},
	} {
		g, err := extract.CompileGlob(tc.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := g.Match(tc.path); got != tc.want {
			t.Errorf("%s ~ %s = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func write(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFindsEveryUsageKind(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/cart.astro", `<glossa-text key="cart.checkout">Zur Kasse</glossa-text>
<glossa-plural message='cart.items' count="3"></glossa-plural>`)
	write(t, root, "src/App.vue", `<template>
  <GlossaText id="terms.hint">Lies die AGB.</GlossaText>
  <p>{{ $t("cart.items", { count: 3 }) }} {{ t('athlete.greeting', { name }) }}</p>
</template>
<script setup lang="ts">
const m = useTypedMessages();
m.checkout.paymentFailed({ reason });
messages.title();
other.title();
format("not.a.message");
</script>`)
	write(t, root, "internal/mail/mail.go", `package mail

func render() {
	_ = client.T(ctx, "email.welcome.subject", glossa.Args{"name": n})
	_ = loc.T("email.welcome.body", nil)
	_ = msg.For(loc).CheckoutPay(12.5)
}`)
	write(t, root, "templates/welcome.html", `<h1>{{t "email.welcome.title" "name" .Name}}</h1>`)
	write(t, root, "node_modules/x/index.ts", `t("vendored.key")`)
	write(t, root, "src/cart.test.ts", `t("test.key")`)

	acc := extract.Accessors{
		TS: map[string]string{"checkout.paymentFailed": "checkout.payment_failed", "title": "title"},
		Go: map[string]string{"CheckoutPay": "checkout.pay"},
	}
	usages, files, err := extract.Scan(root, extract.Options{
		Include:   []string{"**/*.{ts,vue,astro,html,go}"},
		Exclude:   []string{"**/*.test.*"},
		Accessors: acc,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range usages {
		got = append(got, fmt.Sprintf("%s %s:%d %s", u.Key, u.File, u.Line, u.Kind))
	}
	want := []string{
		"athlete.greeting src/App.vue:3 t",
		"cart.checkout src/cart.astro:1 element",
		"cart.items src/App.vue:3 t",
		"cart.items src/cart.astro:2 element",
		"checkout.pay internal/mail/mail.go:6 typed",
		"checkout.payment_failed src/App.vue:7 typed",
		"email.welcome.body internal/mail/mail.go:5 go",
		"email.welcome.subject internal/mail/mail.go:4 go",
		"email.welcome.title templates/welcome.html:1 template",
		"terms.hint src/App.vue:2 element",
		"title src/App.vue:8 typed",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("usages:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if files != 4 {
		t.Errorf("files = %d, want 4 (vendored and test files skipped)", files)
	}
}
