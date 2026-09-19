/**
 * ICU MessageFormat 1 → canonical MF2 data model.
 *
 * Built on the reference conversion (`mf1ToMessageData`), then canonicalized
 * so MF1 lands on *standard* MF2 functions wherever one is equivalent:
 *
 * | MF1                                    | MF2                                        |
 * |----------------------------------------|--------------------------------------------|
 * | `{n, number}` / `integer`              | `:number` / `:integer`                     |
 * | `{n, number, percent}`, `::percent scale/100` | `:percent`                          |
 * | `{n, number, ::currency/EUR}`          | `:currency currency=EUR`                   |
 * | `{n, number, ::measure-unit/…}`        | `:unit unit=…`                             |
 * | `{d, date[, short/medium/long/full]}`  | `:date [length=…] [fields=…]`              |
 * | `{t, time[, short/medium/long/full]}`  | `:time precision=… [timeZoneStyle=…]`      |
 * | `select`                               | `:string` selector                         |
 * | `plural` / `selectordinal`             | `:number` / `:number select=ordinal`       |
 * | `plural, offset:N`                     | `:number` + `.local $x_offset = {$x :offset subtract=N}` |
 *
 * Everything else keeps an `mf1:`-namespaced function (`mf1:currency` without
 * a currency code, `mf1:number` for notations/scales, `mf1:date` for
 * skeletons, `spellout`, `duration`, …). The original argument type and style
 * stay on the expression as `@mf1:argType` / `@mf1:argStyle` attributes, so
 * the message can be exported back to MF1.
 */
import { mf1ToMessageData } from "@messageformat/icu-messageformat-1";
import { parse } from "@messageformat/parser";
import type { PluralCategory, Token } from "@messageformat/parser";
import { fromReference } from "./convert.js";
import type {
  Declaration,
  Expression,
  FunctionRef,
  Message,
  Pattern,
  SelectMessage,
} from "./model.js";

/** A syntax error (or a plural key the locale never uses) in an MF1 message. */
export class MF1SyntaxError extends Error {
  override name = "MF1SyntaxError";
}

type Tokens = Token[];

/**
 * Parse an ICU MessageFormat 1 message into the canonical data model.
 * `locale` decides which plural and ordinal keys are valid.
 *
 * @throws MF1SyntaxError
 */
export function parseMF1(src: string, locale: string): Message {
  let ast: Tokens;
  try {
    ast = parse(src, {
      cardinal: categories(locale, "cardinal"),
      ordinal: categories(locale, "ordinal"),
    });
  } catch (error) {
    throw new MF1SyntaxError(error instanceof Error ? error.message : String(error), {
      cause: error,
    });
  }
  const offsetNames = offsetVariables(ast);
  const msg = fromReference(mf1ToMessageData(ast as Parameters<typeof mf1ToMessageData>[0]));
  return canonicalize(msg, offsetNames);
}

function categories(locale: string, type: Intl.PluralRuleType): PluralCategory[] {
  return new Intl.PluralRules(locale, { type }).resolvedOptions()
    .pluralCategories as PluralCategory[];
}

// ── plural offsets ────────────────────────────────────────────────────

const isSelect = (t: Token) =>
  t.type === "plural" || t.type === "select" || t.type === "selectordinal";

function argNames(tokens: Tokens, names = new Set<string>()): Set<string> {
  for (const t of tokens) {
    if (t.type === "argument" || t.type === "function") names.add(t.arg);
    if (t.type === "function" && t.param) argNames(t.param, names);
    if (isSelect(t) && "cases" in t) {
      names.add(t.arg);
      for (const c of t.cases) argNames(c.tokens, names);
    }
  }
  return names;
}

/**
 * Name the offset variable of every `plural` with an offset, and point its
 * `#` at that variable: in MF1, `#` shows the value minus the offset, while
 * `{arg}` keeps showing the raw value.
 */
function offsetVariables(ast: Tokens): Map<string, string> {
  const taken = argNames(ast);
  const names = new Map<string, string>();
  const nameFor = (arg: string) => {
    let name = names.get(arg);
    if (!name) {
      name = `${arg}_offset`;
      while (taken.has(name)) name += "_";
      taken.add(name);
      names.set(arg, name);
    }
    return name;
  };
  const walk = (tokens: Tokens, octothorpe: string | null) => {
    tokens.forEach((t, i) => {
      if (t.type === "octothorpe" && octothorpe) {
        tokens[i] = { type: "argument", arg: octothorpe, ctx: t.ctx };
      } else if (isSelect(t) && "cases" in t) {
        const inner =
          t.type === "plural" && t.pluralOffset
            ? nameFor(t.arg)
            : t.type === "select"
              ? octothorpe
              : null;
        for (const c of t.cases) walk(c.tokens, inner);
      }
    });
  };
  walk(ast, null);
  return names;
}

// ── canonicalization ──────────────────────────────────────────────────

function canonicalize(msg: Message, offsetNames: Map<string, string>): Message {
  for (const d of msg.declarations) d.value = canonicalExpression(d.value) as typeof d.value;
  if (msg.type === "message") {
    msg.pattern = canonicalPattern(msg.pattern);
    return msg;
  }
  for (const v of msg.variants) v.value = canonicalPattern(v.value);
  for (const d of [...msg.declarations]) {
    const offset = pluralOffset(d);
    const name = offsetNames.get(d.name);
    if (offset !== undefined && name) splitOffsetSelector(msg, d, offset, name);
  }
  return msg;
}

const canonicalPattern = (p: Pattern): Pattern =>
  p.map((el) =>
    typeof el !== "string" && el.type === "expression" ? canonicalExpression(el) : el,
  );

function canonicalExpression(expr: Expression): Expression {
  if (expr.function) expr.function = canonicalFunction(expr.function);
  return expr;
}

function canonicalFunction(fn: FunctionRef): FunctionRef {
  const opts = { ...fn.options };
  const lit = (name: string) => {
    const o = opts[name];
    return o?.type === "literal" ? o.value : undefined;
  };
  const has = (name: string) => name in opts;
  const rename = (name: string, drop: string[] = []): FunctionRef => {
    for (const d of drop) delete opts[d];
    const out: FunctionRef = { type: "function", name };
    if (Object.keys(opts).length) out.options = opts;
    return out;
  };
  switch (fn.name) {
    case "mf1:unit":
      if (lit("mf1:scale") === "100" && (!has("unit") || lit("unit") === "percent")) {
        return rename("percent", ["unit", "mf1:scale"]);
      }
      return has("unit") && !has("mf1:scale") ? rename("unit") : fn;
    case "mf1:currency":
      return has("currency") && !has("mf1:scale") ? rename("currency") : fn;
    case "mf1:date":
      return has("mf1:argStyle") ? fn : rename("date");
    case "mf1:time":
      if (has("mf1:argStyle")) return fn;
      if (opts.timeZoneName) {
        opts.timeZoneStyle = opts.timeZoneName;
        delete opts.timeZoneName;
      }
      return rename("time");
    default:
      return fn;
  }
}

/** The offset of an `.input {$x :mf1:plural offset=N}` cardinal declaration. */
function pluralOffset(d: Declaration): number | undefined {
  const fn = d.value.function;
  if (d.type !== "input" || fn?.name !== "mf1:plural") return undefined;
  const offset = fn.options?.offset;
  if (fn.options?.select || offset?.type !== "literal" || !/^\d+$/.test(offset.value))
    return undefined;
  return Number(offset.value);
}

/**
 * `.input {$c :mf1:plural offset=1} .match $c` becomes
 * `.input {$c :number} .local $c_offset = {$c :offset subtract=1} .match $c $c_offset`:
 * exact keys (`=0`) stay on `$c`, plural categories move to `$c_offset`.
 */
function splitOffsetSelector(
  msg: SelectMessage,
  d: Declaration,
  offset: number,
  name: string,
): void {
  const arg = { type: "variable", name: d.name } as const;
  d.value = { ...d.value, function: { type: "function", name: "number" } };
  msg.declarations.push({
    type: "local",
    name,
    value: {
      type: "expression",
      arg,
      function: {
        type: "function",
        name: "offset",
        options: { subtract: { type: "literal", value: String(offset) } },
      },
    },
  });
  const i = msg.selectors.findIndex((s) => s.name === d.name);
  if (i < 0) return;
  msg.selectors.splice(i + 1, 0, { type: "variable", name });
  for (const v of msg.variants) {
    const key = v.keys[i];
    if (!key) continue;
    const exact = key.type === "literal" && /^\d+$/.test(key.value);
    v.keys.splice(i, 1, exact ? key : { type: "*" }, exact ? { type: "*" } : key);
  }
}
