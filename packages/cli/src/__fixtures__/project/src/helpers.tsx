// Template-literal-style call sites the scanner should pick up
// in addition to the <glossa-*> elements.

import { t, i18n, formatMessage, client, useGlossa } from "./fakeapi";

export function Buttons() {
  return (
    <>
      <button>{t("buttons.save")}</button>
      <button>{i18n.t("buttons.cancel")}</button>
      <button>{formatMessage({ id: "buttons.delete" })}</button>
      <button>{client.message("de", "buttons.copy")}</button>
      <button>{useGlossa("buttons.share")}</button>
    </>
  );
}
