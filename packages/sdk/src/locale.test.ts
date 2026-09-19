import { describe, expect, it } from "vitest";

import { describeLocale } from "./locale.js";

describe("describeLocale", () => {
  it("splits a tag into explicit subtags", () => {
    expect(describeLocale("zh-Hant-TW")).toEqual({
      code: "zh-Hant-TW",
      language: "zh",
      script: "Hant",
      region: "TW",
      direction: "ltr",
    });
    expect(describeLocale("es-419")).toMatchObject({
      language: "es",
      script: "",
      region: "419",
    });
  });

  it("canonicalizes casing and underscores", () => {
    expect(describeLocale("EN_us").code).toBe("en-US");
    expect(describeLocale("iw").code).toBe("he");
  });

  it.each([
    ["ar", "rtl"],
    ["ar-EG", "rtl"],
    ["he", "rtl"],
    ["fa", "rtl"],
    ["ur", "rtl"],
    ["ckb", "rtl"],
    ["pa-Arab", "rtl"],
    ["pa", "ltr"],
    ["de", "ltr"],
    ["ja", "ltr"],
  ])("derives %s as %s from the likely script", (tag, direction) => {
    expect(describeLocale(tag).direction).toBe(direction);
  });

  it("throws a RangeError for a malformed tag", () => {
    expect(() => describeLocale("not a tag")).toThrow(RangeError);
  });
});
