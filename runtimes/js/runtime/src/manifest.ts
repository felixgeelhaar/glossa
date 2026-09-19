/**
 * The delivery contract's wire types (runtimes/SPEC.md §1): a release
 * manifest and the per-locale, per-namespace artifacts it names.
 */
import type { Message } from "./model.js";

export interface ManifestLocale {
  /** Canonical BCP 47, extensions and private use excluded. */
  code: string;
  direction: "ltr" | "rtl";
}

export interface ArtifactRef {
  /** SHA-256 (lowercase hex) of the artifact's exact bytes. */
  sha256: string;
  size: number;
}

export interface ManifestSignature {
  keyId: string;
  alg: "Ed25519";
  /** base64url Ed25519 signature over the JCS form of the manifest without `signatures`. */
  sig: string;
}

export interface Manifest {
  schema: string;
  project: string;
  environment: string;
  release: { id: string; version: number; createdAt: string };
  sourceLocale: string;
  locales: ManifestLocale[];
  /** Locale → ordered fallback locales; `"*"` is the default chain. */
  fallback: Record<string, string[]>;
  /** Locale → namespace → artifact. */
  artifacts: Record<string, Record<string, ArtifactRef>>;
  signatures?: ManifestSignature[];
}

export interface Artifact {
  schema: string;
  locale: string;
  namespace: string;
  messages: Record<string, Message>;
}
