// CLI exit codes. Mirrored in docs/spec so CI integrations can
// branch on them without parsing stderr.

export const EXIT_OK = 0;
/** Missing / malformed glossa.config — caller mis-configuration. */
export const EXIT_CONFIG = 1;
/** Network or API failure — transient, worth retrying. */
export const EXIT_NETWORK = 2;
/** Per-row scan failures occurred even though the request itself succeeded. */
export const EXIT_PARTIAL = 3;
