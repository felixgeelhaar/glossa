/**
 * The Releases port. The /v1 release endpoints are being added to the
 * contract separately; until they land, Studio runs the "coming soon"
 * adapter. Wiring the real one means implementing ReleasesPort over the
 * generated client (list → GET …/releases, publish → POST …/releases)
 * and providing it in main.ts — the view already handles both states.
 */
import type { InjectionKey } from "vue";
import { ApiError } from "./errors";

export interface ProjectRef {
  tenant: string;
  project: string;
}

export interface ReleaseSummary {
  id: string;
  /** What people call it: a version or sequence number. */
  name: string;
  createdAt: string;
  /** Environments currently pointing at this release. */
  environments: string[];
  locales: number;
  messages: number;
}

export type ReleasesAvailability = "coming-soon" | "available";

export interface ReleasesPort {
  readonly availability: ReleasesAvailability;
  list(project: ProjectRef): Promise<ReleaseSummary[]>;
  publish(project: ProjectRef): Promise<ReleaseSummary>;
}

export const comingSoonReleases: ReleasesPort = {
  availability: "coming-soon",
  list: async () => [],
  publish: async () => {
    throw new ApiError(501, "not_implemented", "Releases aren't available yet.");
  },
};

export const RELEASES: InjectionKey<ReleasesPort> = Symbol("releases");
