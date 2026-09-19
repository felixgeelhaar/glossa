// Package glossa is the Go runtime of Glossa: it renders localized
// messages in Go backends — transactional email, PDFs, chat messages, CLI
// output — from releases published through glossa-edge. It implements the
// delivery and runtime contract in runtimes/SPEC.md and passes its
// conformance fixtures.
//
// # Rendering
//
// Create one [Client] per project and environment, and render with the
// locale on the context or an explicit [Localizer]:
//
//	client, err := glossa.New(glossa.Config{
//		EdgeURL:     "https://edge.glossa.dev",
//		DeliveryKey: deliveryKey,
//		Environment: "production",
//		PublicKeys:  []glossa.PublicKey{signingKey},
//		Bundled:     bundledCatalogs, // fs.FS from `glossa pull --release`
//	})
//	defer client.Close()
//
//	subject := client.T(ctx, "invoice.payment_received", glossa.Args{"amount": 42.5})
//	body := client.For(user.Locale).T("invoice.body", args, glossa.Default("Thanks!"))
//
// A message resolves through the release's fallback graph and is formatted
// with the locale it was found in, so plural rules and number formats match
// the language of the text. Rendering never panics and never returns an
// empty string: when no loaded locale has the message, the inline default
// ([Default]) or the message ID is rendered. [Localizer.Explain] shows how
// a message resolved; errors go to Config.OnError (or the log), rate
// limited, never to the caller.
//
// # Loading and reliability
//
// New restores the persisted last-good release from the cache directory,
// or the bundled catalogs when they are newer, so a service renders
// without the network. A background refresh revalidates the manifest
// every five minutes (with jitter) through retries and a circuit breaker.
// A new release is verified — Ed25519 manifest signature over the RFC 8785
// canonical form when keys are configured, and SHA-256 for every artifact
// — and only then activated, atomically. Until then the previous release
// keeps serving.
//
// # Servers
//
// [Client.Middleware] resolves each request's locale with a
// [ResolverChain] (explicit → user → organization → request metadata →
// Accept-Language → source locale), canonicalizes the proposals and
// matches them with RFC 4647 Lookup. Handlers call [Client.T] with the
// request context.
//
// # Templates
//
// [Localizer.FuncMap] provides t, td, lang and dir for html/template and
// text/template. Parse email templates once with [TemplateFuncs], then
// bind the recipient's locale on a clone:
//
//	var welcome = template.Must(template.New("welcome").Funcs(glossa.TemplateFuncs()).Parse(`
//	<html lang="{{lang}}" dir="{{dir}}">
//	  <h1>{{t "email.welcome.title" "name" .Name}}</h1>
//	  <p>{{td "email.welcome.body" "Your first loaf is on its way."}}</p>
//	</html>`))
//
//	func render(client *glossa.Client, u User) (string, error) {
//		tmpl := template.Must(welcome.Clone()) // never execute the base itself
//		var b strings.Builder
//		err := tmpl.Funcs(client.For(u.Locale).FuncMap()).Execute(&b, u)
//		return b.String(), err
//	}
//
// For CLI output, [EnvLocales] reads the user's locale from the POSIX
// environment and [BidiIsolation](false) keeps isolation marks out of the
// terminal.
package glossa
