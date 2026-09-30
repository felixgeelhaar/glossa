//go:build system

package m4_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture repository (RFC 0005 §12.1).
//
// testdata/repo is what M4 adds to the M3 fixture application: a
// `.github/workflows` job that runs `glossa check`, a capture plan for
// the checkout route in German and Japanese, and the stylesheet that
// gives the checkout page its fixed-width pay button. The application
// itself — 150 messages of Brotwerk's shop over eight routes, its Vite
// build and its preview bundle — is M3's, reused rather than copied,
// because §12.1 says to reuse it and a second 150-message fixture would
// be a second thing to keep true.
//
// The repository is materialized into a temporary directory: the M3
// tree first, then testdata/repo over it, then the seeded defects. The
// committed M3 fixture is never written to.

// m3App is the M3 fixture application, the repository's base.
const m3App = "../m3/testdata/app"

// overlay is what M4 adds, checked in beside this file.
const overlay = "testdata/repo"

// Commits. The base commit is what `main` holds; the head commit is the
// pull request's, and the one §12.3 has the CLI and the check run
// agree about.
const (
	baseCommit = "1f4c9a2e57b0d8364ca1e7f90b52d6183ac4e0f7"
	headCommit = "6b28d05fa1c73e94802df615ab3792ce4708d1b6"
	// laterCommit opens the second pull request of §12.4, the one that
	// grades against policy v4.
	laterCommit = "9e35c7104bd2af68135e0c9b7f24ad80516e3b92"
)

const (
	defaultBranch = "main"
	prBranch      = "feature/checkout-copy"
	prNumber      = 11
	// laterBranch is the pull request opened after policy v4 is saved.
	laterBranch   = "feature/pickup-copy"
	laterPRNumber = 12
	application   = "shop"
)

// The keys the seeds touch. Each one is a case §12.2 names.
const (
	// keyArgument is the French translation that ends up missing an
	// argument the German source gained.
	keyArgument = "checkout.payment.activity"
	// keyMarkup is the Japanese translation that ends up carrying
	// markup the German source lost.
	keyMarkup = "checkout.roll.help"
	// keyButton is the checkout page's pay button: what clips in
	// Japanese and not in German.
	keyButton = "checkout.crust.label"
	// keyForbidden is the legal-namespace message whose French uses a
	// forbidden term — an error, by policy v3.
	keyForbidden = "legal.privacy.notice"
	// keyTermMissing is the Spanish translation that does not use the
	// termbase's preferred word — a warning, everywhere else.
	keyTermMissing = "cart.basket.title"
	// keyMaxLength carries a max_length the French translation exceeds.
	keyMaxLength = "checkout.bagel.title"
	// keyUnknown is used by the build and is in no catalog.
	keyUnknown     = "checkout.pickup.reminder"
	unknownFile    = "src/pages/CheckoutPage.vue"
	unknownLine    = 31
	unknownCompont = "CheckoutPage"
)

// missingFrench are the three French translations the pull request does
// not ship.
var missingFrench = []string{"cart.flour.title", "cart.shipping.title", "cart.tray.title"}

// outdatedJapanese are the two whose German source moves under them.
var outdatedJapanese = []string{"home.allergen.title", "products.bun.title"}

// The seeded texts. The base column is what the repository holds at
// baseCommit; the head column what it holds at headCommit.
const (
	argumentSourceBase = "{$name} hat Zahlung geändert."
	argumentSourceHead = "{$name} hat Zahlung um {$amount} geändert."
	argumentFrench     = "{$name} a modifier paiement." // unchanged: the one that breaks

	markupSourceBase = "Mehr über Brötchen lesen Sie im {#link}Hilfebereich{/link}."
	markupSourceHead = "Mehr über Brötchen lesen Sie in der Hilfe."
	markupJapanese   = "ロールパンについては{#link}ヘルプ{/link}をご覧ください。" // unchanged: the one that keeps the link

	privacySourceDE = "Lesen Sie unsere Hinweise zum Datenschutz."
	privacyEN       = "Read our notes on data protection."
	privacyES       = "Lea nuestras notas sobre protección de datos."
	privacyJA       = "データ保護に関する注意事項をお読みください。"
	// privacyFR uses the concept's forbidden French term.
	privacyFR = "Consultez nos notes sur la vie privée."

	// maxLengthFrench is longer than the max_length the test sets on
	// keyMaxLength.
	maxLengthFrench = "ajouter un bagel au panier maintenant"
)

// The termbase §12.2's two terminology cases need.
const (
	termConceptPrivacy = "Datenschutz"
	termForbiddenFR    = "vie privée"
	termPreferredFR    = "confidentialité"
	termConceptBasket  = "Warenkorb"
	termPreferredES    = "carrito"
)

// maxLength is what the test puts on keyMaxLength.
const maxLength = 20

// catalogs is one commit's catalogs: locale → key → MF2 text.
type catalogs map[string]map[string]string

// repo is the materialized fixture repository.
type repo struct {
	dir string
	// base and head are the catalogs at the two commits.
	base, head catalogs
}

// materialize writes the repository into dir: the M3 application, then
// testdata/repo over it, then the preview build's extra stylesheet.
func materialize(t *testing.T, dir string) *repo {
	t.Helper()
	if err := os.CopyFS(dir, os.DirFS(m3App)); err != nil {
		t.Fatal(err)
	}
	// os.CopyFS refuses to replace a file that is already there, and the
	// overlay's whole job is to replace the application's glossa.yaml.
	overlayOver(t, overlay, dir)
	// The preview build is the one the capture drives (M3 §12.3 proved
	// the production build carries no loader). M4 links one stylesheet
	// into it — the pay button — without touching the bundle.
	linkStylesheet(t, filepath.Join(dir, "built", "preview", "index.html"))
	copyFile(t, filepath.Join(dir, "m4.css"), filepath.Join(dir, "built", "preview", "m4.css"))

	r := &repo{dir: dir}
	r.base = readCatalogs(t, dir)
	seedBase(r.base)
	r.head = seedHead(r.base)
	r.write(t, r.base)
	seedUnknownKey(t, dir)
	return r
}

// write puts a set of catalogs into the repository's locales/.
func (r *repo) write(t *testing.T, c catalogs) {
	t.Helper()
	for locale, messages := range c {
		writeJSON(t, filepath.Join(r.dir, "locales", locale+".json"), messages)
	}
}

// locales are the repository's, source first.
var repoLocales = []string{"de", "en", "es", "fr", "ja"}

func readCatalogs(t *testing.T, dir string) catalogs {
	t.Helper()
	out := catalogs{}
	for _, l := range repoLocales {
		raw, err := os.ReadFile(filepath.Join(dir, "locales", l+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("locales/%s.json: %v", l, err)
		}
		out[l] = m
	}
	return out
}

// seedBase is what the repository holds on the default branch: the
// legal message every locale has, the three French translations that
// were never written, and nothing else wrong.
func seedBase(c catalogs) {
	c["de"][keyForbidden] = privacySourceDE
	c["en"][keyForbidden] = privacyEN
	c["es"][keyForbidden] = privacyES
	c["fr"][keyForbidden] = privacyFR
	c["ja"][keyForbidden] = privacyJA
	for _, k := range missingFrench {
		delete(c["fr"], k)
	}
	c["fr"][keyMaxLength] = maxLengthFrench
}

// seedHead is the pull request's commit: the German source moves under
// four translations, and only some of them follow it.
//
// This is how a real project breaks parity, and the only way a server
// project can: the write path parses and gates every translation
// (localization/domain.QAResult.Gate), so a translation that disagrees
// with its source can never be *written* — it can only be *left behind*
// by a source that changed. Seeding it any other way would be seeding a
// state the platform cannot reach.
func seedHead(base catalogs) catalogs {
	c := catalogs{}
	for l, m := range base {
		copyOf := make(map[string]string, len(m))
		for k, v := range m {
			copyOf[k] = v
		}
		c[l] = copyOf
	}
	// parity: the source gains {$amount}; every locale but French
	// follows it.
	c["de"][keyArgument] = argumentSourceHead
	c["en"][keyArgument] = "{$name} changed the payment by {$amount}."
	c["es"][keyArgument] = "{$name} ha cambiado el pago en {$amount}."
	c["ja"][keyArgument] = "{$name} さんが支払いを {$amount} 変更しました。"
	c["fr"][keyArgument] = argumentFrench

	// parity: the source loses its link; every locale but Japanese
	// follows it.
	c["de"][keyMarkup] = markupSourceHead
	c["en"][keyMarkup] = "Read more about rolls in the help."
	c["es"][keyMarkup] = "Más sobre panecillos en la ayuda."
	c["fr"][keyMarkup] = "En savoir plus sur les petits pains dans l’aide."
	c["ja"][keyMarkup] = markupJapanese

	// completeness: the source moves under two Japanese translations
	// and nothing else changes, so they go outdated.
	//
	// The Japanese is dropped from the catalog rather than left in it:
	// `glossa push --translations` writes every translation the files
	// hold against the source revision it finds, so a translation that
	// is still in the file is re-stamped current whatever happened to
	// its source. What goes outdated on a real project is the
	// translation the branch did not touch, and dropping the key from
	// this commit's catalog is how a fixture says "nobody touched it".
	// The server keeps what the base commit pushed.
	c["de"]["home.allergen.title"] = "Allergen jetzt hinzufügen"
	c["en"]["home.allergen.title"] = "add allergen now"
	c["es"]["home.allergen.title"] = "añadir alérgeno ahora"
	c["fr"]["home.allergen.title"] = "ajouter allergène maintenant"
	c["de"]["products.bun.title"] = "Semmel dauerhaft speichern"
	c["en"]["products.bun.title"] = "save bun for good"
	c["es"]["products.bun.title"] = "guardar bollo para siempre"
	c["fr"]["products.bun.title"] = "enregistrer boule définitivement"
	for _, k := range outdatedJapanese {
		delete(c["ja"], k)
	}
	return c
}

// seedUnknownKey adds one usage of a key no catalog has, with the file
// and the line the product's own source uses it on. That is what makes
// an unknown-key finding annotatable (RFC 0005 §2.1: the locus comes
// from Context at report time).
func seedUnknownKey(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "usages.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	usages, ok := doc["usages"].([]any)
	if !ok {
		t.Fatalf("usages.json has no usages array: %v", keysOf(doc))
	}
	doc["usages"] = append(usages, map[string]any{
		"key": keyUnknown, "file": unknownFile, "line": unknownLine,
		"column": 9, "component": unknownCompont, "route": "/kasse", "kind": "t",
	})
	doc["commit"] = baseCommit
	writeJSON(t, path, doc)
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// linkStylesheet adds M4's stylesheet to a built index.html.
func linkStylesheet(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "</head>"
	html := string(raw)
	if !strings.Contains(html, anchor) {
		t.Fatalf("%s has no </head>", path)
	}
	html = strings.Replace(html, anchor, `    <link rel="stylesheet" href="/m4.css">`+"\n  "+anchor, 1)
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
}

// overlayOver copies a tree over another, replacing what is there.
func overlayOver(t *testing.T, from, to string) {
	t.Helper()
	err := fs.WalkDir(os.DirFS(from), ".", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			return os.MkdirAll(filepath.Join(to, p), 0o755)
		}
		raw, err := os.ReadFile(filepath.Join(from, p))
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, p), raw, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	raw, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
