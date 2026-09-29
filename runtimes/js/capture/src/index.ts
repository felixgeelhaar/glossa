export { startCapture, stripMarkers, digest } from "./session.js";
export type { CaptureSession, LoggedRender, SessionCapture } from "./session.js";
export { collectRegions } from "./regions.js";
export type { Box, Capture, CapturedRender, Host, LogEntry, Region } from "./regions.js";
export { FINDING, probe } from "./probes.js";
export type {
  Baseline,
  ProbeContext,
  ProbeFinding,
  ProbeLocus,
  ProbeOptions,
  ProbeResult,
} from "./probes.js";
export { END, START, mark, ranges, strip } from "./markers.js";
export type { Marked } from "./markers.js";
