/**
 * Source of `messageformat/testdata/glossa/runtime-format.json`: real-world
 * German and English UI strings (plus Arabic and Hebrew for bidi), compiled to
 * the canonical data model and formatted with the reference implementation.
 *
 * Regenerate with `pnpm --filter @glossa/messageformat generate:runtime-format`.
 * This module has no runtime imports so the generator (plain Node) and the
 * drift test (vitest) can both use it.
 */
import type { FormatError, Message, MessagePart } from "../src/index.js";

export interface Api {
  parseMF2(src: string): Message;
  format(
    message: Message,
    locale: string,
    values: Record<string, unknown>,
    opts: FormatOpts,
  ): string;
  formatToParts(
    message: Message,
    locale: string,
    values: Record<string, unknown>,
    opts: FormatOpts,
  ): MessagePart<string>[];
}

interface FormatOpts {
  bidiIsolation: "default" | "none";
  onError: (e: FormatError) => void;
}

/** Scalar params; ISO strings under `dates` become `Date` values (`type: "datetime"`). */
type Params = Record<string, string | number>;

interface Case {
  description: string;
  locale: string;
  src: string;
  /** One test per params set. */
  params?: Params[];
  dates?: string[];
  bidiIsolation?: "none";
  parts?: true;
}

const at = "2026-09-19T14:05:00Z";

export const cases: Case[] = [
  // ── plurals with exact keys ──────────────────────────────────────
  {
    description: "en inbox: exact 0 before plural categories",
    locale: "en",
    src:
      ".input {$count :number} .match $count 0 {{Your inbox is empty}} " +
      "one {{You have one new message}} * {{You have {$count} new messages}}",
    params: [{ count: 0 }, { count: 1 }, { count: 2 }, { count: 1250 }],
    bidiIsolation: "none",
  },
  {
    description: "de inbox: exact 0 before plural categories",
    locale: "de",
    src:
      ".input {$count :number} .match $count 0 {{Keine neuen Nachrichten}} " +
      "one {{Eine neue Nachricht}} * {{{$count} neue Nachrichten}}",
    params: [{ count: 0 }, { count: 1 }, { count: 3 }, { count: 1250 }],
    bidiIsolation: "none",
  },
  {
    description: "de cart from ICU MF1 with =0",
    locale: "de",
    // Canonical MF2 produced from ICU MF1 by the Go kernel
    // (messageformat.ParseMF1). MF1 conversion lives only in Go.
    src: ".input {$items :number}\n.match $items\n0 {{Dein Warenkorb ist leer}}\none {{{$items} Artikel im Warenkorb}}\n* {{{$items} Artikel im Warenkorb}}",
    params: [{ items: 0 }, { items: 1 }, { items: 12 }],
    bidiIsolation: "none",
  },
  {
    description: "en exact 1 wins over category one",
    locale: "en",
    src: ".input {$n :number} .match $n 1 {{exactly one}} one {{category one}} * {{other}}",
    params: [{ n: 1 }, { n: 2 }],
    bidiIsolation: "none",
  },
  // ── ordinals ─────────────────────────────────────────────────────
  {
    description: "en ordinal suffixes",
    locale: "en",
    src:
      ".input {$place :number select=ordinal} .match $place " +
      "one {{You finished {$place}st}} two {{You finished {$place}nd}} " +
      "few {{You finished {$place}rd}} * {{You finished {$place}th}}",
    params: [1, 2, 3, 4, 11, 12, 13, 21, 22, 23, 101, 111].map((place) => ({ place })),
    bidiIsolation: "none",
  },
  {
    description: "en selectordinal from ICU MF1",
    locale: "en",
    // Canonical MF2 produced from ICU MF1 by the Go kernel
    // (messageformat.ParseMF1). MF1 conversion lives only in Go.
    src: ".input {$n :number select=ordinal}\n.match $n\none {{Your {$n}st order}}\ntwo {{Your {$n}nd order}}\nfew {{Your {$n}rd order}}\n* {{Your {$n}th order}}",
    params: [{ n: 1 }, { n: 22 }, { n: 13 }],
    bidiIsolation: "none",
  },
  {
    description: "de ordinal as integer with a period",
    locale: "de",
    src: "Sie sind {$pos :integer}. in der Warteschlange.",
    params: [{ pos: 3 }],
    bidiIsolation: "none",
  },
  // ── currency (EUR) ───────────────────────────────────────────────
  {
    description: "de EUR total",
    locale: "de",
    src: "Gesamt: {$total :currency currency=EUR}",
    params: [{ total: 1234.5 }, { total: 0.99 }, { total: -12 }],
    bidiIsolation: "none",
  },
  {
    description: "en-US EUR total",
    locale: "en-US",
    src: "Total: {$total :currency currency=EUR}",
    params: [{ total: 1234.5 }],
    bidiIsolation: "none",
  },
  {
    description: "en EUR without cents",
    locale: "en",
    src: "From {$price :currency currency=EUR fractionDigits=0}",
    params: [{ price: 19.99 }],
    bidiIsolation: "none",
  },
  {
    description: "de EUR spelled out",
    locale: "de",
    src: "{$total :currency currency=EUR currencyDisplay=name}",
    params: [{ total: 1234.5 }],
    bidiIsolation: "none",
  },
  {
    description: "de EUR from an ICU MF1 skeleton",
    locale: "de",
    // Canonical MF2 produced from ICU MF1 by the Go kernel
    // (messageformat.ParseMF1). MF1 conversion lives only in Go.
    src: "Preis: {$price :currency currency=EUR}",
    params: [{ price: 49.9 }],
    bidiIsolation: "none",
  },
  // ── dates and times (UTC) ────────────────────────────────────────
  {
    description: "de long date",
    locale: "de",
    src: "Bestellt am {$date :date length=long timeZone=UTC}",
    params: [{ date: at }],
    dates: ["date"],
    bidiIsolation: "none",
  },
  {
    description: "en long date",
    locale: "en",
    src: "Ordered on {$date :date length=long timeZone=UTC}",
    params: [{ date: at }],
    dates: ["date"],
    bidiIsolation: "none",
  },
  {
    description: "de date with weekday",
    locale: "de",
    src: "{$d :date fields=year-month-day-weekday length=long timeZone=UTC}",
    params: [{ d: at }],
    dates: ["d"],
    bidiIsolation: "none",
  },
  {
    description: "en short date",
    locale: "en",
    src: "{$d :date length=short timeZone=UTC}",
    params: [{ d: at }],
    dates: ["d"],
    bidiIsolation: "none",
  },
  {
    description: "de time",
    locale: "de",
    src: "um {$d :time timeZone=UTC} Uhr",
    params: [{ d: at }],
    dates: ["d"],
    bidiIsolation: "none",
  },
  {
    description: "de short datetime",
    locale: "de",
    src: "{$d :datetime dateLength=short timeZone=UTC}",
    params: [{ d: at }],
    dates: ["d"],
    bidiIsolation: "none",
  },
  {
    // Not en: the space before AM/PM changed between CLDR 47 and 48.
    description: "de datetime with seconds",
    locale: "de",
    src: "{$d :datetime dateLength=long timePrecision=second timeZone=UTC}",
    params: [{ d: at }],
    dates: ["d"],
    bidiIsolation: "none",
  },
  // ── numbers, percent, units ──────────────────────────────────────
  {
    description: "de file size with one decimal",
    locale: "de",
    src: "{$size :number maximumFractionDigits=1} MB",
    params: [{ size: 1536.44 }],
    bidiIsolation: "none",
  },
  {
    description: "en fixed two decimals",
    locale: "en",
    src: "Rating: {$n :number minimumFractionDigits=2}",
    params: [{ n: 3 }],
    bidiIsolation: "none",
  },
  {
    description: "de percent",
    locale: "de",
    src: "{$p :percent} erledigt",
    params: [{ p: 0.256 }],
    bidiIsolation: "none",
  },
  {
    description: "en percent with one decimal",
    locale: "en",
    src: "{$p :percent maximumFractionDigits=1} done",
    params: [{ p: 0.256 }],
    bidiIsolation: "none",
  },
  {
    description: "de kilometers",
    locale: "de",
    src: "Noch {$km :unit unit=kilometer}",
    params: [{ km: 12.5 }],
    bidiIsolation: "none",
  },
  {
    description: "en kilometers, long",
    locale: "en",
    src: "{$km :unit unit=kilometer unitDisplay=long} to go",
    params: [{ km: 12.5 }],
    bidiIsolation: "none",
  },
  // ── nested selectors ─────────────────────────────────────────────
  {
    description: "de gender × plural",
    locale: "de",
    src:
      ".input {$gender :string} .input {$count :number} .match $gender $count " +
      "female 1 {{Sie hat eine Datei geteilt.}} female * {{Sie hat {$count} Dateien geteilt.}} " +
      "male 1 {{Er hat eine Datei geteilt.}} male * {{Er hat {$count} Dateien geteilt.}} " +
      "* 1 {{Eine Datei wurde geteilt.}} * * {{{$count} Dateien wurden geteilt.}}",
    params: [
      { gender: "female", count: 1 },
      { gender: "female", count: 4 },
      { gender: "male", count: 1 },
      { gender: "other", count: 2 },
    ],
    bidiIsolation: "none",
  },
  {
    description: "en nested select/plural from ICU MF1",
    locale: "en",
    // Canonical MF2 produced from ICU MF1 by the Go kernel
    // (messageformat.ParseMF1). MF1 conversion lives only in Go.
    src: ".input {$gender :string}\n.input {$n :number}\n.match $gender $n\nfemale one {{She invited one guest}}\nfemale * {{She invited {$n} guests}}\n* one {{They invited one guest}}\n* * {{They invited {$n} guests}}",
    params: [
      { gender: "female", n: 1 },
      { gender: "female", n: 3 },
      { gender: "x", n: 2 },
    ],
    bidiIsolation: "none",
  },
  {
    description: "de plural offset from ICU MF1",
    locale: "de",
    // Canonical MF2 produced from ICU MF1 by the Go kernel
    // (messageformat.ParseMF1). MF1 conversion lives only in Go.
    src: ".input {$guests :number}\n.local $guests_minus_1 = {$guests :offset subtract=1}\n.match $guests $guests_minus_1\n0 one {{{$host} feiert allein.}}\n0 * {{{$host} feiert allein.}}\n1 one {{{$host} und {$guest} feiern.}}\n1 * {{{$host} und {$guest} feiern.}}\n* one {{{$host}, {$guest} und {$guests_minus_1} weitere Person feiern.}}\n* * {{{$host}, {$guest} und {$guests_minus_1} weitere Personen feiern.}}",
    params: [0, 1, 2, 5].map((guests) => ({ guests, host: "Anna", guest: "Ben" })),
    bidiIsolation: "none",
  },
  {
    description: "en likes with .local and :offset",
    locale: "en",
    src:
      ".input {$count :number} .local $others = {$count :offset subtract=1} .match $others " +
      "0 {{Only you liked this}} one {{You and one other person liked this}} " +
      "* {{You and {$others} others liked this}}",
    params: [{ count: 1 }, { count: 2 }, { count: 5 }],
    bidiIsolation: "none",
  },
  {
    description: "en order status via :string",
    locale: "en",
    src:
      ".input {$status :string} .match $status shipped {{Your order has shipped}} " +
      "delivered {{Your order was delivered}} * {{Your order is being processed}}",
    params: [{ status: "shipped" }, { status: "delivered" }, { status: "new" }],
    bidiIsolation: "none",
  },
  // ── markup ───────────────────────────────────────────────────────
  {
    description: "en link markup",
    locale: "en",
    src: "By continuing you accept the {#link href=|/terms|}terms of service{/link}.",
    parts: true,
  },
  {
    description: "de bold count and line break",
    locale: "de",
    src: "{#bold}{$count :integer}{/bold} ungelesene Nachrichten{#br/}",
    params: [{ count: 1200 }],
    parts: true,
  },
  // ── bidi ─────────────────────────────────────────────────────────
  {
    description: "ar greeting with a Latin name",
    locale: "ar",
    src: "مرحبا {$name}!",
    params: [{ name: "John" }],
    parts: true,
  },
  {
    description: "he name and count",
    locale: "he",
    src: "{$name} הוסיפה {$count :integer} פריטים",
    params: [{ name: "Anna", count: 3 }],
    parts: true,
  },
  {
    description: "en greeting with an Arabic name",
    locale: "en",
    src: "Welcome back, {$name}!",
    params: [{ name: "محمد" }],
  },
  {
    description: "he file name forced LTR with u:dir",
    locale: "he",
    src: "שם הקובץ: {$file :string u:dir=ltr}",
    params: [{ file: "report.pdf" }],
    parts: true,
  },
  // ── fallbacks ────────────────────────────────────────────────────
  {
    description: "de missing variable falls back to {$name}",
    locale: "de",
    src: "Hallo {$name}!",
    params: [{}],
  },
  {
    description: "en non-numeric operand falls back",
    locale: "en",
    src: "{$n :number} items",
    params: [{ n: "abc" }],
    bidiIsolation: "none",
  },
];

export interface FixtureTest {
  description: string;
  locale: string;
  src: string;
  syntax: "mf2";
  message: Message;
  params?: Array<{ name: string; value: string | number; type?: "datetime" }>;
  bidiIsolation?: "none";
  exp: string;
  expParts?: unknown[];
  expErrors?: Array<{ type: string }>;
}

export const fixtureComment = [
  "Glossa runtime conformance: precompiled data model + locale + params → formatted output.",
  "Generated by messageformat/js/scripts/generate-runtime-format.ts from the reference",
  "formatter (messageformat v4); do not edit by hand. Each test: `message` is the canonical",
  "MF2 data model (message.schema.json) compiled from `src` (`syntax` is always mf2; ICU MF1 is converted by the Go kernel only);",
  '`params` use the Unicode suite\'s shape (`type: "datetime"` means an ISO string to turn',
  'into a date); `bidiIsolation` defaults to "default"; `exp` is the formatted string;',
  "`expParts`, when present, the formatted parts; `expErrors` the reported error types.",
  "Output depends on CLDR data: `generatedWith` records the ICU/CLDR versions used.",
].join(" ");

/** Build the fixture's tests with the given implementation of the API. */
export function buildTests(api: Api): FixtureTest[] {
  const tests: FixtureTest[] = [];
  for (const c of cases) {
    const message = api.parseMF2(c.src);
    for (const p of c.params ?? [{}]) {
      const values: Record<string, unknown> = {};
      const params: NonNullable<FixtureTest["params"]> = [];
      for (const [name, value] of Object.entries(p)) {
        const isDate = c.dates?.includes(name) ?? false;
        values[name] = isDate ? new Date(value) : value;
        params.push(isDate ? { name, type: "datetime", value } : { name, value });
      }
      const errors: Array<{ type: string }> = [];
      const bidiIsolation = c.bidiIsolation ?? "default";
      const test: FixtureTest = {
        description: c.description,
        locale: c.locale,
        src: c.src,
        syntax: "mf2",
        message,
        exp: api.format(message, c.locale, values, {
          bidiIsolation,
          onError: (e) => errors.push({ type: e.type }),
        }),
      };
      if (params.length) test.params = params;
      if (c.bidiIsolation) test.bidiIsolation = c.bidiIsolation;
      if (c.parts) {
        const parts = api.formatToParts(message, c.locale, values, {
          bidiIsolation,
          onError: () => {},
        });
        test.expParts = JSON.parse(JSON.stringify(parts)) as unknown[];
      }
      if (errors.length) test.expErrors = errors;
      tests.push(test);
    }
  }
  return tests;
}

/** Serialize with invisible and non-ASCII formatting characters escaped, for review. */
export function serialize(fixture: object): string {
  const json = JSON.stringify(fixture, null, 2);
  const visible = /[\p{L}\p{M}\p{N}]/u;
  return `${json.replace(/[^\x00-\x7e]/g, (c) => (visible.test(c) ? c : `\\u${c.charCodeAt(0).toString(16).padStart(4, "0")}`))}\n`;
}
