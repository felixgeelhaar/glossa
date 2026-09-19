/** Options of the `glossa()` integration, and what reaches the site's code. */
import type { BundledRelease, PublicKey } from "@glossa/runtime";

import type { Routing } from "./routing.js";

export interface GlossaAstroOptions {
  /** Edge origin. With `deliveryKey`: the build fetches its release here, and islands refresh from it. */
  edge?: string;
  /** The project's publishable delivery key. */
  deliveryKey?: string;
  /** Default `production`. */
  environment?: string;
  /** Trusted manifest-signing keys. */
  publicKeys?: PublicKey[];
  /**
   * The release to build with: a `glossa pull --release` directory (relative
   * to the project root) or a release object. Default: the edge's current
   * release, fetched once at build start.
   */
  release?: string | BundledRelease;
  /** The site's locales. Default: Astro's `i18n.locales`, else the release's. */
  locales?: Array<string | { path: string; codes: string[] }>;
  /** Default: Astro's `i18n.defaultLocale`, else the release's source locale. */
  defaultLocale?: string;
  /**
   * Render `<glossa-*>` elements into the HTML: on prerendered pages
   * (`"static"`, the default), also on on-demand pages (`"all"`, which buffers
   * them instead of streaming), or never (`false`).
   */
  prerender?: "static" | "all" | false;
  /**
   * Inline the page locale's slice of the release, so islands and elements
   * hydrate with exactly the server's text: on pages with islands or providers
   * (`"auto"`, the default), on every page, or never.
   */
  inline?: "auto" | "always" | "never";
  /** Define the `<glossa-*>` elements on every page, sharing the page's runtime with islands. */
  elements?: boolean;
}

/** `virtual:glossa/config`: public, so it's safe in client bundles. */
export interface PublicConfig {
  edge?: string;
  deliveryKey?: string;
  environment: string;
  publicKeys?: PublicKey[];
  routing: Routing;
  prerender: "static" | "all" | false;
  inline: "auto" | "always" | "never";
}
