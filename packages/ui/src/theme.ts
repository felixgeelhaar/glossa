// Theme toggle. Persists choice to localStorage; falls back to
// prefers-color-scheme when the user hasn't picked. Single source
// of truth: the `data-glossa-theme` attribute on <html>.

export type Theme = "light" | "dark" | "system";

const STORAGE_KEY = "glossa-ui-theme-v1";

export function getTheme(): Theme {
  if (typeof localStorage === "undefined") return "system";
  const v = localStorage.getItem(STORAGE_KEY);
  return v === "light" || v === "dark" ? v : "system";
}

/** Resolve "system" to the actual mode the OS reports. */
export function resolvedTheme(t: Theme = getTheme()): "light" | "dark" {
  if (t !== "system") return t;
  if (typeof matchMedia === "undefined") return "light";
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** Apply the theme to <html> + persist. */
export function setTheme(t: Theme): void {
  if (typeof document === "undefined") return;
  if (t === "system") {
    document.documentElement.removeAttribute("data-glossa-theme");
    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch {
      /* ignore */
    }
    return;
  }
  document.documentElement.setAttribute("data-glossa-theme", t);
  try {
    localStorage.setItem(STORAGE_KEY, t);
  } catch {
    /* ignore */
  }
}

/** Call once on app boot to apply the stored preference. */
export function initTheme(): void {
  const t = getTheme();
  if (t === "system") return; // CSS prefers-color-scheme picks up
  setTheme(t);
}
