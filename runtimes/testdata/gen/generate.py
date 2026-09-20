#!/usr/bin/env python3
"""Generate the runtime conformance fixtures (runtimes/SPEC.md §7).

Artifacts are stored as their exact bytes (a string), because runtimes
verify SHA-256 over bytes, not over a re-serialization. Manifests are
signed with a fixed, TEST-ONLY Ed25519 key whose seed is below; it
signs nothing outside these fixtures.

Every artifact message is validated against the MF2 data-model schema
and every manifest/artifact against the contract schemas, so a fixture
can't drift from the contract.

    python3 runtimes/testdata/gen/generate.py        # rewrite fixtures
    python3 runtimes/testdata/gen/generate.py --check  # fail on drift (CI)
"""

import base64
import copy
import hashlib
import json
import sys
from pathlib import Path

import jsonschema
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

ROOT = Path(__file__).resolve().parents[1]
REPO = ROOT.parents[1]
SCHEMAS = ROOT / "schemas"
MF2_SCHEMA = REPO / "messageformat/testdata/unicode/data-model/message.schema.json"

TEST_KEY_SEED = bytes(range(32))  # TEST ONLY — never used outside fixtures
TEST_KEY_ID = "k_test"
TEST_KEY = Ed25519PrivateKey.from_private_bytes(TEST_KEY_SEED)
TEST_PUBLIC_KEY = base64.urlsafe_b64encode(TEST_KEY.public_key().public_bytes_raw()).rstrip(b"=").decode()
OTHER_KEY = Ed25519PrivateKey.from_private_bytes(bytes(range(32, 64)))


# ── MF2 data-model helpers ─────────────────────────────────────────────

def text(s):
    return {"type": "message", "declarations": [], "pattern": [s]}


def var(name, fn=None, **options):
    expr = {"type": "expression", "arg": {"type": "variable", "name": name}}
    if fn:
        expr["function"] = {"type": "function", "name": fn}
        if options:
            expr["function"]["options"] = {k: {"type": "literal", "value": str(v)} for k, v in options.items()}
    return expr


def pattern(*parts):
    return {"type": "message", "declarations": [], "pattern": list(parts)}


def plural(name, one, other):
    """`.input {$name :number} .match $name  one {{…}}  * {{…}}`"""
    return {
        "type": "select",
        "declarations": [{"type": "input", "name": name, "value": var(name, "number")}],
        "selectors": [{"type": "variable", "name": name}],
        "variants": [
            {"keys": [{"type": "literal", "value": "one"}], "value": one},
            {"keys": [{"type": "*"}], "value": other},
        ],
    }


# ── Serialization, hashing, signing ─────────────────────────────────────

def artifact_bytes(locale, messages, namespace="default"):
    doc = {"schema": "glossa.artifact/v1", "locale": locale, "namespace": namespace, "messages": messages}
    return json.dumps(doc, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def sha256(s):
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def jcs(value):
    """RFC 8785 canonicalization for the value space used here (no floats)."""
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def sign(manifest, key=TEST_KEY, key_id=TEST_KEY_ID):
    unsigned = {k: v for k, v in manifest.items() if k != "signatures"}
    sig = key.sign(jcs(unsigned).encode("utf-8"))
    signed = copy.deepcopy(unsigned)
    signed["signatures"] = [{"keyId": key_id, "alg": "Ed25519",
                             "sig": base64.urlsafe_b64encode(sig).rstrip(b"=").decode()}]
    return signed


def build_release(release_id, version, source, locales, fallback, catalogs):
    """catalogs: {locale: {message_id: message}} → (manifest, {sha: bytes})."""
    artifacts, blobs = {}, {}
    for loc, messages in catalogs.items():
        body = artifact_bytes(loc, messages)
        digest = sha256(body)
        blobs[digest] = body
        artifacts[loc] = {"default": {"sha256": digest, "size": len(body.encode("utf-8"))}}
    manifest = {
        "schema": "glossa.manifest/v1",
        "project": "prj_fixture",
        "environment": "production",
        "release": {"id": release_id, "version": version, "createdAt": f"2026-09-{version:02d}T08:00:00Z"},
        "sourceLocale": source,
        "locales": [{"code": c, "direction": d} for c, d in locales],
        "fallback": fallback,
        "artifacts": artifacts,
    }
    return manifest, blobs


# ── Scenarios (resolution) ──────────────────────────────────────────────

BASE_LOCALES = [("de", "ltr"), ("de-AT", "ltr"), ("en", "ltr"), ("zh-Hant", "ltr"), ("ar", "rtl"), ("fr-CA", "ltr"), ("fr", "ltr")]
BASE_CATALOGS = {
    "de": {
        "cart.checkout": text("Zur Kasse"),
        "cart.items": plural("n", [var("n"), " Artikel"], [var("n"), " Artikel"]),
        "cart.total": pattern(var("amount", "number"), " Artikel"),
        "greeting": pattern("Hallo ", var("name"), "!"),
        "only.de": text("Nur auf Deutsch"),
    },
    "de-AT": {"cart.checkout": text("Zur Kassa")},
    "en": {
        "cart.checkout": text("Checkout"),
        "cart.items": plural("n", ["one item"], [var("n"), " items"]),
        "greeting": pattern("Hello ", var("name"), "!"),
        "only.en": text("English only"),
    },
    "zh-Hant": {"cart.checkout": text("結帳")},
    "ar": {"greeting": pattern("مرحبا ", var("name"), "!")},
    "fr": {"cart.checkout": text("Paiement")},
    "fr-CA": {},
}


def resolution_scenarios():
    manifest, blobs = build_release(
        "rel_base", 1, "de", BASE_LOCALES,
        {"de-AT": ["de"], "*": ["en"]},
        BASE_CATALOGS,
    )

    def case(requested, id, exp, locale, chain, resolved, values=None, default=None, **extra):
        c = {"requested": requested, "id": id, "values": values or {}, "bidiIsolation": "none",
             "exp": exp, "expLocale": locale, "expChain": chain, "expResolvedFrom": resolved}
        if default is not None:
            c["default"] = default
        c.update(extra)
        return c

    yield "negotiation", {
        "description": "RFC 4647 lookup: exact, truncation, canonicalization, no match",
        "manifest": manifest, "artifacts": blobs,
        "cases": [
            case(["en"], "cart.checkout", "Checkout", "en", ["en", "de"], "en"),
            case(["en-GB"], "cart.checkout", "Checkout", "en", ["en", "de"], "en"),
            case(["zh-Hant-TW"], "cart.checkout", "結帳", "zh-Hant", ["zh-Hant", "en", "de"], "zh-Hant"),
            case(["EN_us"], "cart.checkout", "Checkout", "en", ["en", "de"], "en"),
            case(["ja", "fr-CA"], "cart.checkout", "Paiement", "fr-CA", ["fr-CA", "fr", "en", "de"], "fr"),
            case(["ja"], "cart.checkout", "Zur Kasse", "de", ["de", "en"], "de"),
            case([], "cart.checkout", "Zur Kasse", "de", ["de", "en"], "de"),
        ],
    }

    yield "fallback-graph", {
        "description": "Explicit edges, the '*' chain, truncation, and the source locale",
        "manifest": manifest, "artifacts": blobs,
        "cases": [
            case(["de-AT"], "cart.checkout", "Zur Kassa", "de-AT", ["de-AT", "de", "en"], "de-AT"),
            case(["de-AT"], "only.de", "Nur auf Deutsch", "de-AT", ["de-AT", "de", "en"], "de"),
            case(["de-AT"], "only.en", "English only", "de-AT", ["de-AT", "de", "en"], "en"),
            case(["fr-CA"], "only.de", "Nur auf Deutsch", "fr-CA", ["fr-CA", "fr", "en", "de"], "de"),
            case(["ar"], "greeting", "مرحبا Lina!", "ar", ["ar", "en", "de"], "ar", values={"name": "Lina"}, expDirection="rtl"),
        ],
    }

    yield "formatting-locale", {
        "description": "A message found in a fallback locale is formatted with that locale's rules",
        "manifest": manifest, "artifacts": blobs,
        "cases": [
            case(["en"], "cart.items", "one item", "en", ["en", "de"], "en", values={"n": 1}),
            case(["en"], "cart.items", "3 items", "en", ["en", "de"], "en", values={"n": 3}),
            case(["en"], "cart.total", "1.234,5 Artikel", "en", ["en", "de"], "de", values={"amount": 1234.5}),
        ],
    }

    yield "missing-message", {
        "description": "Nothing in the chain: the inline default, else the message ID; never empty",
        "manifest": manifest, "artifacts": blobs,
        "cases": [
            case(["en"], "does.not.exist", "Fallback text", "en", ["en", "de"], None, default="Fallback text"),
            case(["en"], "does.not.exist", "does.not.exist", "en", ["en", "de"], None),
        ],
    }

    cyclic, cyclic_blobs = build_release(
        "rel_cycle", 1, "en", [("en", "ltr"), ("pt-BR", "ltr"), ("pt-PT", "ltr")],
        {"pt-BR": ["pt-PT"], "pt-PT": ["pt-BR"]},
        {"en": {"hello": text("Hello")}, "pt-BR": {}, "pt-PT": {"hello": text("Olá")}},
    )
    yield "fallback-cycle", {
        "description": "Cycles in the fallback graph stop at the first repeat",
        "manifest": cyclic, "artifacts": cyclic_blobs,
        "cases": [
            case(["pt-BR"], "hello", "Olá", "pt-BR", ["pt-BR", "pt-PT", "en"], "pt-PT"),
        ],
    }


# ── Loading sequences ───────────────────────────────────────────────────

def ok(manifest, etag):
    return {"status": 200, "etag": etag, "body": manifest}


def loading_sequences():
    r1, b1 = build_release("rel_1", 1, "en", [("en", "ltr"), ("de", "ltr")], {},
                           {"en": {"hello": text("Hello v1")}, "de": {"hello": text("Hallo v1")}})
    r2, b2 = build_release("rel_2", 2, "en", [("en", "ltr"), ("de", "ltr")], {},
                           {"en": {"hello": text("Hello v2")}, "de": {"hello": text("Hallo v2")}})
    s1, s2 = sign(r1), sign(r2)
    blobs = {**b1, **b2}

    corrupt = dict(b2)
    for digest in list(corrupt):
        corrupt[digest] = corrupt[digest].replace("v2", "vX")  # bytes no longer match the hash

    unsigned_r2 = {k: v for k, v in s2.items() if k != "signatures"}
    wrong_key_r2 = sign(r2, key=OTHER_KEY, key_id=TEST_KEY_ID)
    future = copy.deepcopy(s2)
    future["schema"] = "glossa.manifest/v2"

    def step(desc, manifest, artifacts, active, source, errors=(), read="hello", locale="en", exp=None):
        return {"description": desc, "edge": {"manifest": manifest, "artifacts": artifacts},
                "read": {"id": read, "requested": [locale]},
                "expActiveRelease": active, "expSource": source, "expErrors": list(errors),
                "exp": exp}

    down = {"status": 503}
    yield "last-good", {
        "description": "Network loss falls back to the persisted last-good release",
        "publicKeys": [{"keyId": TEST_KEY_ID, "key": TEST_PUBLIC_KEY}],
        "steps": [
            step("cold start, edge up", ok(s1, '"m1"'), blobs, "rel_1", "network", exp="Hello v1"),
            step("revalidate, not modified", {"status": 304}, blobs, "rel_1", "memory", exp="Hello v1"),
            step("process restart, edge down", down, {}, "rel_1", "persisted", ["network"], exp="Hello v1", ),
            step("edge back with a new release", ok(s2, '"m2"'), blobs, "rel_2", "network", exp="Hello v2"),
        ],
        "restartBefore": [2],
    }
    yield "atomic-activation", {
        "description": "A release whose artifact fails integrity never becomes active",
        "publicKeys": [{"keyId": TEST_KEY_ID, "key": TEST_PUBLIC_KEY}],
        "steps": [
            step("release 1", ok(s1, '"m1"'), blobs, "rel_1", "network", exp="Hello v1"),
            step("release 2 with corrupted artifact bytes", ok(s2, '"m2"'), {**b1, **corrupt}, "rel_1", "memory",
                 ["integrity"], exp="Hello v1"),
            step("release 2 served correctly", ok(s2, '"m2b"'), blobs, "rel_2", "network", exp="Hello v2"),
        ],
    }
    yield "signatures", {
        "description": "With public keys configured, unsigned or wrongly signed manifests are rejected",
        "publicKeys": [{"keyId": TEST_KEY_ID, "key": TEST_PUBLIC_KEY}],
        "steps": [
            step("release 1 signed", ok(s1, '"m1"'), blobs, "rel_1", "network", exp="Hello v1"),
            step("release 2 unsigned", ok(unsigned_r2, '"m2"'), blobs, "rel_1", "memory", ["signature"], exp="Hello v1"),
            step("release 2 signed by an unknown key", ok(wrong_key_r2, '"m3"'), blobs, "rel_1", "memory", ["signature"],
                 exp="Hello v1"),
        ],
    }
    yield "schema-version", {
        "description": "A manifest with an unknown major schema version is rejected; the last good release stays",
        "publicKeys": [{"keyId": TEST_KEY_ID, "key": TEST_PUBLIC_KEY}],
        "steps": [
            step("release 1", ok(s1, '"m1"'), blobs, "rel_1", "network", exp="Hello v1"),
            step("v2 manifest", ok(future, '"m2"'), blobs, "rel_1", "memory", ["schema"], exp="Hello v1"),
        ],
    }
    yield "cold-offline", {
        "description": "No network and nothing persisted: inline default, never empty",
        "publicKeys": [],
        "steps": [
            step("edge down on first ever start", down, {}, None, "inline", ["network"], exp="hello"),
        ],
    }


# ── Edge: delivery-key scopes (SPEC §2) ─────────────────────────────────

EDGE_PROJECT = "0192f5a0-7a4e-7cc3-9d1e-3a4b5c6d7e8f"
DEFAULT_ENVIRONMENTS = ["development", "preview", "staging", "production"]


def delivery_key(label):
    """A fixed, well-formed publishable key: glossa_pk_ + 32 base64url characters."""
    raw = hashlib.sha256(f"fixture key {label}".encode("utf-8")).digest()[:24]
    return "glossa_pk_" + base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def key_index(key_id, environments=None, branches=None):
    """A key index object; environments=None writes one from before scopes."""
    idx = {"schema": "glossa.delivery-key/v1", "project": EDGE_PROJECT, "key_id": key_id}
    if environments is not None:
        idx["environments"] = sorted(environments)
        idx["branches"] = bool(branches)
    return idx


def edge_fixtures():
    keys = {
        "production": key_index("k_prod", ["production"]),
        "staging-and-production": key_index("k_stage", ["staging", "production"]),
        "preview": key_index("k_preview", ["preview"], branches=True),
        "branches-only": key_index("k_branches", [], branches=True),
        "legacy": key_index("k_legacy"),
        "revoked": None,
    }
    environments = DEFAULT_ENVIRONMENTS + ["qa", "pr-42", "br-0a1b2c3d"]

    def case(key, environment, status):
        return {"key": key, "environment": environment, "expStatus": status}

    cases = []
    for label, idx in keys.items():
        for env in environments:
            if idx is None:
                allowed = False
            else:
                allowlist = idx.get("environments", DEFAULT_ENVIRONMENTS)
                branch = env.startswith(("pr-", "br-"))
                allowed = env in allowlist or (idx.get("branches", False) and branch)
            cases.append(case(label, env, 200 if allowed else 404))
        cases.append({"key": label, "artifact": True, "expStatus": 404 if idx is None else 200})
    yield "key-scopes", {
        "description": "A key reads the environments on its allowlist and, with branches, every branch environment "
                       "(pr-<n>, br-<8 hex>); anything else answers 404 exactly like an unknown key. An index object "
                       "without environments predates scopes and reads as the four default environments. Artifacts "
                       "aren't scoped by environment.",
        "project": EDGE_PROJECT,
        "keys": {label: {"key": delivery_key(label), "index": idx} for label, idx in keys.items()},
        "environments": environments,
        "cases": cases,
    }


# ── Safe markup (formatted parts → HTML) ────────────────────────────────

# Inline, attribute-free phrasing elements a translation may produce. The
# single source of truth: @glossa/elements' SAFE_TAGS and the Go runtime's
# safeTags are tested against this list.
SAFE_TAGS = ("b strong i em u s small mark sub sup code kbd samp var abbr cite dfn q "
             "del ins bdi span br wbr").split()
VOID_TAGS = ["br", "wbr"]


def t(value):
    return {"type": "text", "value": value}


def mk(kind, name, **options):
    p = {"type": "markup", "kind": kind, "name": name}
    if options:
        p["options"] = options
    return p


def markup_fixture():
    open_, close, alone = (lambda n, **o: mk("open", n, **o)), (lambda n: mk("close", n)), (lambda n: mk("standalone", n))

    def case(description, parts, html):
        return {"description": description, "parts": parts, "html": html}

    return {
        "$comment": "Formatted parts -> HTML, the rules of @glossa/elements/parts and the Go runtime's HTML/th. "
                    "`safeTags`: the only markup names that become elements; `voidTags`: the safe tags rendered "
                    "without children or a closing tag. Markup options are always dropped. Text is escaped "
                    "(& < >). Unsafe markup keeps just its content. Unclosed markup closes at the end; a close "
                    "closes everything opened after its matching open; a close without an open is ignored. "
                    "Non-markup parts render as their text: `value`, the joined `parts` values of numbers and "
                    "dates, or `{source}` for a fallback.",
        "safeTags": SAFE_TAGS,
        "voidTags": VOID_TAGS,
        "cases": [
            case("text, isolation, values and fallbacks join into one text node",
                 [t("Hallo "), {"type": "bidiIsolation", "value": "⁨"}, {"type": "string", "value": "Lina"},
                  {"type": "bidiIsolation", "value": "⁩"}, t(", du hast "),
                  {"type": "number", "parts": [{"type": "integer", "value": "1"}, {"type": "group", "value": "."},
                                               {"type": "integer", "value": "000"}]},
                  t(" Punkte "), {"type": "fallback", "source": "$missing"}],
                 "Hallo ⁨Lina⁩, du hast 1.000 Punkte {$missing}"),
            case("safe markup becomes nested elements",
                 [t("Tippe "), open_("b"), t("hier "), open_("em"), t("jetzt"), close("em"), close("b"), t(".")],
                 "Tippe <b>hier <em>jetzt</em></b>."),
            case("standalone void markup", [t("a"), alone("br"), t("b"), alone("wbr")], "a<br>b<wbr>"),
            case("an opened void tag renders once, without children", [open_("br"), t("x"), close("br")], "<br>x"),
            case("standalone non-void markup renders nothing", [t("a"), alone("b"), t("c")], "ac"),
            case("unsafe markup keeps only its content",
                 [t("Klick "), open_("link"), t("hier"), close("link"), open_("script"), t("x"), close("script"),
                  alone("img")],
                 "Klick hierx"),
            case("a link from a translation stays text (en link markup)",
                 [t("By continuing you accept the "), open_("link", href="/terms"), t("terms of service"),
                  close("link"), t(".")],
                 "By continuing you accept the terms of service."),
            case("options are dropped, so a translation can't add attributes",
                 [open_("b", onclick="alert(1)", **{"class": "x"}), t("x"), close("b"),
                  open_("span", style="color:red", title="t"), t("y"), close("span")],
                 "<b>x</b><span>y</span>"),
            case("safe markup inside unsafe markup", [open_("a", href="javascript:alert(1)"), open_("b"), t("x"),
                                                      close("b"), close("a")], "<b>x</b>"),
            case("unclosed markup closes at the end", [open_("b"), t("x"), open_("i"), t("y")], "<b>x<i>y</i></b>"),
            case("a close without an open is ignored", [t("x"), close("b"), t("y")], "xy"),
            case("a close closes everything opened after its open",
                 [open_("b"), t("1"), open_("i"), t("2"), close("b"), t("3"), close("i")],
                 "<b>1<i>2</i></b>3"),
            case("a close for unsafe markup ends the safe markup inside it",
                 [open_("link"), t("1"), open_("b"), t("2"), close("link"), t("3")],
                 "1<b>2</b>3"),
            case("text is escaped, quotes are kept",
                 [t("<img src=x onerror=alert(1)> & "), open_("strong"), t('"fett" \'x\''), close("strong"),
                  alone("br")],
                 "&lt;img src=x onerror=alert(1)&gt; &amp; <strong>\"fett\" 'x'</strong><br>"),
            case("markup names are case-sensitive", [open_("B"), t("x"), close("B")], "x"),
            case("empty safe elements are kept", [open_("b"), close("b"), t("x")], "<b></b>x"),
        ],
    }


# ── Validation and output ───────────────────────────────────────────────

def validators():
    load = lambda p: json.loads(p.read_text())
    mf2 = jsonschema.Draft7Validator(load(MF2_SCHEMA))
    manifest = jsonschema.Draft202012Validator(load(SCHEMAS / "manifest.schema.json"))
    artifact = jsonschema.Draft202012Validator(load(SCHEMAS / "artifact.schema.json"))
    return mf2, manifest, artifact


def delivery_key_validator():
    return jsonschema.Draft202012Validator(json.loads((SCHEMAS / "delivery-key.schema.json").read_text()))


def validate_release(manifest, blobs, v):
    mf2, man, art = v
    if manifest.get("schema") == "glossa.manifest/v1":
        man.validate(manifest)
    for body in blobs.values():
        doc = json.loads(body)
        art.validate(doc)
        for message in doc["messages"].values():
            mf2.validate(message)


def render(obj):
    return json.dumps(obj, ensure_ascii=False, indent=2) + "\n"


def main():
    check = "--check" in sys.argv
    v = validators()
    outputs = {}
    for name, sc in resolution_scenarios():
        validate_release(sc["manifest"], sc["artifacts"], v)
        outputs[ROOT / "scenarios" / f"{name}.json"] = render(sc)
    for name, seq in loading_sequences():
        for st in seq["steps"]:
            m = st["edge"]["manifest"]
            if m.get("status") == 200:
                validate_release(m["body"], {}, v)
        outputs[ROOT / "loading" / f"{name}.json"] = render(seq)
    outputs[ROOT / "markup.json"] = render(markup_fixture())
    key_schema = delivery_key_validator()
    for name, fx in edge_fixtures():
        for k in fx["keys"].values():
            if k["index"] is not None:
                key_schema.validate(k["index"])
        outputs[ROOT / "edge" / f"{name}.json"] = render(fx)
    drift = [p for p, content in outputs.items() if not p.exists() or p.read_text() != content]
    if check:
        if drift:
            print("runtime fixtures are stale:", *[str(p.relative_to(REPO)) for p in drift], sep="\n  ")
            sys.exit(1)
        print(f"{len(outputs)} runtime fixtures up to date")
        return
    for p, content in outputs.items():
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content)
    print(f"wrote {len(outputs)} fixtures ({len(drift)} changed)")


if __name__ == "__main__":
    main()
