import { describe, expect, it } from "vitest";
import { configuredEdge, maskKey, snippets } from "./snippets";

const key = "glossa_pk_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345";
const base = { edge: "https://edge.example.com", deliveryKey: key, environment: "production", publicKeys: [] };

describe("snippets", () => {
  it("covers every runtime with the key, edge and environment", () => {
    const list = snippets(base);
    expect(list.map((s) => s.id)).toEqual(["runtime", "vue", "elements", "go"]);
    for (const s of list) {
      expect(s.code).toContain(key);
      expect(s.code).toContain("https://edge.example.com");
      expect(s.code).toContain("production");
    }
  });

  it("uses each runtime's own API", () => {
    const [runtime, vue, elements, go] = snippets(base);
    expect(runtime!.code).toContain('import { createRuntime } from "@glossa/runtime";');
    expect(vue!.code).toContain(".use(\n    createGlossa({");
    expect(elements!.code).toContain(`<glossa-provider\n  edge="https://edge.example.com"\n  delivery-key="${key}"`);
    expect(go!.code).toContain("glossa.New(glossa.Config{");
    expect(go!.code).toContain(`DeliveryKey: "${key}",`);
    expect(go!.code).not.toContain("PublicKeys");
  });

  it("pins signing keys when there are any", () => {
    const [runtime, , elements, go] = snippets({ ...base, environment: "staging", publicKeys: [{ keyId: "k1", key: "pub1" }] });
    expect(runtime!.code).toContain('publicKeys: [{ keyId: "k1", key: "pub1" }],');
    expect(runtime!.code).toContain('environment: "staging",');
    expect(elements!.code).toContain('public-keys="k1:pub1"');
    expect(go!.code).toContain('key1, err := glossa.ParsePublicKey("k1", "pub1")');
    expect(go!.code).toContain("PublicKeys:  []glossa.PublicKey{key1},");
  });

  it("escapes attribute values in markup", () => {
    const [, , elements] = snippets({ ...base, edge: 'https://e.example/"x' });
    expect(elements!.code).toContain('edge="https://e.example/&quot;x"');
  });
});

it("reads the edge origin from the build", () => {
  expect(configuredEdge({ VITE_GLOSSA_EDGE_URL: "https://edge.acme.dev/ " })).toBe("https://edge.acme.dev");
  expect(configuredEdge({})).toBeUndefined();
});

it("masks a key to its recognisable start", () => {
  expect(maskKey(key)).toBe("glossa_pk_ABCD…");
});
