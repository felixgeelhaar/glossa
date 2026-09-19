/**
 * zod schemas for the responses Studio trusts: they are checked at the
 * boundary before any view sees them. The compile-time assertions at the
 * bottom tie each schema to the generated contract types, so a contract
 * change that breaks a schema fails `pnpm lint`.
 */
import { z } from "zod";
import type { components } from "./schema";

const timestamp = z.string().min(1);
const id = z.string().min(1);

export const Role = z.enum(["owner", "admin", "developer", "translator", "reviewer"]);
export const Syntax = z.enum(["mf1", "mf2"]);
export const Direction = z.enum(["ltr", "rtl"]);
export const ReviewState = z.enum(["draft", "needs_review", "approved", "rejected"]);
export const Origin = z.enum(["human", "ai", "translation_memory", "machine_translation", "import", "adaptation"]);
export const MessageState = z.enum(["active", "obsolete"]);
export const Platform = z.enum(["web", "api", "ios", "android", "other"]);

export const FieldError = z.object({ pointer: z.string(), detail: z.string() });

export const QAFinding = z.object({
  code: z.string(),
  severity: z.enum(["error", "warning"]),
  subject: z.string().optional(),
  detail: z.string().optional(),
  message: z.string(),
});

export const Problem = z.object({
  type: z.string(),
  title: z.string(),
  status: z.number().int(),
  code: z.string(),
  detail: z.string().optional(),
  instance: z.string().optional(),
  errors: z.array(FieldError).optional(),
  findings: z.array(QAFinding).optional(),
});

export const Person = z.object({
  id,
  email: z.string(),
  display_name: z.string().optional(),
  email_verified: z.boolean(),
  totp_enabled: z.boolean(),
  individual_tenant_id: id,
  created_at: timestamp,
});

export const Session = z.object({ person: Person, csrf_token: z.string().min(1), expires_at: timestamp });

export const Tenant = z.object({
  id,
  kind: z.enum(["individual", "organization"]),
  slug: z.string(),
  name: z.string(),
  created_at: timestamp,
});

export const Membership = z.object({
  member_id: id,
  tenant: Tenant,
  roles: z.array(Role),
  locales: z.array(z.string()),
});

export const Me = z.object({ person: Person, csrf_token: z.string().min(1), memberships: z.array(Membership) });

export const Passkey = z.object({ id: z.string(), name: z.string(), created_at: timestamp });
export const PasskeyOptions = z.object({ options: z.record(z.string(), z.unknown()) });
export const TotpEnrollment = z.object({ secret: z.string(), otpauth_uri: z.string() });

export const ProjectSettings = z.object({ default_syntax: Syntax, review_required: z.boolean() });
export const Project = z.object({
  id,
  slug: z.string(),
  name: z.string(),
  source_locale: z.string(),
  settings: ProjectSettings,
  created_at: timestamp,
  updated_at: timestamp,
});

export const Application = z.object({
  id,
  slug: z.string(),
  name: z.string(),
  platform: Platform,
  created_at: timestamp,
  updated_at: timestamp,
});

/** The MF2 data model; its structure is checked by the formatter itself. */
export const MF2Message = z.looseObject({ type: z.enum(["message", "select"]) });

export const Argument = z.object({
  name: z.string(),
  type: z.enum(["string", "number", "integer", "percent", "currency", "date", "time", "datetime", "unit", "select"]),
  function: z.string().optional(),
  selector: z
    .object({ kind: z.enum(["plural", "ordinal", "exact", "string"]), keys: z.array(z.string()) })
    .optional(),
});

export const MarkupElement = z.object({ name: z.string(), kind: z.enum(["open", "standalone", "close"]) });

export const MessageContent = z.object({
  text: z.string(),
  syntax: Syntax,
  model: MF2Message,
  arguments: z.array(Argument),
  markup: z.array(MarkupElement),
});

export const Message = z.object({
  id,
  key: z.string(),
  namespace: z.string(),
  description: z.string(),
  max_length: z.number().int().optional(),
  state: MessageState,
  source: MessageContent,
  source_revision: z.number().int(),
  created_at: timestamp,
  updated_at: timestamp,
});

export const SourceRevision = z.object({
  revision: z.number().int(),
  text: z.string(),
  syntax: Syntax,
  model: MF2Message,
  author: z.string(),
  created_at: timestamp,
});

export const ProjectLocale = z.object({
  code: z.string(),
  direction: Direction,
  is_source: z.boolean(),
  created_at: timestamp,
});

export const FallbackGraph = z.object({ fallback: z.record(z.string(), z.array(z.string())) });

export const Translation = z.object({
  id,
  message_id: id,
  locale: z.string(),
  text: z.string(),
  syntax: Syntax,
  model: MF2Message,
  state: ReviewState,
  origin: Origin,
  author: z.string(),
  source_revision: z.number().int(),
  current_source_revision: z.number().int(),
  outdated: z.boolean(),
  warnings: z.array(QAFinding),
  revision: z.number().int(),
  created_at: timestamp,
  updated_at: timestamp,
});

export const TranslationRevision = z.object({
  revision: z.number().int(),
  kind: z.enum(["content", "review"]),
  text: z.string(),
  syntax: Syntax,
  state: ReviewState,
  origin: Origin,
  origin_detail: z.record(z.string(), z.unknown()),
  author: z.string(),
  source_revision: z.number().int(),
  findings: z.array(QAFinding),
  created_at: timestamp,
});

const ItemError = z.object({ code: z.string(), detail: z.string(), findings: z.array(QAFinding).optional() });
export const MessageUpsertResult = z.object({
  results: z.array(
    z.object({
      key: z.string(),
      status: z.enum(["created", "revised", "updated", "unchanged", "failed"]),
      message: Message.optional(),
      error: ItemError.optional(),
    }),
  ),
});

/** A page of a list operation. */
export const page = <T extends z.ZodType>(item: T) =>
  z.object({ items: z.array(item), next_page_token: z.string().optional() });

export type Role = z.infer<typeof Role>;
export type Syntax = z.infer<typeof Syntax>;
export type ReviewState = z.infer<typeof ReviewState>;
export type Problem = z.infer<typeof Problem>;
export type QAFinding = z.infer<typeof QAFinding>;
export type Person = z.infer<typeof Person>;
export type Session = z.infer<typeof Session>;
export type Tenant = z.infer<typeof Tenant>;
export type Membership = z.infer<typeof Membership>;
export type Me = z.infer<typeof Me>;
export type Passkey = z.infer<typeof Passkey>;
export type TotpEnrollment = z.infer<typeof TotpEnrollment>;
export type Project = z.infer<typeof Project>;
export type ProjectSettings = z.infer<typeof ProjectSettings>;
export type Application = z.infer<typeof Application>;
export type Argument = z.infer<typeof Argument>;
export type MarkupElement = z.infer<typeof MarkupElement>;
export type Message = z.infer<typeof Message>;
export type SourceRevision = z.infer<typeof SourceRevision>;
export type ProjectLocale = z.infer<typeof ProjectLocale>;
export type FallbackGraph = z.infer<typeof FallbackGraph>;
export type Translation = z.infer<typeof Translation>;
export type TranslationRevision = z.infer<typeof TranslationRevision>;
export type MessageUpsertResult = z.infer<typeof MessageUpsertResult>;
export type Platform = z.infer<typeof Platform>;

// ── contract alignment (compile time only) ─────────────────────────────
type S = components["schemas"];
type Fits<A, B> = [A] extends [B] ? true : false;
type Assert<T extends true> = T;
export type ContractAlignment = [
  Assert<Fits<Problem, S["Problem"]>>,
  Assert<Fits<Session, S["Session"]>>,
  Assert<Fits<Me, S["Me"]>>,
  Assert<Fits<Project, S["Project"]>>,
  Assert<Fits<Application, S["Application"]>>,
  Assert<Fits<Argument, S["Argument"]>>,
  Assert<Fits<Message, S["Message"]>>,
  Assert<Fits<SourceRevision, S["SourceRevision"]>>,
  Assert<Fits<ProjectLocale, S["ProjectLocale"]>>,
  Assert<Fits<FallbackGraph, S["FallbackGraph"]>>,
  Assert<Fits<Translation, S["Translation"]>>,
  Assert<Fits<TranslationRevision, S["TranslationRevision"]>>,
  Assert<Fits<MessageUpsertResult, S["MessageUpsertResult"]>>,
  Assert<Fits<Passkey, S["Passkey"]>>,
  Assert<Fits<TotpEnrollment, S["TotpEnrollment"]>>,
];
