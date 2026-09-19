/** Mount a project screen with a project context, the ports it uses and a router, as ProjectLayout would. */
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { vi } from "vitest";
import { computed, defineComponent, ref, type Component } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { INTEGRATION, type IntegrationPort } from "../api/integration";
import { INTELLIGENCE, type IntelligencePort } from "../api/intelligence";
import { KNOWLEDGE, type KnowledgePort } from "../api/knowledge";
import { RELEASES, type ReleasesPort } from "../api/releases";
import type { Project, ProjectLocale, Role } from "../api/schemas";
import { grantFor } from "../session/permissions";
import { refreshSession } from "../session/session";
import { PROJECT, type ProjectContext } from "../views/project/context";

const Empty = defineComponent({ template: "<div />" });
const ROUTE_NAMES: Record<string, string> = { "releases/:release": "release", "files/import": "import", "files/imports/:job": "import-job" };

const ME = {
  person: { id: "me", email: "me@example.com", email_verified: true, totp_enabled: false, individual_tenant_id: "t", created_at: "2026-09-01T00:00:00Z" },
  csrf_token: "csrf",
  memberships: [],
};

const NOW = "2026-09-19T08:00:00Z";
export const locale = (code: string, is_source = false): ProjectLocale => ({ code, direction: "ltr", is_source, created_at: NOW });
export const demoProject: Project = {
  id: "p",
  slug: "demo",
  name: "Demo",
  source_locale: "en",
  settings: { default_syntax: "mf1", review_required: true },
  created_at: NOW,
  updated_at: NOW,
};

export function projectContext(roles: Role[] = ["developer"], locales: ProjectLocale[] = [], memberLocales: string[] = []): ProjectContext {
  const all = ref(locales);
  return {
    tenant: computed(() => "t"),
    projectId: computed(() => "p"),
    project: ref(locales.length ? demoProject : undefined),
    etag: ref(undefined),
    locales: all,
    targets: computed(() => all.value.filter((l) => !l.is_source)),
    grant: computed(() => grantFor({ roles, locales: memberLocales })),
    reloadProject: async () => undefined,
    reloadLocales: async () => undefined,
  };
}

export interface ScreenOptions {
  port?: ReleasesPort;
  knowledge?: KnowledgePort;
  intelligence?: IntelligencePort;
  integration?: IntegrationPort;
  roles?: Role[];
  /** Locale scope of the member (translators, reviewers). */
  memberLocales?: string[];
  locales?: ProjectLocale[];
  path?: string;
  /** Answers for fetch calls that don't go through a port (by path). */
  fetch?: (path: string) => unknown;
}

export async function mountProjectScreen(component: Component, options: ScreenOptions): Promise<VueWrapper> {
  // The signed-in person is `person:me` (the fake ports' author); member names come back empty.
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      const body = path === "/v1/me" ? ME : (options.fetch?.(path) ?? { items: [] });
      return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
    }),
  );
  await refreshSession();
  const router = createRouter({
    history: createMemoryHistory(),
    routes: ["releases", "releases/:release", "settings", "translate", "review", "terms", "style", "ai", "files", "files/import", "files/imports/:job"]
      .map((p) => ({ path: `/t/:tenant/p/:project/${p}`, name: ROUTE_NAMES[p] ?? p, component: Empty }))
      .concat([
        { path: "/t/:tenant", name: "projects", component: Empty },
        { path: "/t/:tenant/settings/knowledge", name: "workspace-knowledge", component: Empty },
      ]),
  });
  await router.push(options.path ?? "/t/t/p/p/releases");
  const provide: Record<symbol, unknown> = { [PROJECT as symbol]: projectContext(options.roles, options.locales, options.memberLocales) };
  if (options.port) provide[RELEASES as symbol] = options.port;
  if (options.knowledge) provide[KNOWLEDGE as symbol] = options.knowledge;
  if (options.intelligence) provide[INTELLIGENCE as symbol] = options.intelligence;
  if (options.integration) provide[INTEGRATION as symbol] = options.integration;
  const w = mount(component, { attachTo: document.body, global: { plugins: [router], provide } });
  await flushPromises();
  return w;
}

export interface TenantScreenOptions {
  integration?: IntegrationPort;
  roles?: Role[];
  path: string;
}

/** Mount a workspace (tenant-level) screen: the member's grant comes from the session, as in the app. */
export async function mountTenantScreen(component: Component, options: TenantScreenOptions): Promise<VueWrapper> {
  const me = {
    ...ME,
    memberships: [
      {
        member_id: "m",
        tenant: { id: "t", kind: "organization", slug: "acme", name: "Acme", created_at: NOW },
        roles: options.roles ?? ["developer"],
        locales: [],
      },
    ],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      return new Response(JSON.stringify(path === "/v1/me" ? me : { items: [] }), { status: 200, headers: { "Content-Type": "application/json" } });
    }),
  );
  await refreshSession();
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/t/:tenant", name: "projects", component: Empty },
      { path: "/t/:tenant/settings/knowledge", name: "workspace-knowledge", component: Empty },
      { path: "/t/:tenant/settings/knowledge/imports/:job", name: "workspace-import-job", component: Empty },
    ],
  });
  await router.push(options.path);
  const provide: Record<symbol, unknown> = {};
  if (options.integration) provide[INTEGRATION as symbol] = options.integration;
  const w = mount(component, { attachTo: document.body, global: { plugins: [router], provide } });
  await flushPromises();
  return w;
}
