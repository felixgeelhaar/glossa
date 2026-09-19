import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { computed, ref } from "vue";
import { comingSoonReleases, RELEASES, type ReleasesPort } from "../../api/releases";
import { grantFor } from "../../session/permissions";
import { PROJECT, type ProjectContext } from "./context";
import ReleasesView from "./ReleasesView.vue";

const context = (): ProjectContext => ({
  tenant: computed(() => "t"),
  projectId: computed(() => "p"),
  project: ref(undefined),
  etag: ref(undefined),
  locales: ref([]),
  targets: computed(() => []),
  grant: computed(() => grantFor({ roles: ["developer"], locales: [] })),
  reloadProject: async () => undefined,
  reloadLocales: async () => undefined,
});

describe("ReleasesView", () => {
  it("shows a clear coming-soon state until the release endpoints land", () => {
    const w = mount(ReleasesView, { global: { provide: { [PROJECT as symbol]: context(), [RELEASES as symbol]: comingSoonReleases } } });
    expect(w.get("[data-testid=releases-coming-soon]").text()).toContain("Releases are coming soon");
    expect(w.find("table").exists()).toBe(false);
  });

  it("lists and publishes through the port once one is available", async () => {
    const port: ReleasesPort = {
      availability: "available",
      list: vi.fn().mockResolvedValue([{ id: "r1", name: "2026.09.19-1", createdAt: "2026-09-19T12:00:00Z", environments: ["production"], locales: 3, messages: 42 }]),
      publish: vi.fn().mockResolvedValue(undefined),
    };
    const w = mount(ReleasesView, { global: { provide: { [PROJECT as symbol]: context(), [RELEASES as symbol]: port } } });
    await flushPromises();
    expect(w.get("tbody").text()).toContain("2026.09.19-1");
    await w.get("button").trigger("click");
    await flushPromises();
    expect(port.publish).toHaveBeenCalledWith({ tenant: "t", project: "p" });
    expect(port.list).toHaveBeenCalledTimes(2);
  });
});
