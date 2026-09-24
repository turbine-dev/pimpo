#!/usr/bin/env python3
"""Settle the plan's pending decisions with Jev. Writes decisions.json."""
import json
from prioritize import PRODUCT, call

QS = {
    "name": {"type": "choice", "instructions": "Which name best fits the `product` for its `target_user`: memorable, easy to say in English and Portuguese, suggests reliability and control, and unlikely to collide with existing AI agent projects (GitHub search counts of repos named '<name> agent': vigia 8, routinely 0, tollgate 0, waypost 2, ledgerly 1, keel 18)?",
             "criteria": {"vigia": "Vigia (Portuguese for watchman)", "routinely": "Routinely (the agent that turns tasks into routines)",
                          "tollgate": "Tollgate (every action passes a checkpoint)", "waypost": "Waypost", "ledgerly": "Ledgerly (every action has a receipt)", "keel": "Keel (keeps things steady)"}},
    "judgment_backend": {"type": "choice", "instructions": "For judgment steps inside routines, which default backend fits the `product` best at launch, given it must be free and open source but needs calibrated probabilities?",
             "criteria": {"jev_optional_llm_default": "Cheap LLM by default (asked to return probabilities), Jev when the user has a key, local model added later",
                          "jev_default": "Jev by default", "local_default": "Local small model by default from day one"}},
    "gmail_access": {"type": "choice", "instructions": "How should the first version access Gmail for a single self-hosting user?",
             "criteria": {"imap_app_password": "IMAP with an app password", "oauth_own_client": "OAuth with the user's own Google Cloud client", "both_imap_first": "IMAP first, OAuth guided later"}},
    "llm_backend_dev": {"type": "choice", "instructions": "Which LLM backend should the first version implement first for exploration and compilation, for a solo founder who already pays for Claude Code?",
             "criteria": {"anthropic_api": "Anthropic Messages API with an API key", "claude_cli": "Claude Code headless (`claude -p`) using the user's subscription, API providers next", "openai_compatible": "Any OpenAI-compatible API first"}},
    "routine_language": {"type": "choice", "instructions": "Which language/runtime for compiled routines?", "criteria": {"js_goja": "JavaScript on goja (pure Go)", "starlark": "Starlark", "js_quickjs_wasm": "JavaScript on QuickJS in WebAssembly"}},
    "policy_language": {"type": "choice", "instructions": "Which language for compiled user rules?", "criteria": {"cel": "CEL (Common Expression Language)", "rego": "Rego (OPA)", "custom_yaml": "A small custom YAML rule format"}},
    "ui_stack": {"type": "choice", "instructions": "Which UI stack for an embedded local web UI where design quality matters a lot, built by one founder with coding agents?",
             "criteria": {"react_vite_tailwind": "React + Vite + TypeScript + Tailwind + Radix primitives + Motion", "svelte": "SvelteKit static", "htmx": "Server-rendered Go templates + htmx"}},
    "private_research": {"type": "noul", "instructions": "Should the `product` run private user interviews (no public announcement) during F1, even though public promotion is on hold?"},
}
r = call({"product": PRODUCT}, QS)
json.dump(r, open("decisions.json", "w"), indent=1)
for k, a in r["answers"].items():
    if a["type"] == "choice":
        print(k, a["choice"], sorted(((o, round(p, 2)) for o, p in a["probabilities"].items()), key=lambda x: -x[1])[:3])
    else:
        print(k, round(a["noul"], 2))
