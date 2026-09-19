// Package intelligence is Glossa's Intelligence bounded context (RFC 0003
// §3–§4): AI translation that amplifies accumulated knowledge instead of
// replacing it (intent §54).
//
// This wave is the core library only — no HTTP, tables, jobs or outbox
// wiring:
//
//   - domain: the model-agnostic provider port and its errors, routing
//     policy, budget port, pricing, the Knowledge ports the agent's tools
//     consume, the Suggestion with its provenance, and the confidence
//     score with its explanation and review routing. Pure.
//   - app: the translation agent (agent-go state machine driving an
//     axi-go toolset), the provider router with fallback and budget
//     guard, validation, and the Translator entry point.
//   - adapters: Anthropic (official SDK), OpenAI-compatible and Gemini
//     providers, the fortify resilience wrapper, the cassette provider
//     that replays recorded responses, and in-memory Knowledge and
//     budget fakes.
//   - prompts: versioned prompt templates, embedded.
//   - evals: golden sets per locale pair and the eval runner.
package intelligence
