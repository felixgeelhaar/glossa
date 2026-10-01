// RFC 0006 §12.6 and §7.3: render every key in every locale twice —
// through v0.3's own formatter (@felixgeelhaar/glossa-format, its
// hand-written ICU subset) over the text v0.3's API serves, and through
// @glossa/runtime over the artifact glossa-edge serves — with the same
// arguments. The two share no code: one is v0.3's parser and formatter,
// the other the MF1 → MF2 converter plus the new interpreter.
//
//   node render.mjs <input.json> <output.json> <packages/format> <runtimes/js/runtime>
//
// input:  { cases: [{ key, locale, args }], v0: { <locale>: { <key>: <ICU text> } },
//           edgeURL, deliveryKey, environment }
// output: [{ key, locale, args, v0, runtime, v0Error, runtimeError, release }]
import { readFileSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { join } from "node:path";

const [, , inPath, outPath, formatDir, runtimeDir] = process.argv;
const input = JSON.parse(readFileSync(inPath, "utf8"));
const { format } = await import(pathToFileURL(join(formatDir, "dist", "index.js")).href);
const { createRuntime } = await import(pathToFileURL(join(runtimeDir, "dist", "index.js")).href);

const errors = [];
const runtimes = new Map();
async function runtimeFor(locale) {
  if (!runtimes.has(locale)) {
    const rt = createRuntime({
      edge: input.edgeURL,
      deliveryKey: input.deliveryKey,
      environment: input.environment,
      locales: [locale],
      storage: null,
      refreshInterval: 0,
      bidiIsolation: "none",
      onError: (e) => errors.push(`${locale}: ${e.type}: ${e.message ?? ""}`),
    });
    await rt.ready;
    runtimes.set(locale, rt);
  }
  return runtimes.get(locale);
}

const out = [];
for (const c of input.cases) {
  const row = { key: c.key, locale: c.locale, args: c.args };
  const text = input.v0[c.locale]?.[c.key];
  try {
    row.v0 = text === undefined ? undefined : format(text, c.locale, c.args);
    if (text === undefined) row.v0Error = "v0.3's API serves no text for it";
  } catch (e) {
    row.v0Error = String(e?.message ?? e);
  }
  try {
    const rt = await runtimeFor(c.locale);
    row.release = rt.release?.id ?? null;
    const ex = rt.explain(c.key);
    row.runtime = rt.t(c.key, c.args);
    if (ex.resolvedFrom !== c.locale) {
      row.runtimeError = `resolved from ${ex.resolvedFrom ?? "nothing (the inline default)"}, not ${c.locale}`;
    }
  } catch (e) {
    row.runtimeError = String(e?.message ?? e);
  }
  out.push(row);
}
writeFileSync(outPath, JSON.stringify({ rows: out, errors: errors.slice(0, 20) }));
