export { createClient, GlossaError } from "./client.js";
export type { Client } from "./client.js";
export type {
  Bundle,
  ClientConfig,
  Locale,
  ScanInput,
  ScanResponse,
  ScanResult,
  TranslationStatus,
  TranslationUpdatedEvent,
} from "./types.js";
export type { SubscribeOptions, Subscription } from "./subscribe.js";
export { describeLocale } from "./locale.js";
export type { Direction, LocaleInfo } from "./locale.js";
export { resolveApiError } from "./apierr.js";
export type {
  ApiErrorBody,
  ApiErrorPayload,
  ResolveOptions,
} from "./apierr.js";
