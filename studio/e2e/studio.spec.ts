import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { signInLink } from "./harness";

/** With STUDIO_SCREENSHOTS=<dir>, keeps a full-page screenshot of each screen checked (for design review). */
async function snap(page: Page, name: string): Promise<void> {
  const dir = process.env.STUDIO_SCREENSHOTS;
  if (dir) await page.screenshot({ path: `${dir}/${name}.png`, fullPage: true });
}

/** WCAG 2.2 AA through axe; any violation fails with its rule ids and targets. */
async function expectAccessible(page: Page, screen: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"]).analyze();
  const summary = violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`);
  expect(summary, `axe violations on ${screen}`).toEqual([]);
  await snap(page, screen.replace(/\W+/g, "-"));
}

test("sign in by magic link, set up a project, translate with the keyboard, fix QA, approve", async ({ page }) => {
  const email = `translator-${Date.now()}@example.com`;

  // ── sign in with the link the dev mailer captured ──────────────────
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Sign in to Glossa" })).toBeVisible();
  await expect(page.getByText("Public Beta").first()).toBeVisible();
  await expectAccessible(page, "sign-in");
  await page.getByLabel("Email address").fill(email);
  await page.getByRole("button", { name: "Email me a sign-in link" }).click();
  await expect(page.getByRole("heading", { name: "Check your inbox" })).toBeVisible();

  await page.goto(await signInLink(email));
  await expect(page.getByRole("heading", { name: "Projects" })).toBeVisible();
  expect(page.url()).not.toContain("token=");
  await expectAccessible(page, "projects");

  // ── create a project ───────────────────────────────────────────────
  await page.getByRole("button", { name: "New project" }).click();
  await page.getByLabel("Name", { exact: true }).fill("Demo App");
  await page.getByLabel("Source locale").fill("en");
  await expect(page.getByLabel("Translations need review before release")).toBeChecked();
  await page.getByRole("button", { name: "Create project" }).click();

  // ── add locales (BCP 47, canonicalized, direction shown) ───────────
  await expect(page.getByRole("heading", { name: "Locales", exact: true })).toBeVisible();
  const add = page.getByLabel("Add locale");
  await add.fill("de_de");
  await expect(page.getByText("Adds de-DE — German (Germany), written left to right.")).toBeVisible();
  await add.fill("de");
  await page.getByRole("button", { name: "Add locale" }).click();
  await expect(page.getByRole("cell", { name: "de", exact: true })).toBeVisible();
  await add.fill("ar");
  await page.getByRole("button", { name: "Add locale" }).click();
  await expect(page.getByRole("row", { name: /ar .*Right to left/ })).toBeVisible();
  await add.fill("en-u-ca-gregory");
  await expect(page.getByText("Extensions (-u-, -t-) and private use (-x-) aren't allowed.")).toBeVisible();
  await add.fill("");
  await expectAccessible(page, "locales");

  // ── import a small catalog through the API (as the CLI would) ──────
  const projectId = new URL(page.url()).pathname.split("/")[4]!;
  const tenantId = new URL(page.url()).pathname.split("/")[2]!;
  const me = await (await page.request.get("/v1/me")).json();
  const upsert = await page.request.post(`/v1/tenants/${tenantId}/projects/${projectId}/message-upserts`, {
    headers: { "X-CSRF-Token": me.csrf_token },
    data: {
      items: [
        { key: "app.title", text: "Glossa demo", description: "Browser tab title" },
        { key: "cart.items", text: "{count, plural, one {# item} other {# items}}", description: "Cart badge" },
        { key: "greeting", text: "Hello, {name}!", description: "Shown on the dashboard after sign-in" },
      ],
    },
  });
  expect(upsert.ok()).toBe(true);
  expect((await upsert.json()).results.map((r: { status: string }) => r.status)).toEqual(["created", "created", "created"]);
  // Coverage filters see new messages once Localization has processed their events.
  await expect
    .poll(async () => (await (await page.request.get(`/v1/tenants/${tenantId}/projects/${projectId}/messages?missing_in=de`)).json()).items.length)
    .toBe(3);

  // ── the translator workspace, keyboard first ───────────────────────
  await page.getByRole("link", { name: "Translate" }).click();
  await page.getByLabel("Target locale").selectOption("de");
  const list = page.getByRole("listbox", { name: "Messages" });
  await expect(list.getByRole("option")).toHaveCount(3);
  await expect(page.getByRole("heading", { name: "app.title" })).toBeVisible();
  await expect(list.getByRole("option", { name: /app\.title/ })).toContainText("Missing");

  await page.keyboard.press("j");
  await expect(page.getByRole("heading", { name: "cart.items" })).toBeVisible();
  await page.keyboard.press("j");
  await expect(page.getByRole("heading", { name: "greeting" })).toBeVisible();
  await expect(page.getByText("Shown on the dashboard after sign-in")).toBeVisible();
  await expect(page.getByTestId("preview-source")).toHaveText("Hello, Ada!");

  // A broken placeholder: the server's structural QA refuses it.
  await page.keyboard.press("Enter");
  const editor = page.getByTestId("target-editor");
  await expect(editor).toBeFocused();
  await expect(editor).toHaveAttribute("lang", "de");
  await expect(editor).toHaveAttribute("dir", "ltr");
  await editor.fill("Hallo, {nam}!");
  await page.keyboard.press("ControlOrMeta+Enter");
  const qa = page.getByTestId("qa-findings");
  await expect(qa).toBeVisible();
  await expect(qa).toContainText("missing-argument");
  await expect(editor).toHaveAttribute("aria-invalid", "true");
  await expectAccessible(page, "workspace with QA findings");

  // Fixed, then saved and approved in one chord.
  await editor.fill("Hallo, {name}!");
  await page.keyboard.press("ControlOrMeta+Shift+Enter");
  await expect(page.getByTestId("editor-status")).toHaveText("Approved.");
  await expect(page.getByTestId("translation-state")).toHaveText("Approved");
  await expect(qa).toBeHidden();
  await expect(page.getByTestId("preview-target")).toHaveText("Hallo, Ada!");

  // Save only: back to the list, translate the plural, which waits for review.
  await page.keyboard.press("Escape");
  await expect(list).toBeFocused();
  await page.keyboard.press("k");
  await expect(page.getByRole("heading", { name: "cart.items" })).toBeVisible();
  await page.keyboard.press("Enter");
  await editor.fill("{count, plural, one {# Artikel} other {# Artikel}}");
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(page.getByTestId("editor-status")).toHaveText("Saved.");
  await expect(page.getByTestId("translation-state")).toHaveText("Needs review");

  // …and a reviewer approves it with the button.
  await page.getByRole("button", { name: "Approve", exact: true }).click();
  await expect(page.getByTestId("translation-state")).toHaveText("Approved");

  // History shows both revisions with provenance.
  await page.getByRole("tab", { name: "History" }).click();
  await expect(page.getByRole("list", { name: "Revisions, newest first" }).getByRole("listitem")).toHaveCount(2);
  await expectAccessible(page, "workspace history");

  // The shortcut sheet.
  await page.getByRole("tab", { name: "Editor" }).focus();
  await page.keyboard.press("?");
  await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
  await expectAccessible(page, "shortcut sheet");
  await page.keyboard.press("Escape");

  // ── releases: a clear coming-soon state ────────────────────────────
  await page.getByRole("link", { name: "Releases" }).click();
  await expect(page.getByTestId("releases-coming-soon")).toBeVisible();
});

test("dark theme stays accessible", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("kl-theme", "dark"));
  await page.goto("/auth/sign-in");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expectAccessible(page, "sign-in (dark)");
});
