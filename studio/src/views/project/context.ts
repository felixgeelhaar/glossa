/**
 * What every project screen shares: the project (with its ETag), its
 * locales and the member's grant. ProjectLayout loads it once and
 * provides it; screens inject it.
 */
import { inject, type ComputedRef, type InjectionKey, type Ref } from "vue";
import type { Project, ProjectLocale } from "../../api/schemas";
import type { Grant } from "../../session/permissions";

export interface ProjectContext {
  tenant: ComputedRef<string>;
  projectId: ComputedRef<string>;
  project: Ref<Project | undefined>;
  etag: Ref<string | undefined>;
  locales: Ref<ProjectLocale[]>;
  /** Target locales: every locale but the source. */
  targets: ComputedRef<ProjectLocale[]>;
  grant: ComputedRef<Grant>;
  reloadProject(): Promise<void>;
  reloadLocales(): Promise<void>;
}

export const PROJECT: InjectionKey<ProjectContext> = Symbol("project");

export function useProject(): ProjectContext {
  const ctx = inject(PROJECT);
  if (!ctx) throw new Error("useProject() outside ProjectLayout");
  return ctx;
}
