#!/usr/bin/env python3
"""Ask Jev to score every candidate feature and UI concept for the new personal agent.

Usage: set -a && . ~/.jev-gateway/.env && set +a && python3 prioritize.py
Writes features.json and ui.json next to this file.
"""
import json
import os
import sys
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

API = "https://api.typesafe.ai/v1/systemone"
MODEL = "jev-latest"
HERE = Path(__file__).parent

PRODUCT = {
    "name": "Zodim (working name)",
    "what": "A free, open-source, local-first personal AI agent that competes with OpenClaw and Hermes Agent. "
    "Users talk to it on Telegram/WhatsApp and a web/desktop UI. Its core idea: the first time it does a task it uses an LLM "
    "(with approvals for irreversible actions); when that works, it compiles the task into a tested code routine with declared "
    "capabilities that runs on a schedule without an LLM, calling a small local judgment model only for judgment steps. "
    "It is secure by construction: rules live outside the model context, capability-scoped skills, approvals on the phone, "
    "credentials the agent never sees, receipts and undo for every action, hard spending caps. Single Go binary.",
    "target_user": "People who tried OpenClaw or Hermes and were burned: unreliable runs, silent failures, surprise token bills, "
    "an agent that deleted email after forgetting a rule, fear of malicious skills. Also small-business owners wanting a back office from the phone.",
    "competitors": "OpenClaw (390k stars, ~150 extensions, 60 model providers, insecure defaults, no spend cap, no audit of arguments) and "
    "Hermes Agent (persistent memory, self-written skills as prompt text, approvals that /yolo can bypass, no spend cap).",
    "team": "One founder who programs in Go and TypeScript, helped by coding agents.",
    "mvp_goal": "The morning brief (calendar + important emails + weather to Telegram) runs 30 days in a row as a compiled routine, "
    "with near-zero LLM cost, no silent failures, and irreversible actions always approved.",
}

FEATURES = {
    # Core innovation
    "explore_mode": "Exploration mode: the LLM plans and executes a new request with tool calls, asking approval for irreversible actions.",
    "routine_compiler": "Routine compiler: turns a successful exploration into a code routine (JS in QuickJS) plus tests built from recorded inputs/outputs plus a capability manifest.",
    "judgment_steps": "Judgment steps inside routines ('is this email important?') answered by a small local model with calibrated probability; below a threshold it asks the user.",
    "self_repair": "Self-repair: when a routine breaks (API changed, test fails, low confidence), it stops, alerts, re-explores and proposes a code diff for the user to approve.",
    "scheduler": "Scheduler for routines (cron-like, plus triggers like new email).",
    # Security
    "policy_engine": "Policy engine with rules outside the model context, written in plain language and compiled to verifiable rules checked before every action.",
    "capabilities": "Capability scoping: every routine, skill and conversation runs with only the capabilities it declared (e.g., read Gmail, send Telegram).",
    "phone_approvals": "Approvals on the phone (Telegram buttons) for irreversible actions, with a plain-language explanation of what will happen.",
    "anti_nag": "Anti-nag approvals: batching similar requests, learning safe patterns ('make this permanent?'), risk classifier so only risky actions ask.",
    "credential_broker": "Credential broker: the agent only sees handles; real secrets are injected into outgoing requests; per-capability domain allowlists.",
    "sandbox_default": "Sandbox on by default for shell commands and exploration code.",
    "spend_caps": "Hard spending caps per day and per routine, enforced before each LLM call; no silent fallback to other providers.",
    # Trust
    "audit_log": "Tamper-evident audit log with full arguments of every action, covering chats, routines and schedules.",
    "undo": "Undo by default: delete becomes trash, send becomes delayed send with a 10-minute undo, file snapshots.",
    "versioned_memory": "Memory as readable files versioned like git, with history and restore to any point.",
    "memory_provenance": "Memory provenance: every fact tagged with its source; low-trust content never becomes an instruction.",
    "health_watchdog": "Health watchdog: detects failed routines, undelivered messages, stuck tasks; always alerts on the phone.",
    "safe_updates": "Safe updates: snapshot before update and automatic rollback if the service is unhealthy.",
    # Adoption
    "single_binary": "Single Go binary install, no Node/Python/Docker required; runs on Mac, Raspberry Pi or a $4 VPS.",
    "migration": "One-command migration from OpenClaw/Hermes: skills, memory, channels and schedules, with a capability report per skill.",
    "guard_plugin": "Guard plugin for OpenClaw and Hermes users who have not migrated: same policy engine via their pre-tool hooks.",
    "subscription_brain": "Use an existing Claude or ChatGPT subscription (via Claude Code / Codex CLI) as the LLM brain instead of paying per token.",
    "any_provider": "Any LLM provider via API (Anthropic, OpenAI, OpenRouter, Ollama local).",
    "local_judgment_model": "Train and ship a small local judgment model (MLX on Apple Silicon) for triage/importance/risk decisions.",
    # Use cases
    "uc_morning_brief": "Use case: morning brief to Telegram (calendar, important emails, weather, tasks).",
    "uc_inbox_triage": "Use case: inbox triage (draft replies, archive, unsubscribe), always reversible.",
    "uc_small_business": "Use case: small-business back office from the phone (quote from a photo, PDF invoice, customer follow-ups).",
    "uc_skills_import": "Run imported SKILL.md skills from the OpenClaw/Hermes ecosystem with minimal capabilities.",
    # Channels
    "ch_telegram": "Telegram channel (chat + approvals).",
    "ch_whatsapp": "WhatsApp channel.",
    "ch_email": "Email as a channel (talk to the agent by email).",
    "ch_voice": "Voice notes in and out.",
    # Later
    "family_mode": "Family/small-team mode: several users, roles, per-person credentials and memory.",
    "threat_network": "Opt-in shared protection network: signatures of malicious skills and dangerous action patterns shared across users.",
    "mobile_app": "Native mobile app (iOS/Android).",
    # UI
    "ui_routines": "UI: routines view — each routine as a card with its schedule, last runs, cost, health, capabilities, and its code/tests/diff history.",
    "ui_receipts": "UI: receipts feed — a timeline of everything the agent did, human-readable, with arguments on demand and one-click undo.",
    "ui_approvals": "UI: approvals inbox on web/PWA mirroring the phone approvals, with context and 'always allow' learning.",
    "ui_rules": "UI: rules editor — write rules in plain language, see the compiled policy, test it against past actions ('this rule would have blocked 3 actions last week').",
    "ui_explore_live": "UI: live exploration view — watch the agent work step by step, pause, edit, approve, then 'turn this into a routine'.",
    "ui_cost": "UI: cost dashboard — spend per day/routine/model, budget bars, projected monthly bill.",
    "ui_memory": "UI: memory browser — readable memory files with sources, history, and restore.",
    "ui_onboarding": "UI: onboarding wizard in plain language ('what may the agent do without asking you?'), connecting accounts, first routine in 5 minutes.",
    "ui_pwa": "UI: installable PWA with push notifications as an alternative to Telegram.",
    "ui_desktop": "UI: native desktop app (Tauri) wrapping the web UI with tray icon and notifications.",
}

FEATURE_QS = {
    "user_value": {
        "type": "score",
        "instructions": "For the `product`'s `target_user`, how much real value does `feature` deliver in daily use?",
        "criteria": [
            "Little: nice to have, most users would not notice",
            "Some: useful occasionally",
            "High: solves a pain they feel often",
            "Critical: a main reason they would switch or stay",
        ],
    },
    "differentiation": {
        "type": "score",
        "instructions": "Compared with the `product`'s `competitors` (OpenClaw and Hermes Agent) as they are today, how differentiated is `feature`?",
        "criteria": [
            "Parity: both already have it",
            "Slightly better: a modest improvement on what they have",
            "Clearly better: they have a weak or off-by-default version",
            "Unique: neither has anything like it",
        ],
    },
    "build_risk": {
        "type": "score",
        "instructions": "For the `product`'s `team`, how hard and risky is building `feature` well?",
        "criteria": [
            "Low: standard engineering",
            "Moderate: some tricky parts with known solutions",
            "High: hard problems, many edge cases or fragile third-party APIs",
            "Very high: open research problem",
        ],
    },
    "mvp_need": {
        "type": "score",
        "instructions": "How necessary is `feature` to reach the `product`'s `mvp_goal`?",
        "criteria": [
            "Not needed: can wait until after the MVP",
            "Helpful: improves the MVP but the goal works without it",
            "Important: a minimal version should be in the MVP",
            "Essential: the MVP goal cannot be reached without it",
        ],
    },
}

UI_QS = {
    "primary_surface": {
        "type": "choice",
        "instructions": "Which should be the primary everyday interface for the `product`'s `target_user`?",
        "criteria": {
            "chat_only": "Chat only (Telegram/WhatsApp); no other UI",
            "chat_plus_web": "Chat for conversation and approvals, plus a local web UI (PWA) for routines, receipts, rules and cost",
            "desktop_app": "A native desktop app as the main surface, chat secondary",
            "mobile_app": "A native mobile app as the main surface",
        },
    },
    "hero_screen": {
        "type": "choice",
        "instructions": "Which single screen best communicates the `product`'s difference from OpenClaw and Hermes the first time someone opens it?",
        "criteria": {
            "routines": "Routines: cards of compiled routines with runs, cost, health and capabilities",
            "receipts": "Receipts: a timeline of everything the agent did, with undo",
            "explore_live": "Live exploration that ends with 'turn this into a routine'",
            "rules": "Rules in plain language with a test against past actions",
            "cost": "Cost dashboard with budget and projected bill",
        },
    },
    "wow_factor": {
        "type": "choice",
        "instructions": "Which moment would most make a burned OpenClaw/Hermes user say 'wow, this is different'?",
        "criteria": {
            "compile_moment": "Watching a task they just did become a readable, tested routine with a capability list, and seeing its cost drop to ~$0",
            "blocked_moment": "Seeing a dangerous action blocked and explained on their phone, with one-tap undo",
            "rule_test": "Writing a rule and instantly seeing which past actions it would have stopped",
            "migration_report": "Migrating from OpenClaw and seeing what each skill could have done versus what it can do now",
            "cost_projection": "Seeing the monthly bill projection fall after routines are compiled",
        },
    },
    "desktop_needed_mvp": {
        "type": "noul",
        "instructions": "Is a native desktop app (e.g., Tauri) necessary for the `product`'s `mvp_goal`, given a local web UI/PWA exists?",
    },
}


def call(state, questions):
    req = urllib.request.Request(
        API,
        data=json.dumps({"state": state, "model": MODEL, "questions": questions}).encode(),
        headers={"Authorization": f"Bearer {os.environ['TYPESAFE_API_KEY']}", "Content-Type": "application/json"},
    )
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=120) as r:
                return json.load(r)
        except (urllib.error.URLError, TimeoutError):
            if attempt == 3:
                raise
            time.sleep(2 ** attempt)


def main():
    if not os.environ.get("TYPESAFE_API_KEY"):
        sys.exit("set TYPESAFE_API_KEY")

    def score(item):
        key, text = item
        return key, call({"product": PRODUCT, "feature": text}, FEATURE_QS)

    with ThreadPoolExecutor(8) as ex:
        ui_future = ex.submit(call, {"product": PRODUCT, "ui_features": {k: v for k, v in FEATURES.items() if k.startswith("ui_")}}, UI_QS)
        results = dict(ex.map(score, FEATURES.items()))
        ui = ui_future.result()

    rows = []
    for key, r in results.items():
        a = r["answers"]
        rows.append({
            "feature": key,
            "value": round(a["user_value"]["score"], 2),
            "diff": round(a["differentiation"]["score"], 2),
            "risk": round(a["build_risk"]["score"], 2),
            "mvp": round(a["mvp_need"]["score"], 2),
        })
    # Priority favors value and differentiation, penalizes risk; kept simple so it can be re-weighted without re-asking Jev.
    for row in rows:
        row["priority"] = round(row["value"] * 0.45 + row["diff"] * 0.35 - row["risk"] * 0.2, 2)
    rows.sort(key=lambda r: -r["priority"])
    (HERE / "features.json").write_text(json.dumps({"rows": rows, "raw": results}, indent=1, ensure_ascii=False))
    (HERE / "ui.json").write_text(json.dumps(ui, indent=1, ensure_ascii=False))

    print(f"{'feature':22} {'value':>5} {'diff':>5} {'risk':>5} {'mvp':>5} {'prio':>5}")
    for r in rows:
        print(f"{r['feature']:22} {r['value']:5.2f} {r['diff']:5.2f} {r['risk']:5.2f} {r['mvp']:5.2f} {r['priority']:5.2f}")
    print()
    for k, a in ui["answers"].items():
        if a["type"] == "choice":
            probs = sorted(a["probabilities"].items(), key=lambda x: -x[1])
            print(f"{k:20} {a['choice']} (conf {a['confidence']:.2f}) " + ", ".join(f"{o}={p:.2f}" for o, p in probs[:3]))
        else:
            print(f"{k:20} P(yes)={a['noul']:.2f}")


if __name__ == "__main__":
    main()
