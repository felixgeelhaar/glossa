import { createRouter, createWebHistory, type RouteRecordRaw, type Router } from "vue-router";
import { homeTenant, membershipFor, refreshSession, rememberTenant, useSession } from "./session/session";

declare module "vue-router" {
  interface RouteMeta {
    /** Reachable without a session. */
    public?: boolean;
    title?: string;
  }
}

export const routes: RouteRecordRaw[] = [
  { path: "/auth/sign-in", name: "sign-in", component: () => import("./views/auth/SignInView.vue"), meta: { public: true, title: "Sign in" } },
  { path: "/auth/register", name: "register", component: () => import("./views/auth/RegisterView.vue"), meta: { public: true, title: "Create an account" } },
  { path: "/auth/reset-password", name: "reset-password", component: () => import("./views/auth/ResetPasswordView.vue"), meta: { public: true, title: "Reset password" } },
  {
    path: "/",
    component: () => import("./components/AppShell.vue"),
    children: [
      { path: "", name: "home", redirect: () => ({ name: "projects", params: { tenant: homeTenant() ?? "none" } }) },
      { path: "account", name: "account", component: () => import("./views/AccountView.vue"), meta: { title: "Account & security" } },
      { path: "organizations/new", name: "new-organization", component: () => import("./views/NewOrganizationView.vue"), meta: { title: "New organization" } },
      { path: "t/:tenant", name: "projects", component: () => import("./views/ProjectsView.vue"), meta: { title: "Projects" } },
      {
        path: "t/:tenant/p/:project",
        component: () => import("./views/project/ProjectLayout.vue"),
        children: [
          { path: "", name: "project", redirect: (to) => ({ name: "translate", params: to.params }) },
          { path: "translate", name: "translate", component: () => import("./views/project/WorkspaceView.vue"), meta: { title: "Translate" } },
          { path: "locales", name: "locales", component: () => import("./views/project/LocalesView.vue"), meta: { title: "Locales" } },
          { path: "settings", name: "settings", component: () => import("./views/project/SettingsView.vue"), meta: { title: "Settings" } },
          { path: "releases", name: "releases", component: () => import("./views/project/ReleasesView.vue"), meta: { title: "Releases" } },
        ],
      },
      { path: ":pathMatch(.*)*", name: "not-found", component: () => import("./views/NotFoundView.vue"), meta: { title: "Not found" } },
    ],
  },
];

export function installGuards(router: Router): void {
  const { status } = useSession();
  router.beforeEach(async (to) => {
    if (status.value === "unknown") await refreshSession().catch(() => null);
    const signedIn = status.value === "authenticated";
    if (to.meta.public) {
      // A signed-in person landing on sign-in without a link goes home.
      if (signedIn && to.name === "sign-in" && !to.hash.includes("token=")) return { name: "home" };
      return true;
    }
    if (!signedIn) return { name: "sign-in", query: to.fullPath === "/" ? {} : { next: to.fullPath } };
    const tenant = typeof to.params.tenant === "string" ? to.params.tenant : undefined;
    if (tenant) {
      if (!membershipFor(tenant)) {
        const home = homeTenant();
        return home && home !== tenant ? { name: "projects", params: { tenant: home } } : true;
      }
      rememberTenant(tenant);
    }
    return true;
  });
  router.afterEach((to) => {
    document.title = to.meta.title ? `${to.meta.title} · Glossa Studio` : "Glossa Studio";
  });
}

export function createStudioRouter(): Router {
  const router = createRouter({ history: createWebHistory(), routes });
  installGuards(router);
  return router;
}
