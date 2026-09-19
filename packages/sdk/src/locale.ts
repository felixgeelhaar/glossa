// Locale identity helpers. Mirrors the Go side's locale.Code
// (apps/api/internal/domain/locale) so a locale means the same thing
// on every runtime. Locale data comes from the platform's Intl (CLDR);
// nothing here is a hand-maintained locale table except the list of
// right-to-left scripts, which is Unicode data that Intl only exposes
// through the not-yet-universal Intl.Locale#getTextInfo.

/** Base text direction, matching the HTML `dir` attribute. */
export type Direction = "ltr" | "rtl";

/** A locale's canonical tag, explicit subtags and text direction. */
export interface LocaleInfo {
  /** Canonical BCP 47 tag, e.g. `zh-Hant-TW`. */
  code: string;
  /** Primary language subtag, e.g. `zh`. */
  language: string;
  /** Explicit script subtag (`Hant`), `""` when the tag has none. */
  script: string;
  /** Explicit region subtag (`TW`, `419`), `""` when the tag has none. */
  region: string;
  /** Direction of the explicit or CLDR likely script. */
  direction: Direction;
}

// ISO 15924 scripts whose characters are right-to-left (Unicode bidi
// class R or AL). Keep in sync with rtlScripts in the Go locale package.
const RTL_SCRIPTS = new Set(
  (
    "Adlm Arab Aran Armi Avst Chrs Cprt Elym Hatr Hebr Hung Khar Lydi Mand Mani Mend " +
    "Merc Mero Narb Nbat Nkoo Orkh Ougr Palm Phli Phlp Phnx Prti Rohg Samr Sarb Sogd " +
    "Sogo Syrc Thaa Yezi"
  ).split(" "),
);

/**
 * Describe a BCP 47 tag. Accepts `_` as a separator (`en_US`) the way
 * the server does. Throws a `RangeError` for a malformed tag.
 */
export function describeLocale(tag: string): LocaleInfo {
  const loc = new Intl.Locale(tag.replaceAll("_", "-"));
  const likelyScript = loc.script ?? loc.maximize().script ?? "";
  return {
    code: loc.baseName,
    language: loc.language,
    script: loc.script ?? "",
    region: loc.region ?? "",
    direction: RTL_SCRIPTS.has(likelyScript) ? "rtl" : "ltr",
  };
}
