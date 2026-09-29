# `glossa_flutter` — the Flutter layer

`GlossaText` and everything else Flutter needs, on top of the pure-Dart
runtime in [`..`](../README.md). Designed in
[RFC 0005 §6](../../../docs/rfcs/0005-quality.md).

It is a **separate package on purpose**. `package:glossa` must not import
Flutter — that is what lets it be tested with `dart test`, compiled for the
web with `dart compile js`, and used from a server or a CLI. Declaring
Flutter as a dependency there would break `dart pub get` for everyone
without the Flutter SDK, including the `runtimes-dart` CI job. So the core
stays Flutter-free and this package is the only place `package:flutter`
appears. `flutter/` is excluded from the core's `analysis_options.yaml`
for the same reason: `dart analyze` in `runtimes/dart` must not try to
resolve Flutter.

```yaml
dependencies:
  glossa_flutter:
    path: ../glossa/runtimes/dart/flutter   # until it is published (§15.2)
```

`package:glossa_flutter/glossa_flutter.dart` re-exports the whole core, so
an application imports one library.

## What it adds

| | |
|---|---|
| `GlossaScope` | binds a `GlossaClient` to a widget subtree, subscribes to the SPEC §6 error channel, rebuilds when a release activates or the locale changes, and revalidates the manifest on app resume (SPEC §3) |
| `GlossaText` | a `Text` that renders a message by id and turns its MessageFormat 2 markup into styles instead of escaping it |
| `glossaSpan`, `GlossaTagStyles`, `GlossaTagBuilder` | formatted parts as `InlineSpan`s, under the shared safe-tag list |
| `loadBundledRelease` | the SPEC §3 bundle, read out of the app's assets |
| `Glossa` | `t`, `parts`, `span`, `explain`, `errors`, `locale`, `textDirection`, `availableLocales`, `setLocales`, `refresh` |

```dart
final glossa = GlossaClient(
  edge: 'https://edge.example.com',
  deliveryKey: 'pk_live_…',          // publishable by design (SPEC §2)
  locales: ['de-AT'],
  publicKeys: [GlossaPublicKey.parse('k_2026a', '…')],
  transport: ioTransport(),           // package:glossa/io.dart
  store: FileReleaseStore.scoped(
    await getApplicationSupportDirectory(),   // package:path_provider
    deliveryKey: 'pk_live_…',
    environment: 'production',
  ),
  bundled: await loadBundledRelease(),
);

runApp(
  GlossaScope(
    client: glossa,
    onError: (e) => logger.warning('$e'),
    child: const App(),
  ),
);

// anywhere below it
GlossaText('cart.items', values: {'n': cart.length});
GlossaScope.of(context).explain('cart.items').chain;   // [de-AT, de, en]
```

`path_provider` is deliberately **not** a dependency of this package: the
application decides where its cache lives, and a plugin dependency we
don't need costs size no one asked for (RFC 0005 §6.4).

**Configure signing keys.** SPEC §1.3 lets a runtime without public keys
skip signature verification; mobile OTA should not. Pass `publicKeys`.

## Markup, and why a translation can't add a link

MF2 markup reaches the widget as parts, and `partsToTree` in the core
turns them into the tree every runtime shares
([`runtimes/testdata/markup.json`](../../testdata/markup.json), SPEC §5).
Only names on `safeTags` become elements, and **markup options are
dropped** — a translation that writes `{#link href=|javascript:…|}` gets
its text rendered and nothing else.

`GlossaTagStyles.standard()` gives the unambiguous tags a style: `b` and
`strong` bold, `i`, `em`, `var`, `cite` and `dfn` italic, `u` and `ins`
underlined, `s` and `del` struck through, `code`, `kbd` and `samp`
monospace. `{#br/}` is a line break and `{#wbr/}` a zero-width space. The
tags that need a size or a colour (`small`, `sub`, `sup`, `mark`) carry no
default, because only the application's theme knows: give them one with
`GlossaTagStyles.standard().merge({...})`, on the scope or on one
`GlossaText`.

A tappable link is markup the **application** renders:

```dart
GlossaText(
  'legal.accept',
  builders: {
    'link': (context, children) => TextSpan(
      children: children,
      style: const TextStyle(decoration: TextDecoration.underline),
      recognizer: TapGestureRecognizer()..onTap = () => open(termsUrl),
    ),
  },
)
```

The builder gets the children and the context, and nothing from the
translation: the URL is the call site's, always.

## Shipping a release in the app

The layout is the one SPEC §3 fixes for every runtime and for
`glossa pull --release` — the edge's URL space below `/v1/{deliveryKey}/`:

```text
assets/glossa/manifest.json      the manifest, exactly as the edge serves it
assets/glossa/a/<sha256>.json    one artifact, exact bytes
```

```yaml
flutter:
  assets:
    - assets/glossa/
    - assets/glossa/a/      # Flutter's asset directories are not recursive
```

`loadBundledRelease()` reads `manifest.json` and then exactly the
artifacts that manifest names — no asset-manifest scan. An artifact the
bundle doesn't carry is skipped, so shipping a subset of the locales is a
supported choice and the loader falls through to the persisted cache or
the network for the rest. Bundled bytes are trusted like application code
(SPEC §3): they are neither re-hashed nor signature-checked.

The same directory is a valid persisted cache (`FileReleaseStore`), so a
release the app cached can be committed as its bundle unchanged.

## Typed accessors

`glossa generate` writes Dart accessors when `glossa.yaml` has
`generate.dart`:

```yaml
generate:
  dart: lib/glossa/messages.dart
  # dart_runtime: package:glossa_flutter/glossa_flutter.dart   # optional
```

```dart
final messages = Messages(GlossaScope.of(context).t);
messages.checkout.pay(amount: order.total);
GlossaText(MessageIds.checkoutPay, values: {'amount': order.total});
```

The generated file carries `// dart format off`: it is laid out by the Go
generator, which cannot run `dart_style`, so `dart format` must leave it
alone — otherwise every run of it would make `glossa generate --check`
fail.

## Running the tests

There is **no CI job for this package yet** (RFC 0005 §13, wave 4). Run it
locally, with the Flutter stable channel:

```sh
cd runtimes/dart/flutter
flutter pub get
dart format --output=none --set-exit-if-changed .
flutter analyze --fatal-infos
flutter test
```

The core's own suites stay on the plain Dart SDK, from `runtimes/dart`:
`dart analyze --fatal-infos` and `dart test`. Neither suite touches the
network.

A widget test's body runs against a fake clock, so the loader's first pass
has to happen inside `tester.runAsync` — `test/glossa_text_test.dart`'s
`offlineClient` does that, and every test here goes through it.

## Not here yet

- The §6.4 size and startup budgets, measured and enforced (wave 4).
- An IndexedDB `ReleaseStore` for Flutter web; a web build keeps its
  release in memory and reloads it from the edge on every start.
