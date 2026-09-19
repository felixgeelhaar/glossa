package codegen_test

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/codegen"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// catalog is the source the golden files are generated from.
var catalog = map[string]string{
	"cart.checkout":           "Zur Kasse",
	"cart.items":              "{count, plural, one {# item} other {# items}}",
	"checkout.pay":            "Pay {amount, number, ::currency/EUR}",
	"athlete.greeting":        "{gender, select, female {Hi {name}} other {Hello {name}}}",
	"order.shipped":           "Shipped on {date, date, short} at {time, time, short}",
	"checkout.payment-failed": "Payment failed: {reason}",
	"checkout.payment_failed": "Collides with payment-failed",
	"legal.type":              "{type} */ with a comment breaker",
	"nav.home":                "Home",
	"nav.home.title":          "Home title",
}

func entries(t *testing.T) []codegen.Entry {
	t.Helper()
	var out []codegen.Entry
	for k, text := range catalog {
		m, err := mf.ParseMF1(text, "en")
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		out = append(out, codegen.Entry{Key: k, Text: text, Arguments: mf.Arguments(m)})
	}
	return out
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/cli/codegen -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file (go test ./internal/cli/codegen -update):\n%s", name, got)
	}
}

func TestTypeScriptGolden(t *testing.T) {
	ts, warnings := codegen.TypeScript(entries(t), codegen.TSOptions{Source: "locales/en.json"})
	golden(t, "messages.ts", ts)
	golden(t, "glossa-vue.ts", codegen.Vue("messages.ts"))
	var keys []string
	for _, w := range warnings {
		keys = append(keys, w.Key)
	}
	if strings.Join(keys, ",") != "checkout.payment_failed,nav.home" {
		t.Errorf("warnings = %+v", warnings)
	}
}

func TestGoGolden(t *testing.T) {
	src, warnings, err := codegen.Go(entries(t), codegen.GoOptions{Package: "msg", Runtime: "github.com/felixgeelhaar/glossa/runtimes/go", Source: "locales/en.json"})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "messages.go.txt", src)
	if len(warnings) != 1 || warnings[0].Key != "checkout.payment_failed" {
		t.Errorf("warnings = %+v", warnings)
	}
}

func TestNamesMapAccessorsBackToKeys(t *testing.T) {
	keys := []string{"checkout.pay", "checkout.payment_failed", "cart.items"}
	ts := codegen.TSNames(keys)
	if ts["checkout.paymentFailed"] != "checkout.payment_failed" || ts["cart.items"] != "cart.items" {
		t.Errorf("TSNames = %v", ts)
	}
	gn := codegen.GoNames(keys)
	if gn["CheckoutPaymentFailed"] != "checkout.payment_failed" || gn["CartItems"] != "cart.items" {
		t.Errorf("GoNames = %v", gn)
	}
}

// repoRoot is the monorepo root (it holds pnpm-workspace.yaml).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("not inside the glossa repository")
		}
		dir = parent
	}
}

// TestGeneratedTypeScriptTypeChecks compiles the generated module and the
// Vue registration against the real @glossa/vue sources with tsc, plus a
// consumer whose @ts-expect-error lines prove missing and mistyped
// arguments fail at compile time.
func TestGeneratedTypeScriptTypeChecks(t *testing.T) {
	root := repoRoot(t)
	tsc := filepath.Join(root, "runtimes", "js", "vue", "node_modules", ".bin", "tsc")
	if _, err := os.Stat(tsc); err != nil {
		t.Skip("tsc not installed: run `pnpm install` at the repository root to type-check generated TypeScript")
	}
	dir := t.TempDir()
	ts, _ := codegen.TypeScript(entries(t), codegen.TSOptions{Source: "locales/en.json"})
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("messages.ts", string(ts))
	write("glossa-vue.ts", string(codegen.Vue("messages.ts")))
	write("consumer.ts", consumerTS)
	js := filepath.Join(root, "runtimes", "js")
	write("tsconfig.json", `{
  "compilerOptions": {
    "target": "ES2022", "lib": ["ES2022", "DOM"], "module": "ESNext", "moduleResolution": "Bundler",
    "strict": true, "noUncheckedIndexedAccess": true, "noEmit": true, "skipLibCheck": true, "types": [],
    "isolatedModules": true, "esModuleInterop": true,
    "paths": {
      "@glossa/vue": ["`+filepath.ToSlash(filepath.Join(js, "vue", "src", "index.ts"))+`"],
      "@glossa/runtime": ["`+filepath.ToSlash(filepath.Join(js, "runtime", "src", "index.ts"))+`"],
      "@glossa/elements/parts": ["`+filepath.ToSlash(filepath.Join(js, "elements", "src", "parts.ts"))+`"]
    }
  },
  "files": ["messages.ts", "glossa-vue.ts", "consumer.ts"]
}`)
	cmd := exec.Command(tsc, "-p", filepath.Join(dir, "tsconfig.json"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tsc: %v\n%s", err, out)
	}
}

const consumerTS = `import { createMessages, type Messages } from "./messages.js";
import { useTypedMessages } from "./glossa-vue.js";
import { useGlossa } from "@glossa/vue";

const t = (id: string, values: Record<string, unknown>): string => id + JSON.stringify(values);
const messages = createMessages(t);

export const ok: string[] = [
  messages.checkout.pay({ amount: 12.5 }),
  messages.cart.items({ count: 3 }),
  messages.cart.checkout(),
  messages.athlete.greeting({ gender: "female", name: "Lina" }),
  messages.athlete.greeting({ gender: "nonbinary", name: "Kim" }),
  messages.order.shipped({ date: new Date(), time: Date.now() }),
  messages.checkout.paymentFailed({ reason: "declined" }),
  messages.legal.type({ type: "x" }),
];

// @ts-expect-error amount is required
messages.checkout.pay({});
// @ts-expect-error amount is a number
messages.checkout.pay({ amount: "12" });
// @ts-expect-error no such message
messages.checkout.refund();
// @ts-expect-error cart.checkout takes no values
messages.cart.checkout({ extra: 1 });

// The registration types @glossa/vue's own t().
export function inSetup(): string {
  const m = useTypedMessages();
  const { t: typed } = useGlossa();
  // @ts-expect-error unknown ID once GlossaRegister is augmented
  typed("checkout.refund");
  const key: keyof Messages = "cart.items";
  return m.cart.items({ count: 1 }) + typed(key, { count: 2 });
}
`

// TestGeneratedGoCompiles builds the generated file against the real Go
// runtime in a scratch module.
func TestGeneratedGoCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a scratch module")
	}
	root := repoRoot(t)
	src, _, err := codegen.Go(entries(t), codegen.GoOptions{Package: "msg", Runtime: "github.com/felixgeelhaar/glossa/runtimes/go", Source: "locales/en.json"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runtimeDir := filepath.Join(root, "runtimes", "go")
	gomod, err := os.ReadFile(filepath.Join(runtimeDir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	gosum, _ := os.ReadFile(filepath.Join(runtimeDir, "go.sum"))
	// The runtime's own requirements, plus replaces to the repo checkout.
	mod := strings.Replace(string(gomod), "module github.com/felixgeelhaar/glossa/runtimes/go", "module scratch", 1)
	mod = strings.ReplaceAll(mod, "=> ../../messageformat", "=> "+filepath.Join(root, "messageformat"))
	mod += "\nrequire github.com/felixgeelhaar/glossa/runtimes/go v0.0.0\n" +
		"replace github.com/felixgeelhaar/glossa/runtimes/go => " + runtimeDir + "\n"
	write("go.mod", mod)
	write("go.sum", string(gosum))
	write("msg/messages.go", string(src))
	write("main.go", `package main

import (
	"context"
	"fmt"
	"time"

	glossa "github.com/felixgeelhaar/glossa/runtimes/go"

	"scratch/msg"
)

func main() {
	client, err := glossa.New(glossa.Config{EdgeURL: "http://127.0.0.1:1", DeliveryKey: "pk", Environment: "production"})
	if err != nil {
		panic(err)
	}
	defer client.Close()
	m := msg.For(client.For("en"))
	fmt.Println(m.CheckoutPay(12.5), m.CartItems(3), m.CartCheckout(), m.AthleteGreeting("female", "Lina"),
		m.OrderShipped(time.Now(), time.Now()), msg.IDCheckoutPay)
	fmt.Println(msg.FromContext(context.Background(), client).CheckoutPaymentFailed("declined"))
}
`)
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Go doesn't compile: %v\n%s\n%s", err, out, src)
	}
}
