/// Glossa's Dart runtime: the delivery contract in `runtimes/SPEC.md`,
/// implemented in pure Dart so it works on the VM, in AOT and on the web.
///
/// Wave 1 covers locale identity and negotiation (SPEC §4), message
/// resolution over the fallback graph (§4.3), the MessageFormat 2
/// interpreter over the precompiled data model (§5), `explain()` and the
/// error channel (§6). The loader, signature verification and the Flutter
/// widgets are waves 2 and 3 — see `README.md`.
///
/// ```dart
/// final catalog = Catalog.fromRelease(
///   manifest: Manifest.decode(manifestBytes),
///   artifacts: {sha256: artifactBytes},
/// );
/// final t = catalog.forLocales(['de-AT']);
/// print(t.t('cart.checkout')); // Zur Kassa
/// print(t.explain('cart.checkout').chain); // [de-AT, de, en]
/// ```
library;

export 'src/catalog.dart'
    show Catalog, Explanation, Localizer, Outcome, Source, Step;
export 'src/errors.dart' show ErrorChannel, ErrorType, GlossaError;
export 'src/format.dart'
    show FormatOptions, MessageFormatError, formatMessage, formatToParts;
export 'src/functions.dart'
    show
        FunctionContext,
        FunctionError,
        MessageFunction,
        MessageValue,
        builtins,
        pluralCategory;
export 'src/locale.dart'
    show
        Direction,
        acceptLanguage,
        canonicalizeLocale,
        canonicalizeLocales,
        directionOf,
        fallbackChain,
        lookupLocale,
        resolveLocales,
        rtlScripts,
        truncations;
export 'src/manifest.dart'
    show
        Artifact,
        LocaleEntry,
        Manifest,
        ReleaseRef,
        SchemaException,
        artifactSchema,
        manifestSchema;
export 'src/model.dart'
    show
        CatchallKey,
        Declaration,
        Expression,
        FunctionRef,
        InputDeclaration,
        Literal,
        LocalDeclaration,
        Markup,
        MarkupKind,
        Message,
        MessageModelException,
        Operand,
        Pattern,
        PatternElement,
        PatternMessage,
        SelectMessage,
        TextElement,
        VariableRef,
        Variant,
        VariantKey;
export 'src/parts.dart'
    show
        BidiIsolationPart,
        FallbackPart,
        MarkupPart,
        Part,
        TextPart,
        ValuePart,
        partsToString;
export 'src/subtags.g.dart' show subtagRegistryDate;
