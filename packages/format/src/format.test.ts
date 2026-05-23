import { describe, expect, it } from "vitest";
import { format } from "./format.js";

describe("format — variables", () => {
  it("interpolates a named value", () => {
    expect(format("Hi {name}!", "en", { name: "Sophia" })).toBe("Hi Sophia!");
  });

  it("renders missing variables as empty string", () => {
    expect(format("Hi {name}!", "en", {})).toBe("Hi !");
  });

  it("stringifies numbers and booleans", () => {
    expect(format("{n} = {ok}", "en", { n: 42, ok: true })).toBe("42 = true");
  });
});

describe("format — apostrophe escaping", () => {
  it("renders literal apostrophes via ''", () => {
    expect(format("it''s fine", "en")).toBe("it's fine");
  });

  it("treats quoted runs as literal", () => {
    expect(format("'{not a var}'", "en", { var: "x" })).toBe("{not a var}");
  });
});

describe("format — plural (English)", () => {
  it("picks the `one` branch for 1", () => {
    expect(
      format(
        "{n, plural, one {one item} other {# items}}",
        "en",
        { n: 1 },
      ),
    ).toBe("one item");
  });

  it("picks the `other` branch for 5", () => {
    expect(
      format(
        "{n, plural, one {one item} other {# items}}",
        "en",
        { n: 5 },
      ),
    ).toBe("5 items");
  });

  it("prefers =N exact cases over keyword resolution", () => {
    const msg =
      "{n, plural, =0 {no items} one {one item} other {# items}}";
    expect(format(msg, "en", { n: 0 })).toBe("no items");
    expect(format(msg, "en", { n: 1 })).toBe("one item");
    expect(format(msg, "en", { n: 2 })).toBe("2 items");
  });

  it("handles negative counts via the locale's plural rules", () => {
    // English: -1 falls into `one` per Intl.PluralRules (it returns
    // "one" for n=-1 because the cardinal-form set treats |n|=1).
    // We just assert behaviour matches the platform rule, whatever
    // that rule says — we don't override it.
    const rule = new Intl.PluralRules("en").select(-1);
    const branch = rule === "one" ? "one item" : "-1 items";
    expect(
      format(
        "{n, plural, one {one item} other {# items}}",
        "en",
        { n: -1 },
      ),
    ).toBe(branch);
  });
});

describe("format — plural (German)", () => {
  it("uses German plural rules", () => {
    const msg =
      "{n, plural, one {ein Eintrag} other {# Einträge}}";
    expect(format(msg, "de", { n: 1 })).toBe("ein Eintrag");
    expect(format(msg, "de", { n: 2 })).toBe("2 Einträge");
  });
});

describe("format — select", () => {
  it("matches the requested branch", () => {
    const msg =
      "{gender, select, female {Athletin} male {Athlet} other {Athlet:in}}";
    expect(format(msg, "de", { gender: "female" })).toBe("Athletin");
    expect(format(msg, "de", { gender: "male" })).toBe("Athlet");
    expect(format(msg, "de", { gender: "nonbinary" })).toBe("Athlet:in");
  });

  it("falls back to `other` when the variable is missing", () => {
    const msg =
      "{g, select, female {A} male {B} other {C}}";
    expect(format(msg, "en", {})).toBe("C");
  });
});

describe("format — nesting", () => {
  it("plural inside select inside literal", () => {
    const msg =
      "Hi {name}, you have {kind, select, app {{count, plural, one {one app} other {# apps}}} other {something}}.";
    expect(
      format(msg, "en", { name: "Sophia", kind: "app", count: 3 }),
    ).toBe("Hi Sophia, you have 3 apps.");
    expect(
      format(msg, "en", { name: "Sophia", kind: "app", count: 1 }),
    ).toBe("Hi Sophia, you have one app.");
    expect(
      format(msg, "en", { name: "Sophia", kind: "other-kind", count: 1 }),
    ).toBe("Hi Sophia, you have something.");
  });
});

describe("format — deeply nested selects", () => {
  it("walks more than two layers", () => {
    const msg =
      "{tier, select, gold {{member, select, true {Gold member} other {Gold guest}}} silver {Silver} other {Bronze}}";
    expect(format(msg, "en", { tier: "gold", member: true })).toBe(
      "Gold member",
    );
    expect(format(msg, "en", { tier: "gold", member: false })).toBe(
      "Gold guest",
    );
    expect(format(msg, "en", { tier: "silver" })).toBe("Silver");
    expect(format(msg, "en", { tier: "platinum" })).toBe("Bronze");
  });
});
