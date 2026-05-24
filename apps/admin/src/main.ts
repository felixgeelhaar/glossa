// Bootstrap — side-effecting imports register the custom elements
// and apply the persisted theme before the first paint so the
// admin doesn't flash light → dark.

import "@felixgeelhaar/glossa-ui/tokens.css";
import { initTheme } from "@felixgeelhaar/glossa-ui";
import "@felixgeelhaar/glossa-ui";
import "@felixgeelhaar/glossa-elements";

initTheme();

document.body.classList.add("gl-root");

import "./admin-app.js";
import "./ai-providers-tab.js";
import "./audit-tab.js";
import "./bulk-tab.js";
import "./diff-tab.js";
import "./editor-tab.js";
import "./key-edit.js";
import "./key-list.js";
import "./keys-tab.js";
import "./locales-tab.js";
import "./users-tab.js";
