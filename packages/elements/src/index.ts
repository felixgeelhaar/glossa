// Side-effecting imports — defining the custom elements is the
// whole point of importing this package.
import "./glossa-provider.js";
import "./glossa-text.js";
import "./glossa-rich.js";
import "./glossa-plural.js";
import "./glossa-select.js";
import "./glossa-selector.js";

export { GlossaProvider } from "./glossa-provider.js";
export { GlossaText } from "./glossa-text.js";
export { GlossaRich } from "./glossa-rich.js";
export { GlossaPlural } from "./glossa-plural.js";
export { GlossaSelect } from "./glossa-select.js";
export { GlossaSelector, pickBestMatch } from "./glossa-selector.js";
export type { GlossaLocaleChangeDetail } from "./glossa-selector.js";
export { glossaContext } from "./context.js";
export type { GlossaContextValue } from "./context.js";
