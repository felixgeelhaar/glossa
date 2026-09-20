/**
 * Validate capture-script output against runtimes/testdata/schemas/captures.v1.schema.json
 * (with usages.v1, which it references), wrapped in a minimal capture
 * document the way `glossa capture` wraps it. Test-only.
 */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { Ajv2020 } from "ajv/dist/2020.js";

import type { Capture } from "../regions.js";

const schemas = join(import.meta.dirname, "../../../../testdata/schemas");
const schema = (name: string) =>
  JSON.parse(readFileSync(join(schemas, name), "utf8")) as Record<string, unknown>;

const ajv = new Ajv2020({ allErrors: true, strict: false });
ajv.addFormat("uri", (s: string) => URL.canParse(s));
ajv.addSchema(schema("usages.v1.schema.json"));
const validate = ajv.compile(schema("captures.v1.schema.json"));

/** The captures.v1 document around one capture's `renders` and `regions`. */
export const document = (c: Capture) => ({
  schema: "glossa.captures/v1",
  application: "web",
  commit: "9f2c1e7a4b3d5c6e8f0a1b2c3d4e5f6a7b8c9d0e",
  branch: "main",
  tool: { name: "glossa", version: "0.0.0" },
  captures: [
    {
      route: "/",
      url: "http://localhost:4173/",
      viewport: { width: 1280, height: 800 },
      locale: "de",
      image: { sha256: "0".repeat(64), width: 1280, height: 800 },
      renders: c.renders,
      regions: c.regions,
    },
  ],
});

/** The schema errors for `c`, or `[]`. */
export function schemaErrors(c: Capture): string[] {
  return validate(document(c))
    ? []
    : (validate.errors ?? []).map((e) => `${e.instancePath} ${e.message ?? ""}`);
}
