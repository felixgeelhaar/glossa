/**
 * Walk an AST against a values map for a given locale.
 *
 * Plural keyword resolution defers to the browser's `Intl.PluralRules`
 * (works across ~100 locales without us shipping CLDR data). Falls
 * back gracefully when `Intl.PluralRules` isn't present (old browsers,
 * test environments) by treating every count as `other`.
 */

import { parse, type Node } from "./parse.js";

export type Values = Record<string, string | number | boolean | null | undefined>;

export class FormatError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "FormatError";
  }
}

/**
 * Format a message string with the given values.
 *
 * Convenience wrapper that parses on every call. For hot paths (e.g.
 * a translation rendered on every keystroke) call `parse` once and
 * use `formatAst` to skip the parse cost.
 */
export function format(
  message: string,
  locale: string,
  values: Values = {},
): string {
  const ast = parse(message);
  return formatAst(ast, locale, values);
}

export function formatAst(
  ast: readonly Node[],
  locale: string,
  values: Values = {},
): string {
  return renderNodes(ast, locale, values, /*poundValue=*/ undefined);
}

function renderNodes(
  nodes: readonly Node[],
  locale: string,
  values: Values,
  poundValue: number | undefined,
): string {
  let out = "";
  for (const node of nodes) {
    out += renderNode(node, locale, values, poundValue);
  }
  return out;
}

function renderNode(
  node: Node,
  locale: string,
  values: Values,
  poundValue: number | undefined,
): string {
  switch (node.type) {
    case "literal":
      return node.value;
    case "var":
      return stringify(values[node.name]);
    case "pound":
      return poundValue === undefined ? "" : String(poundValue);
    case "plural": {
      const raw = values[node.name];
      const n = toNumber(raw);
      // Exact match wins before keyword resolution — `=0 {no items}`
      // beats `zero` / `other`.
      const exactBranch = node.exact[n];
      if (exactBranch !== undefined) {
        return renderNodes(exactBranch, locale, values, n);
      }
      const keyword = pluralKeyword(locale, n);
      const branch =
        node.cases[keyword] ?? node.cases.other ?? [];
      return renderNodes(branch, locale, values, n);
    }
    case "select": {
      const raw = values[node.name];
      const key = stringify(raw);
      const branch = node.cases[key] ?? node.cases.other ?? [];
      return renderNodes(branch, locale, values, poundValue);
    }
  }
}

function stringify(v: unknown): string {
  if (v === null || v === undefined) return "";
  return String(v);
}

function toNumber(v: unknown): number {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string") {
    const n = Number(v);
    return Number.isFinite(n) ? n : 0;
  }
  return 0;
}

function pluralKeyword(locale: string, n: number): string {
  // Intl.PluralRules is widely available in Node 22+ and every evergreen
  // browser. The try/catch guards JSDOM / older environments without
  // forcing a polyfill dependency.
  try {
    const rules = new Intl.PluralRules(locale);
    return rules.select(n);
  } catch {
    return "other";
  }
}
