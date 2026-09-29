#!/usr/bin/env python3
"""Order the roadmap to make Pimpo a serious competitor to OpenClaw and Hermes (2026-09-29). Writes decisions_roadmap.json.
Usage: set -a && . ~/.jev-gateway/.env && set +a && python3 decisions_roadmap.py
"""
import json
from concurrent.futures import ThreadPoolExecutor
from prioritize import PRODUCT, FEATURE_QS, call

NOW = ("Pimpo today (all built and tested, but never released): tasks explored once by an LLM become tested JavaScript routines that run "
       "without an LLM on schedules, watches, webhooks and answers; rules, approvals, undo and a tamper-evident log outside the model; "
       "Claude Code, Codex and opencode subscriptions plus 12 API providers and Ollama, with automatic model and effort choice; "
       "connectors from JSON files, OpenAPI descriptions and any MCP server without recompiling; 7 chat channels (WhatsApp only through the "
       "official business API); local and cloud voices; phone app only for approvals and receipts; desktop for macOS, Windows, Linux built in CI "
       "but unsigned and without auto-update; no Docker image; gallery of 11 routines, its repo not yet published; the agent can read web pages "
       "but cannot drive a browser, run shell commands or code outside routines, or run several agents in parallel. First setup needs Claude Code. "
       "OpenClaw has ~390k stars, thousands of community skills (SKILL.md, ClawHub), browser control, personal WhatsApp and iMessage, phone nodes "
       "(camera, location), voice wake; Hermes has terminal backends (local, Docker, SSH, cloud), subagents, self-written skills and a user model.")
QS = dict({q: FEATURE_QS[q] for q in ("user_value", "differentiation", "build_risk")})
QS["adoption_block"] = {
    "type": "score",
    "instructions": "Without `feature`, how much does it stop people from trying or keeping Pimpo at all, given `today`?",
    "criteria": [
        "Not at all: people adopt Pimpo fine without it",
        "A little: some people miss it after a while",
        "Much: many people give up or never start without it",
        "Blocking: almost nobody can start or keep using Pimpo without it",
    ],
}
QS["safety_risk"] = {
    "type": "score",
    "instructions": "How much does building `feature` put Pimpo's safety promises at risk (actions without approval, leaked secrets, instructions smuggled from content, other people's data), even if built carefully?",
    "criteria": [
        "None: it does not touch what the agent can do",
        "Low: contained by the existing capabilities, rules and approvals",
        "Medium: new powers that need a new containment design",
        "High: broad powers (arbitrary code, logged-in sessions, unofficial APIs) that are hard to contain",
    ],
}
ITEMS = {
    "release": "Publish the first release (v0.6.0) with binaries, desktop installers and checksums.",
    "updater": "Auto-update in the desktop app (Tauri updater with signed update manifests), with a snapshot before each update and one-click rollback.",
    "signing": "Official code signing: Apple Developer ID with notarization for macOS, and Authenticode signing for Windows installers.",
    "docker": "An official Docker image and a guided install on a VPS or home server, with remote access from the desktop and phone apps.",
    "ecosystem_repos": "Publish the gallery and protection-list repositories and grow the gallery from 11 to about 50 useful, signed, audited routines.",
    "discovery": "A website with a demo video, plus Homebrew, winget and Flathub packages.",
    "browser": "An agent-driven browser: open sites, click, fill forms, use the owner's logged-in sessions, with every action going through capabilities, rules and approvals, and browser steps compilable into routines.",
    "sandbox": "Shell and code execution in an isolated sandbox (container or VM, no secrets, network allowlist), locally or on a remote host, with approval for anything that leaves the sandbox.",
    "subagents": "Parallel agents and long tasks: split a large job across several agents that run for hours in the background, with progress, budget and a final report.",
    "skills": "Install and use SKILL.md skills from OpenClaw, Hermes and agentskills.io directly, reviewed and scoped to declared capabilities, and compile them into routines when possible.",
    "phone_node": "The phone as part of the agent: camera, location, phone notifications and shortcuts available as capabilities.",
    "channels": "More channels: iMessage on the Mac and personal WhatsApp (unofficial protocol).",
    "voice_mode": "Always-on voice: wake word and continuous spoken conversation on desktop and phone.",
    "proactive": "Proactivity: a loop where the agent on its own looks at what changed (email, calendar, feeds) and suggests what to do or which routine to create.",
    "user_model": "Learning the user: a model of the owner's preferences that improves from corrections and choices over time and shapes answers and routines.",
    "easy_setup": "First setup in minutes with only an API key or a local Ollama, without Claude Code.",
    "real_validation": "Validate with real accounts for several weeks (email, calendar, sheets, channels) and fix what breaks.",
    "bugfixes": "Fix the bugs found while documenting: repair never gets the failure message, webhook-only routines log false schedule failures, answers run paused routines, wrong settings messages, the judge ignoring the Ollama address.",
}

def score(item):
    k, text = item
    return k, call({"product": PRODUCT, "today": NOW, "feature": text}, QS)

if __name__ == "__main__":
    with ThreadPoolExecutor(6) as ex:
        res = dict(ex.map(score, ITEMS.items()))
    json.dump(res, open("decisions_roadmap.json", "w"), indent=1)
    rows = []
    for k, r in res.items():
        a = r["answers"]
        v, d, b, ad, s = (a[q]["score"] for q in ("user_value", "differentiation", "build_risk", "adoption_block", "safety_risk"))
        rows.append((round(2 * v + d + 2 * ad - b - s, 2), k, round(v, 2), round(d, 2), round(ad, 2), round(b, 2), round(s, 2)))
    print("total key value diff adoption build safety")
    for row in sorted(rows, reverse=True):
        print(*row)
