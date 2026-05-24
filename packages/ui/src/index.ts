// Side-effect imports register the custom elements. Importing
// `@felixgeelhaar/glossa-ui` once at app boot is enough.
import "./button.js";
import "./input.js";
import "./select.js";
import "./textarea.js";
import "./card.js";
import "./badge.js";
import "./table.js";
import "./tabs.js";
import "./toast.js";
import "./toolbar.js";
import "./theme-toggle.js";

export { GlButton } from "./button.js";
export type { ButtonVariant, ButtonSize } from "./button.js";
export { GlInput } from "./input.js";
export { GlSelect } from "./select.js";
export type { GlSelectOption } from "./select.js";
export { GlTextarea } from "./textarea.js";
export { GlCard } from "./card.js";
export { GlBadge } from "./badge.js";
export type { BadgeVariant } from "./badge.js";
export { GlTable, glTableStyles } from "./table.js";
export { GlTabs } from "./tabs.js";
export { GlToast, toast } from "./toast.js";
export type { ToastVariant } from "./toast.js";
export { GlToolbar } from "./toolbar.js";
export { GlThemeToggle } from "./theme-toggle.js";
export { getTheme, setTheme, resolvedTheme, initTheme } from "./theme.js";
export type { Theme } from "./theme.js";
