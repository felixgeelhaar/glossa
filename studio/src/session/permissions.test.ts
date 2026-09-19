import { describe, expect, it } from "vitest";
import { allows, allowsFor, grantFor } from "./permissions";

describe("grantFor", () => {
  it("gives owners everything, everywhere", () => {
    const g = grantFor({ roles: ["owner"], locales: [] });
    expect(allows(g, "owners.manage")).toBe(true);
    expect(allowsFor(g, "translations.review", "ja")).toBe(true);
  });

  it("keeps admins from owner changes", () => {
    const g = grantFor({ roles: ["admin"], locales: [] });
    expect(allows(g, "owners.manage")).toBe(false);
    expect(allows(g, "tenant.manage")).toBe(true);
  });

  it("lets developers write but not review", () => {
    const g = grantFor({ roles: ["developer"], locales: [] });
    expect(allows(g, "catalog.write")).toBe(true);
    expect(allowsFor(g, "translations.write", "de")).toBe(true);
    expect(allowsFor(g, "translations.review", "de")).toBe(false);
  });

  it("limits translators and reviewers to their locales and CLDR descendants", () => {
    const g = grantFor({ roles: ["reviewer"], locales: ["de", "es-419"] });
    expect(allowsFor(g, "translations.review", "de")).toBe(true);
    expect(allowsFor(g, "translations.review", "de-AT")).toBe(true);
    expect(allowsFor(g, "translations.write", "es-AR")).toBe(true);
    expect(allowsFor(g, "translations.write", "es")).toBe(false);
    expect(allowsFor(g, "translations.review", "fr")).toBe(false);
    expect(allows(g, "translations.review")).toBe(false);
    expect(allows(g, "catalog.read")).toBe(true);
    expect(allows(g, "catalog.write")).toBe(false);
  });

  it("doesn't let a locale scope narrow another role's permission", () => {
    const g = grantFor({ roles: ["developer", "translator"], locales: ["de"] });
    expect(allowsFor(g, "translations.write", "fr")).toBe(true);
  });

  it("unions scopes and treats a missing membership as nothing", () => {
    expect(grantFor(undefined).size).toBe(0);
    expect(allowsFor(grantFor(undefined), "translations.read", "de")).toBe(false);
  });
});
