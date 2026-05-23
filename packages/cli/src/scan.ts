// Source walker + key extractor.
//
// Honours .gitignore by composing user-defined globs with the
// repo's ignore patterns via the `ignore` package — same model the
// nox scanner uses, so anyone who's set up nox already understands
// the boundary.
//
// Extraction is regex-driven on purpose. A full TS/HTML parser
// would be overkill: the patterns we care about are short, the
// false-positive rate is low (key strings are stable), and a
// regex pass is two orders of magnitude faster than spinning up a
// TS compiler on every source file.

import { readFile } from "node:fs/promises";
import { glob } from "node:fs/promises";
import { relative, resolve } from "node:path";

import ignore, { type Ignore } from "ignore";

/**
 * One extracted key occurrence. Keeping the file + line lets the
 * CLI print human-friendly diagnostics — a translator who can't
 * find a key in the bundle should be able to grep the codebase
 * for it.
 */
export interface KeyHit {
  name: string;
  file: string;
  line: number;
}

/**
 * Patterns to extract translation keys from. Each must capture the
 * key name in group 1 — everything else is shape. Order doesn't
 * matter (we re-execute each per file); duplicates are de-duped
 * downstream by [[toScanInputs]].
 *
 * Covers:
 *  - <glossa-* key="…">             — the elements
 *  - t("…") / t('…')                — common helper-style API
 *  - i18n.t("…") / I18n.t("…")      — react-i18next / Rails style
 *  - formatMessage({ id: "…" })     — formatjs / react-intl
 *  - client.message(<locale>, "…")  — direct SDK call
 *  - useGlossa("…") / useT("…")     — common hook names
 *
 * Key shape is restricted to dotted lowercase identifiers so we
 * don't accept random string literals — same regex the API's
 * domain layer enforces in translationkey.Name.
 */
const KEY_CHAR = "[a-z0-9_]+(?:\\.[a-z0-9_]+)*";

const EXTRACTORS: RegExp[] = [
  // <glossa-text|rich|plural|select key="…">
  new RegExp(`<glossa-(?:text|rich|plural|select)\\b[^>]*?\\bkey\\s*=\\s*["'](${KEY_CHAR})["']`, "g"),
  // t("…") / t('…')
  new RegExp(`\\bt\\(\\s*["'](${KEY_CHAR})["']`, "g"),
  // i18n.t("…") / I18n.t("…")
  new RegExp(`\\b[iI]18n\\.t\\(\\s*["'](${KEY_CHAR})["']`, "g"),
  // formatMessage({ id: "…" })
  new RegExp(`\\bformatMessage\\(\\s*\\{[^}]*\\bid\\s*:\\s*["'](${KEY_CHAR})["']`, "g"),
  // client.message(locale, "…")
  new RegExp(`\\.message\\(\\s*[^,]+,\\s*["'](${KEY_CHAR})["']`, "g"),
  // useGlossa("…") / useT("…")
  new RegExp(`\\buse(?:Glossa|T)\\(\\s*["'](${KEY_CHAR})["']`, "g"),
];

/**
 * Walk every file under `cwd` matching `patterns`, honour
 * `.gitignore`, run the key-extraction regex, return all hits.
 *
 * Pure I/O; tests pass a fixture directory and assert on the
 * returned hit list.
 */
export async function scanSources(cwd: string, patterns: string[]): Promise<KeyHit[]> {
  const ig = await loadGitignore(cwd);
  const hits: KeyHit[] = [];

  for (const pattern of patterns) {
    for await (const file of glob(pattern, { cwd })) {
      const abs = resolve(cwd, file);
      const rel = relative(cwd, abs);
      if (ig.ignores(rel)) continue;
      const text = await readFile(abs, "utf8");
      extractFromText(text, rel, hits);
    }
  }

  // Sort for determinism — scan output goes straight into a
  // server-side upsert plus a diff-friendly log, both of which
  // want a stable order.
  hits.sort((a, b) => a.name.localeCompare(b.name) || a.file.localeCompare(b.file) || a.line - b.line);
  return hits;
}

/** Pull every key occurrence out of one text buffer. */
function extractFromText(text: string, file: string, hits: KeyHit[]): void {
  const lineStarts = computeLineStarts(text);
  for (const re of EXTRACTORS) {
    re.lastIndex = 0;
    for (let m = re.exec(text); m !== null; m = re.exec(text)) {
      const name = m[1];
      if (!name) continue;
      hits.push({ name, file, line: lineFor(lineStarts, m.index) });
    }
  }
}

function computeLineStarts(text: string): number[] {
  const starts: number[] = [0];
  for (let i = 0; i < text.length; i++) {
    if (text.charCodeAt(i) === 10 /* \n */) starts.push(i + 1);
  }
  return starts;
}

function lineFor(lineStarts: number[], offset: number): number {
  // Binary search for the last lineStart <= offset.
  let lo = 0;
  let hi = lineStarts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >>> 1;
    const start = lineStarts[mid];
    if (start === undefined || start > offset) {
      hi = mid - 1;
    } else {
      lo = mid;
    }
  }
  return lo + 1;
}

async function loadGitignore(cwd: string): Promise<Ignore> {
  const ig = ignore();
  // Always ignore node_modules / dist — common defaults even with
  // no .gitignore file in fixture trees.
  ig.add(["node_modules/", "dist/", ".git/"]);
  try {
    const raw = await readFile(resolve(cwd, ".gitignore"), "utf8");
    ig.add(raw);
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== "ENOENT") throw err;
  }
  return ig;
}

/**
 * Deduplicate hits by key name and return the unique list of
 * `{ name }` rows the API's `/keys:scan` endpoint expects.
 */
export function toScanInputs(hits: KeyHit[]): Array<{ name: string }> {
  const seen = new Set<string>();
  const out: Array<{ name: string }> = [];
  for (const h of hits) {
    if (seen.has(h.name)) continue;
    seen.add(h.name);
    out.push({ name: h.name });
  }
  return out;
}
