import { describe, expect, it } from "vitest";
import { parse, ParseError } from "./parse.js";

describe("parse — literals", () => {
  it("returns a single literal for a plain string", () => {
    expect(parse("Hello world")).toEqual([
      { type: "literal", value: "Hello world" },
    ]);
  });

  it("returns empty array for empty input", () => {
    expect(parse("")).toEqual([]);
  });
});

describe("parse — variables", () => {
  it("parses a bare variable", () => {
    expect(parse("Hi {name}!")).toEqual([
      { type: "literal", value: "Hi " },
      { type: "var", name: "name" },
      { type: "literal", value: "!" },
    ]);
  });

  it("allows dots and dashes in identifiers", () => {
    expect(parse("{user.first-name}")).toEqual([
      { type: "var", name: "user.first-name" },
    ]);
  });

  it("tolerates whitespace inside the braces", () => {
    expect(parse("{ name }")).toEqual([{ type: "var", name: "name" }]);
  });
});

describe("parse — apostrophe escaping", () => {
  it("turns '' into a literal apostrophe", () => {
    expect(parse("it''s fine")).toEqual([
      { type: "literal", value: "it's fine" },
    ]);
  });

  it("treats '{ ... ' as a quoted run that hides braces", () => {
    expect(parse("show '{name}' here")).toEqual([
      { type: "literal", value: "show {name} here" },
    ]);
  });

  it("matches an unterminated quoted run to end-of-string", () => {
    // Per ICU, an unterminated quote consumes to EOF; the closing
    // apostrophe is optional in our implementation.
    expect(parse("'{never closed")).toEqual([
      { type: "literal", value: "{never closed" },
    ]);
  });
});

describe("parse — plural", () => {
  it("parses keyword cases", () => {
    const ast = parse(
      "{count, plural, one {one item} other {# items}}",
    );
    expect(ast).toHaveLength(1);
    const root = ast[0];
    expect(root?.type).toBe("plural");
    if (root?.type !== "plural") throw new Error();
    expect(root.name).toBe("count");
    expect(Object.keys(root.cases).sort()).toEqual(["one", "other"]);
    expect(root.cases.one).toEqual([{ type: "literal", value: "one item" }]);
    expect(root.cases.other).toEqual([
      { type: "pound" },
      { type: "literal", value: " items" },
    ]);
  });

  it("parses =N exact cases alongside keywords", () => {
    const ast = parse(
      "{n, plural, =0 {no items} one {one item} other {# items}}",
    );
    const root = ast[0];
    if (root?.type !== "plural") throw new Error();
    expect(root.exact[0]).toEqual([{ type: "literal", value: "no items" }]);
    expect(root.cases.one).toEqual([{ type: "literal", value: "one item" }]);
  });

  it("rejects a plural missing the `other` case", () => {
    expect(() => parse("{n, plural, one {x}}")).toThrow(ParseError);
  });
});

describe("parse — select", () => {
  it("parses gender-style selects", () => {
    const ast = parse(
      "{gender, select, female {Athletin} male {Athlet} other {Athlet:in}}",
    );
    const root = ast[0];
    if (root?.type !== "select") throw new Error();
    expect(root.name).toBe("gender");
    expect(Object.keys(root.cases).sort()).toEqual([
      "female",
      "male",
      "other",
    ]);
  });

  it("rejects a select missing the `other` case", () => {
    expect(() => parse("{g, select, male {x}}")).toThrow(ParseError);
  });
});

describe("parse — nesting", () => {
  it("nests plural inside select", () => {
    const src =
      "{kind, select, app {{count, plural, one {one app} other {# apps}}} other {…}}";
    const ast = parse(src);
    const select = ast[0];
    if (select?.type !== "select") throw new Error();
    const appBranch = select.cases.app;
    expect(appBranch).toBeDefined();
    expect(appBranch?.[0]?.type).toBe("plural");
  });
});

describe("parse — error cases", () => {
  it("flags unknown argType", () => {
    expect(() => parse("{x, number, integer}")).toThrow(ParseError);
  });

  it("flags trailing junk", () => {
    expect(() => parse("ok}")).toThrow(ParseError);
  });
});
