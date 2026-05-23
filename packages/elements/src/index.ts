// Side-effecting imports — defining the custom elements is the
// whole point of importing this package.
import "./glossa-provider.js";
import "./glossa-text.js";
import "./glossa-rich.js";
import "./glossa-plural.js";
import "./glossa-select.js";

export { GlossaProvider } from "./glossa-provider.js";
export { GlossaText } from "./glossa-text.js";
export { GlossaRich } from "./glossa-rich.js";
export { GlossaPlural } from "./glossa-plural.js";
export { GlossaSelect } from "./glossa-select.js";
export { glossaContext } from "./context.js";
export type { GlossaContextValue } from "./context.js";
