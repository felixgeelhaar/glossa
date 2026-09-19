# @glossa/capture

Capture mode ([RFC 0004 §3.1](../../../docs/rfcs/0004-context.md)): finding
which pixels of a page belong to which message, for screenshots in Studio's
"Where it appears" pane and for the in-product editor.

This package is **loaded only in a capture or editor session**. `glossa
capture` injects it into the page it screenshots, and the overlay loader loads
it in a preview session. Applications never import it, and nothing in
`@glossa/runtime`, `@glossa/elements`, `@glossa/vue` or `@glossa/react`
imports it. That's why it is a package of its own rather than a
`@glossa/runtime/capture` entry: a production bundle can't reach it through
any import an application makes, its test tooling (Playwright, ajv, esbuild)
stays out of the runtime, and a test (`src/bundle.test.ts`) bundles an app on
the runtime and every component package and checks that no capture code is in
it. The runtime only has the extension point, `onRender`, which costs under
100 bytes.

```ts
import { startCapture } from "@glossa/capture";

const session = startCapture(runtime); // or [runtimeA, runtimeB] for islands
// …the page re-renders with markers and host attributes…
const { renders, regions } = session.collect(); // captures.v1 fields
session.stop(); // remove the hook and every marker
```

## How messages are found

- **Components** (`<glossa-text>`, Vue `<GlossaText>`, React `<T>`) put
  `data-glossa-id` and `data-glossa-locale` on their host element while a
  session's hook is installed. The Vue and React components have no element
  of their own, so in a session they wrap their content in a
  `<span style="display: contents">`. The region (kind `element`) is the box
  of what the host renders, through `display: contents`, shadow roots and slots.
- **`t()` strings** are wrapped in invisible markers: a start mark, the
  render's index in the session's render log in binary, the text, an end mark.
  The capture script walks text nodes (open shadow roots included), turns
  every marked range into boxes with `Range.getClientRects()`, one region of
  kind `text` per line, and looks the index up in the log. Markers nest, so a
  marked value inside another message yields both regions.
- **Attributes**: a marked `placeholder`, `title`, `aria-label`, `alt` (any
  attribute, in fact) or the `value` of a button or text input is a region of
  kind `attribute`: the element's box.
- A region that renders zero-size, is clipped away by an `overflow`
  container, lies outside the page, or is `visibility: hidden`, `display:
  none` or transparent has `visible: false`, so the gap shows instead of
  being hidden.

Boxes are CSS pixels from the top-left of the full page, whatever the scroll
position. `collect()` returns the `renders` the regions refer to and the
`regions`, exactly as a `glossa.captures/v1` capture holds them
([schema](../../testdata/schemas/captures.v1.schema.json)); `glossa capture`
adds the route, viewport, locale and image. Message IDs that aren't catalog
keys can't be named in that format and are left out; an inline default's
locale is `und`.

### The markers

The characters are the Unicode invisible operators U+2061–U+2064 (start
U+2063, end U+2064, binary digits U+2061/U+2062). They are default-ignorable,
so browsers draw nothing and advance nothing for them; bidi class BN, so the
bidi algorithm ignores them; joining type transparent, so Arabic letters next
to them keep their shapes; and line-break class AL, so they add no break
opportunity inside a word. Zero-width joiners (ZWJ/ZWNJ) would change shaping
and ZWSP would add break opportunities, so they aren't used. The browser tests
check that no box on the fixture page moves by more than 0.01 px when the
markers are added.

Equal renders (same ID, locale, values and output) share one log entry. The
log keeps a digest of the values (FNV-1a of their JSON), never the values.

## The trade-off

**Markers change string lengths.** During a session, a `t()` string is a few
characters longer than what the page shows: `maxlength` inputs, code that
compares or slices translated strings, `document.title` and strings copied
into non-DOM places (clipboard, analytics, `localStorage`) see the markers.
That's why markers exist only while a session's hook is installed, never in a
normal page view, and why `stop()` removes the hook (frameworks re-render
without markers) and then strips whatever markers are still in the document:
text, attribute values, input values and open shadow roots
(`stripMarkers(root)` does just that part).

The alternatives were worse: matching rendered text back to messages fails on
duplicates ("Save" appears ten times, from different messages) and on
formatted values; marking only elements misses every `t()` call, which is most
Vue and Go-template usage.

## API

| | |
|---|---|
| `startCapture(runtimes) → CaptureSession` | Installs the hook on one runtime or several (they share one log). |
| `session.renders` | The render log: `{ id, locale, digest }`, a marker's index is a position in it. |
| `session.collect(root?) → { renders, regions }` | The capture script, over the document or a subtree. |
| `session.stop()` | Removes the hooks and strips the markers left in the document. Idempotent. |
| `collectRegions(log, root?)` | The capture script on its own, for a log kept elsewhere. |
| `stripMarkers(root?)`, `strip(s)` | Remove markers from a DOM tree or a string. |
| `mark(index, text)`, `ranges(s)`, `digest(values)` | The marker format and the values digest. |

## Tests

- `pnpm test`: markers, the session and the capture script's structure in
  jsdom, with Vue and React apps, validated against captures.v1 with ajv; the
  tree-shaking check.
- `pnpm test:browser`: geometry in Chromium with Playwright on a fixture page:
  "Speichern" rendered by three different messages, formatted values,
  attributes, hidden and off-screen text, RTL text, wrapped lines, a
  `<glossa-text>` host, scroll independence, and that markers move nothing.
  Needs the workspace built and `pnpm exec playwright install chromium`.
