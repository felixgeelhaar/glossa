/** Mount a project screen with a project context, a Releases port and a router, as ProjectLayout would. */
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { vi } from "vitest";
import { computed, defineComponent, ref, type Component } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { RELEASES, type ReleasesPort } from "../api/releases";
import type { Role } from "../api/schemas";
import { grantFor } from "../session/permissions";
import { refreshSession } from "../session/session";
import { PROJECT, type ProjectContext } from "../views/project/context";

const Empty = defineComponent({ template: "<div />" });

const ME = {
  person: { id: "me", email: "me@example.com", email_verified: true, totp_enabled: false, individual_tenant_id: "t", created_at: "2026-09-01T00:00:00Z" },
  csrf_token: "csrf",
  memberships: [],
};

export function projectContext(roles: Role[] = ["developer"]): ProjectContext {
  return {
    tenant: computed(() => "t"),
    projectId: computed(() => "p"),
    project: ref(undefined),
    etag: ref(undefined),
    locales: ref([]),
    targets: computed(() => []),
    grant: computed(() => grantFor({ roles, locales: [] })),
    reloadProject: async () => undefined,
    reloadLocales: async () => undefined,
  };
}

export async function mountProjectScreen(
  component: Component,
  options: { port: ReleasesPort; roles?: Role[]; path?: string },
): Promise<VueWrapper> {
  // The signed-in person is `person:me` (the fake port's author); member names come back empty.
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const body = new URL(req.url).pathname === "/v1/me" ? ME : { items: [] };
      return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
    }),
  );
  await refreshSession();
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/t/:tenant/p/:project/releases", name: "releases", component: Empty },
      { path: "/t/:tenant/p/:project/releases/:release", name: "release", component: Empty },
      { path: "/t/:tenant/p/:project/settings", name: "settings", component: Empty },
      { path: "/t/:tenant", name: "projects", component: Empty },
    ],
  });
  await router.push(options.path ?? "/t/t/p/p/releases");
  const w = mount(component, {
    attachTo: document.body,
    global: {
      plugins: [router],
      provide: { [PROJECT as symbol]: projectContext(options.roles), [RELEASES as symbol]: options.port },
    },
  });
  await flushPromises();
  return w;
}
