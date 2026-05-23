/**
 * Tiny ICU MessageFormat subset parser.
 *
 * Grammar handled (case-by-case):
 *
 *   message     ::= ( literal | placeholder )*
 *   placeholder ::= "{" name [ "," ftype [ "," arg ] ] "}"
 *   ftype       ::= "plural" | "select"
 *   arg         ::= plural-cases | select-cases
 *   plural-cases::= ( "=N" message | KEYWORD message )+
 *   KEYWORD     ::= "zero" | "one" | "two" | "few" | "many" | "other"
 *   select-cases::= ( IDENT message )+
 *
 * Apostrophes follow the ICU spec:
 *   - "''"   → literal apostrophe
 *   - "'{"   → starts a quoted run; ends at the next "'" or EOF
 *   - "'X'"  → literal X (X is "{" or "}" or "#")
 *
 * The "#" token inside a plural branch interpolates the current count
 * value verbatim (a deliberate ICU simplification — full ICU runs the
 * count through the locale's number formatter).
 *
 * Everything else (custom formatters, `number`/`date`/`time` argType,
 * RTL handling) is intentionally out of scope. See docs/design.md §4.
 */

export type Node = LiteralNode | VarNode | PluralNode | SelectNode | PoundNode;

export interface LiteralNode {
  type: "literal";
  value: string;
}

export interface VarNode {
  type: "var";
  name: string;
}

/** `{count, plural, one {…} other {…}}` */
export interface PluralNode {
  type: "plural";
  name: string;
  /** Exact-match cases, e.g. `=0`, `=1`. Higher priority than keyword cases. */
  exact: Record<number, Node[]>;
  /** Keyword cases keyed by Intl.LDMLPluralRule (`zero` / `one` / … / `other`). */
  cases: Record<string, Node[]>;
}

/** `{gender, select, female {…} other {…}}` */
export interface SelectNode {
  type: "select";
  name: string;
  cases: Record<string, Node[]>;
}

/** `#` inside a plural branch — interpolates the current count. */
export interface PoundNode {
  type: "pound";
}

export class ParseError extends Error {
  constructor(
    message: string,
    public readonly offset: number,
  ) {
    super(`${message} at offset ${offset}`);
    this.name = "ParseError";
  }
}

/**
 * Parse a message into an AST.
 *
 * `parse` runs once per message at registration time; the resulting AST
 * is reused across every render. `format` walks the AST against the
 * values map.
 */
export function parse(input: string): Node[] {
  const p = new Parser(input);
  const nodes = p.parseMessage(/*inPlural=*/ false, /*until=*/ "");
  if (!p.atEnd()) {
    throw new ParseError(`unexpected character '${p.peek()}'`, p.pos);
  }
  return nodes;
}

class Parser {
  pos = 0;
  constructor(public readonly src: string) {}

  atEnd(): boolean {
    return this.pos >= this.src.length;
  }

  peek(): string {
    return this.src[this.pos] ?? "";
  }

  advance(): string {
    const ch = this.src[this.pos] ?? "";
    this.pos += 1;
    return ch;
  }

  expect(ch: string): void {
    if (this.peek() !== ch) {
      throw new ParseError(`expected '${ch}' but got '${this.peek() || "EOF"}'`, this.pos);
    }
    this.pos += 1;
  }

  skipSpace(): void {
    while (!this.atEnd() && /\s/.test(this.peek())) this.pos += 1;
  }

  /**
   * Parse the contents of a message until a closing brace (when nested)
   * or EOF (top-level). `inPlural` enables `#` interpolation; `until`
   * is `"}"` for nested calls and `""` at the top level.
   */
  parseMessage(inPlural: boolean, until: string): Node[] {
    const out: Node[] = [];
    let buf = "";

    const flush = () => {
      if (buf) {
        out.push({ type: "literal", value: buf });
        buf = "";
      }
    };

    while (!this.atEnd()) {
      const ch = this.peek();
      if (until && ch === until) break;
      // Unmatched `}` at the top level is a syntax error per ICU.
      if (!until && ch === "}") {
        throw new ParseError("unmatched '}'", this.pos);
      }

      if (ch === "'") {
        // ICU apostrophe rules.
        this.pos += 1;
        if (this.peek() === "'") {
          // Literal apostrophe.
          buf += "'";
          this.pos += 1;
          continue;
        }
        // Quoted run begins. Ends at the next `'` or EOF.
        // Per ICU spec, `'X'` only quotes when X is a syntax character;
        // otherwise the leading `'` is itself literal. We follow the
        // simpler "any quoted run is literal" reading which matches
        // formatjs's MessageFormat compiler and is what callers expect
        // in practice.
        while (!this.atEnd() && this.peek() !== "'") {
          buf += this.advance();
        }
        if (this.peek() === "'") this.pos += 1;
        continue;
      }

      if (ch === "{") {
        flush();
        out.push(this.parsePlaceholder());
        continue;
      }

      if (ch === "#" && inPlural) {
        flush();
        out.push({ type: "pound" });
        this.pos += 1;
        continue;
      }

      buf += this.advance();
    }

    flush();
    return out;
  }

  parsePlaceholder(): Node {
    this.expect("{");
    this.skipSpace();
    const name = this.readIdent();
    this.skipSpace();

    // Bare variable: `{name}`.
    if (this.peek() === "}") {
      this.pos += 1;
      return { type: "var", name };
    }

    this.expect(",");
    this.skipSpace();
    const ftype = this.readIdent();
    this.skipSpace();
    this.expect(",");
    this.skipSpace();

    if (ftype === "plural") {
      const node = this.parsePluralCases(name);
      return node;
    }
    if (ftype === "select") {
      const node = this.parseSelectCases(name);
      return node;
    }
    throw new ParseError(`unsupported argType '${ftype}'`, this.pos);
  }

  parsePluralCases(name: string): PluralNode {
    const exact: PluralNode["exact"] = {};
    const cases: PluralNode["cases"] = {};

    while (!this.atEnd() && this.peek() !== "}") {
      this.skipSpace();
      let caseKey = this.readIdent();
      let isExact = false;
      let exactN = 0;

      if (caseKey === "" && this.peek() === "=") {
        // Exact match `=0`, `=1`, ...
        this.pos += 1;
        const digits = this.readDigits();
        if (digits === "") {
          throw new ParseError("expected digits after '='", this.pos);
        }
        isExact = true;
        exactN = parseInt(digits, 10);
      }

      if (caseKey === "" && !isExact) {
        throw new ParseError("expected plural case keyword or '='", this.pos);
      }

      this.skipSpace();
      this.expect("{");
      const branch = this.parseMessage(/*inPlural=*/ true, "}");
      this.expect("}");
      this.skipSpace();

      if (isExact) {
        exact[exactN] = branch;
      } else {
        cases[caseKey] = branch;
      }
    }
    this.expect("}");

    if (!cases.other) {
      throw new ParseError(`plural '${name}' missing 'other' case`, this.pos);
    }
    return { type: "plural", name, exact, cases };
  }

  parseSelectCases(name: string): SelectNode {
    const cases: SelectNode["cases"] = {};

    while (!this.atEnd() && this.peek() !== "}") {
      this.skipSpace();
      const caseKey = this.readIdent();
      if (caseKey === "") {
        throw new ParseError("expected select case identifier", this.pos);
      }
      this.skipSpace();
      this.expect("{");
      const branch = this.parseMessage(/*inPlural=*/ false, "}");
      this.expect("}");
      this.skipSpace();
      cases[caseKey] = branch;
    }
    this.expect("}");

    if (!cases.other) {
      throw new ParseError(`select '${name}' missing 'other' case`, this.pos);
    }
    return { type: "select", name, cases };
  }

  readIdent(): string {
    let s = "";
    while (!this.atEnd()) {
      const ch = this.peek();
      // ICU idents allow ASCII letters, digits, `_`, `-`, `.`.
      if (/[A-Za-z0-9_\-.]/.test(ch)) {
        s += this.advance();
      } else {
        break;
      }
    }
    return s;
  }

  readDigits(): string {
    let s = "";
    while (!this.atEnd() && /[0-9]/.test(this.peek())) {
      s += this.advance();
    }
    return s;
  }
}
