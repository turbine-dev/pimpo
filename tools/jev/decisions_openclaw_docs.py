#!/usr/bin/env python3
"""Order what to take from the OpenClaw docs survey (2026-09-27), items reframed for Pimpo. Writes decisions_openclaw_docs.json."""
import json
from concurrent.futures import ThreadPoolExecutor
from prioritize import PRODUCT, FEATURE_QS, call
NOW = ("Pimpo today: tasks become tested code routines (cron or watch triggers, state between runs, routines using routines, kept in a git repo) "
       "that run without an LLM; rules, approvals, receipts with undo; 7 chat channels with pairing; chat in the app keeps the conversation, "
       "but each Telegram/WhatsApp/Discord/Slack/Signal message starts a fresh task with no memory of the previous message; "
       "models: Claude Code (the owner's subscription) or API models the owner must type by exact name and price per million tokens "
       "(Anthropic, OpenAI, OpenRouter, Ollama only), hidden in one card in Settings, no detection of installed tools or local servers, "
       "no fallback when a model fails; a system panel shows each part's state; no auto-updater; a cat mascot. "
       "OpenClaw's model setup detects installed agents (Codex, Claude Code) and local servers (LM Studio, Ollama), lists ~30 providers with logos, "
       "tests a model with one click and shows categorized errors, and has fallback chains.")
ITEMS = {
    "models_redesign": "Model setup redesigned: detect Claude Code, Codex, a running Ollama or LM Studio and list their models; a provider catalog (Anthropic, OpenAI, Google Gemini, OpenRouter, DeepSeek, Groq, Mistral, xAI, LM Studio, any OpenAI-compatible URL) with known prices filled in and OpenRouter's live prices; 'test and use' in one click with latency and a plain-language error; shown as the first step of setup and in Connections.",
    "model_fallback": "Fallback models per job: when the chosen model fails (outage, rate limit, key refused, budget per model), the task retries on the next model in an ordered list, and the owner gets one notice when it falls back and one when it recovers.",
    "channel_conversation": "Conversation memory on chat channels: follow-up messages on Telegram, WhatsApp, Discord, Slack and Signal continue the same conversation (like the app's chat) instead of starting a fresh task, with a /new command to start over.",
    "codex_backend": "Use the owner's ChatGPT subscription through the Codex CLI as an alternative agent backend to Claude Code, with Pimpo's tools through MCP.",
    "typing_progress": "Typing indicator and a short progress note on chat channels while a task runs, so the owner knows it is working.",
    "doctor": "A 'check everything' doctor: one button that tests each channel, account, model and backup for real and explains how to fix what fails.",
    "inbound_webhook": "Inbound webhooks as a routine trigger: a secret URL per routine that other services (GitHub, Zapier, IFTTT, a form) can call to start it.",
    "usage_by_model": "Cost and usage by model and by job (explore, compile, judge, write), with provider quota windows where the API reports them.",
    "group_chats": "Group chats: the bot answers in a family or team group only when mentioned, with per-group limits.",
    "auto_update": "Auto-update with stable and beta channels and one-click rollback to the previous version.",
    "browser_tool": "A browser tool: the agent drives an isolated browser profile to read and act on websites.",
}
def score(item):
    k, text = item
    return k, call({"product": PRODUCT, "today": NOW, "feature": text}, {q: FEATURE_QS[q] for q in ("user_value", "differentiation", "build_risk")})
with ThreadPoolExecutor(8) as ex:
    res = dict(ex.map(score, ITEMS.items()))
json.dump(res, open("decisions_openclaw_docs.json", "w"), indent=1)
rows = []
for k, r in res.items():
    a = r["answers"]
    v, d, b = (a[q]["score"] for q in ("user_value", "differentiation", "build_risk"))
    rows.append((round(v * 2 + d - b, 2), k, round(v, 2), round(d, 2), round(b, 2)))
for row in sorted(rows, reverse=True):
    print(*row)
