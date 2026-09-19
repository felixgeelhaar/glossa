import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { MF1SyntaxError, parseMF1 } from "./mf1.js";
import { format, parseMF2, stringify } from "./mf2.js";
import { isMessage } from "./schema.js";
import { testdataDir } from "./testing/index.js";

/** Convert, then show as MF2 syntax without the round-trip attributes. */
const mf2 = (src: string, locale = "en") =>
  stringify(parseMF1(src, locale))
    .replace(/ @mf1:arg(Type|Style)=(\|[^|]*\||[^\s}]+)/g, "")
    .replace(/\n/g, " ");

describe("parseMF1 maps ICU MessageFormat 1 onto standard MF2 functions", () => {
  it.each([
    ["Hello {name}", "Hello {$name}"],
    ["It's '{literal}'", "It's \\{literal\\}"],
    ["{n, number}", "{$n :number}"],
    ["{n, number, integer}", "{$n :integer}"],
    ["{n, number, percent}", "{$n :percent}"],
    [
      "{n, number, ::percent scale/100 .0}",
      "{$n :percent minimumFractionDigits=1 maximumFractionDigits=1}",
    ],
    ["{n, number, ::currency/EUR}", "{$n :currency currency=EUR}"],
    ["{n, number, ::.00}", "{$n :number minimumFractionDigits=2 maximumFractionDigits=2}"],
    ["{n, number, ::measure-unit/length-kilometer}", "{$n :unit unit=kilometer}"],
    ["{d, date}", "{$d :date}"],
    ["{d, date, short}", "{$d :date length=short}"],
    ["{d, date, long}", "{$d :date length=long}"],
    ["{d, date, full}", "{$d :date fields=year-month-day-weekday length=long}"],
    ["{t, time}", "{$t :time precision=second}"],
    ["{t, time, short}", "{$t :time precision=minute}"],
    ["{t, time, full}", "{$t :time precision=second timeZoneStyle=long}"],
    [
      "{g, select, female {She} male {He} other {They}}",
      ".input {$g :string} .match $g female {{She}} male {{He}} * {{They}}",
    ],
    [
      "{c, plural, =0 {none} one {# item} other {# items}}",
      ".input {$c :number} .match $c 0 {{none}} one {{{$c} item}} * {{{$c} items}}",
    ],
    [
      "{c, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}",
      ".input {$c :number select=ordinal} .match $c one {{{$c}st}} two {{{$c}nd}} few {{{$c}rd}} * {{{$c}th}}",
    ],
  ])("%s → %s", (src, exp) => {
    expect(mf2(src)).toBe(exp);
  });

  it("lifts nested selectors into one .match with the catch-all last", () => {
    const src =
      "{g, select, female {{c, plural, one {She has # cat} other {She has # cats}}} " +
      "other {{c, plural, one {They have # cat} other {They have # cats}}}}";
    expect(mf2(src)).toBe(
      ".input {$g :string} .input {$c :number} .match $g $c " +
        "female one {{She has {$c} cat}} female * {{She has {$c} cats}} " +
        "* one {{They have {$c} cat}} * * {{They have {$c} cats}}",
    );
  });

  it("maps a plural offset onto :offset, keeping exact matches on the raw value", () => {
    const src =
      "{c, plural, offset:1 =0 {Nobody} =1 {{host}} one {{host} and # other} " +
      "other {{host} and # others}}";
    expect(mf2(src)).toBe(
      ".input {$c :number} .local $c_offset = {$c :offset subtract=1} .match $c $c_offset " +
        "1 * {{{$host}}} 0 * {{Nobody}} * one {{{$host} and {$c_offset} other}} " +
        "* * {{{$host} and {$c_offset} others}}",
    );
    const msg = parseMF1(src, "en");
    const fmt = (c: number) => format(msg, "en", { c, host: "Ada" }, { bidiIsolation: "none" });
    expect([0, 1, 2, 3].map(fmt)).toEqual(["Nobody", "Ada", "Ada and 1 other", "Ada and 2 others"]);
  });

  it("avoids a name clash for the offset variable", () => {
    const out = mf2("{c, plural, offset:1 one {# {c_offset}} other {#}}");
    expect(out).toContain(".local $c_offset_ = {$c :offset subtract=1}");
    expect(out).toContain("{{{$c_offset_} {$c_offset}}}");
  });

  it("falls back to mf1: functions only where MF2 has no equivalent", () => {
    expect(mf2("{n, number, currency}")).toBe("{$n :mf1:currency}");
    expect(mf2("{n, number, ::compact-short}")).toMatch(/^\{\$n :mf1:number /);
    expect(mf2("{d, date, ::yMMMd}")).toMatch(/^\{\$d :mf1:date /);
    expect(mf2("{n, spellout}")).toBe("{$n :mf1:spellout}");
    expect(mf2("{d, duration}")).toBe("{$d :mf1:duration}");
  });

  it("keeps the MF1 argument type and style as attributes for export", () => {
    const msg = parseMF1("{n, number, percent}", "en");
    expect(msg).toMatchObject({
      pattern: [
        {
          attributes: {
            "mf1:argType": { type: "literal", value: "number" },
            "mf1:argStyle": { type: "literal", value: "percent" },
          },
        },
      ],
    });
  });

  it("produces plain JSON matching the data model schema", () => {
    const msg = parseMF1("{n, number, ::.00} {c, plural, offset:2 =0 {a} other {#}}", "de");
    expect(isMessage(msg)).toBe(true);
    expect(JSON.parse(JSON.stringify(msg))).toEqual(msg);
    expect(parseMF2(stringify(msg))).toEqual(msg);
  });

  it("validates plural keys against the locale", () => {
    expect(() => parseMF1("{n, plural, one {a} few {b} other {c}}", "en")).toThrow(MF1SyntaxError);
    expect(() => parseMF1("{n, plural, one {a} few {b} many {c} other {d}}", "pl")).not.toThrow();
  });

  it("reports syntax errors as MF1SyntaxError", () => {
    expect(() => parseMF1("{n, plural, one {a}", "en")).toThrow(MF1SyntaxError);
  });

  it("formats converted German messages", () => {
    const msg = parseMF1(
      "{count, plural, =0 {Keine Dateien} one {Eine Datei} other {# Dateien}} ({size, number, ::.0} MB)",
      "de",
    );
    const out = format(msg, "de", { count: 1234, size: 3.25 }, { bidiIsolation: "none" });
    expect(out).toBe("1.234 Dateien (3,3 MB)");
  });
});

/**
 * Shared fixture owned by the Go converter: MF1 source → canonical MF2 data
 * model. Both implementations must produce identical data models.
 * Shape (mirrors the Unicode suite): { "tests": [{ "src", "locale"?, "exp" }] }.
 */
const sharedFixture = join(testdataDir, "glossa/mf1-to-mf2.json");

describe.skipIf(!existsSync(sharedFixture))("shared fixture glossa/mf1-to-mf2.json", () => {
  interface Case {
    description?: string;
    src: string;
    locale?: string;
    exp: unknown;
  }
  const load = (): Case[] => {
    const file = JSON.parse(readFileSync(sharedFixture, "utf8")) as {
      defaultTestProperties?: Partial<Case>;
      tests: Case[];
    };
    return file.tests.map((t) => ({ ...file.defaultTestProperties, ...t }));
  };
  it.each(existsSync(sharedFixture) ? load().map((c) => [c.description ?? c.src, c] as const) : [])(
    "%s",
    (_name, c) => {
      expect(parseMF1(c.src, c.locale ?? "en")).toEqual(c.exp);
    },
  );
});

if (!existsSync(sharedFixture)) {
  console.info(`skipping shared MF1 fixture: ${sharedFixture} not present`);
}
