#!/usr/bin/env python3
"""Score the OpenClaw features Zodim lacks, to order the next work. Writes decisions_gaps.json."""
import json
from concurrent.futures import ThreadPoolExecutor
from prioritize import PRODUCT, FEATURE_QS, call

NOW = ("Zodim today: compiled routines with replay tests, approvals with a mandatory floor, receipts in a hash chain, signed gallery of routines, "
       "memory with provenance, budget, family members, encrypted cloud backup, phone pairing with Tailscale built in, desktop/iOS/Android apps. "
       "The owner talks to it only on Telegram, WhatsApp or email; the LLM is only the Claude Code CLI; connectors are local MCP programs installed from a zip.")
GAPS = {
    "chat": "In-app chat sessions with history, suggested prompts and a button to turn a conversation into a routine, under the same approvals.",
    "models": "Choose among model providers (Anthropic API key, OpenAI, OpenRouter, local Ollama) besides the Claude Code CLI, per task.",
    "catalog": "A signed, audited catalog of connectors with search, categories and one-click install.",
    "mcp_url": "Add any MCP server from settings by command or remote URL, not only from a zip.",
    "web_search": "A web.search capability with a choice of provider (Brave, DuckDuckGo).",
    "run_history": "A global run history across routines and schedules shorter than an hour (every 15 or 30 minutes).",
    "memory_consolidation": "Nightly memory consolidation that merges duplicate facts, plus search by meaning, keeping unconfirmed facts out of decisions.",
    "voice_out": "Spoken replies (text to speech) and a microphone in the in-app chat.",
    "channels": "Two-way conversation on Slack and Discord, and Signal/iMessage/SMS channels.",
    "assistants": "Several named assistants, each with its own tools, permissions and channels.",
    "help": "A built-in 'what can Zodim do?' helper that answers questions about the app itself, per-type notification settings and a Labs area.",
}

def score(item):
    k, text = item
    return k, call({"product": PRODUCT, "today": NOW, "feature": text}, {q: FEATURE_QS[q] for q in ("user_value", "differentiation", "build_risk")})

with ThreadPoolExecutor(6) as ex:
    res = dict(ex.map(score, GAPS.items()))
json.dump(res, open("decisions_gaps.json", "w"), indent=1)
rows = []
for k, r in res.items():
    a = r["answers"]
    v, d, b = (a[q]["score"] for q in ("user_value", "differentiation", "build_risk"))
    rows.append((round(v * 2 + d - b, 2), k, round(v, 2), round(d, 2), round(b, 2)))
for row in sorted(rows, reverse=True):
    print(*row)
