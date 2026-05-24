// Bootstrap — side-effecting imports register the custom elements
// and apply the persisted theme before the first paint so the
// admin doesn't flash light → dark.

import "@glossa/ui/tokens.css";
import { initTheme } from "@glossa/ui";
import "@glossa/ui";
import "@glossa/elements";

initTheme();

document.body.classList.add("gl-root");

import "./admin-app.js";
import "./audit-tab.js";
import "./bulk-tab.js";
import "./diff-tab.js";
import "./editor-tab.js";
import "./key-edit.js";
import "./key-list.js";
import "./locales-tab.js";
import "./users-tab.js";
