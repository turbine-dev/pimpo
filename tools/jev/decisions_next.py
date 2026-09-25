#!/usr/bin/env python3
"""Order the next work after the OpenClaw/Hermes complaint survey, items already reframed critically. Writes decisions_next.json."""
import json
from concurrent.futures import ThreadPoolExecutor
from prioritize import PRODUCT, FEATURE_QS, call

NOW = ("Zodim today: tasks become tested code routines that run on a cron schedule without an LLM, asking a small judge for yes/no decisions; "
       "rules and approvals live outside the model; chat in the app rehearses changes before confirming; 7 chat channels; API or Claude Code models; "
       "no auto-updater (users install new versions by hand); daily snapshots exist. Survey of OpenClaw/Hermes users: they love proactivity "
       "(heartbeats, the agent starting conversations) and hate token bills, silent failures, updates that break things, bots that go silent.")
ITEMS = {
    "event_triggers": "Event-triggered routines: a routine declares a read capability to watch (e.g. new emails matching a query, new RSS items, a Home Assistant state) that Zodim polls without an LLM, and runs only for new items, so the agent 'starts a conversation' only when something matters.",
    "write_step": "A bounded write step inside routines: a cheap model writes a summary or draft reply for each item, capped by the budget, recorded in tests like judgments, never able to act by itself.",
    "prompt_cache": "Prompt caching on API models: tools and system prompt marked cacheable so repeated exploration turns cost a fraction.",
    "channel_alert": "Channel health alerts: when a chat channel (Telegram, WhatsApp, Discord, Slack, Signal) stops working for minutes, tell the owner on another channel and in the inbox.",
    "upgrade_snapshot": "On the first start of a new version, take a snapshot automatically and offer 'go back to before the update' in one click.",
    "remote_desktop": "Desktop app can connect to a Zodim running on another computer or a VPS instead of running its own, like the phone app.",
    "imessage": "iMessage channel on the Mac (needs Full Disk Access and AppleScript).",
}

def score(item):
    k, text = item
    return k, call({"product": PRODUCT, "today": NOW, "feature": text}, {q: FEATURE_QS[q] for q in ("user_value", "differentiation", "build_risk")})

with ThreadPoolExecutor(7) as ex:
    res = dict(ex.map(score, ITEMS.items()))
json.dump(res, open("decisions_next.json", "w"), indent=1)
rows = []
for k, r in res.items():
    a = r["answers"]
    v, d, b = (a[q]["score"] for q in ("user_value", "differentiation", "build_risk"))
    rows.append((round(v * 2 + d - b, 2), k, round(v, 2), round(d, 2), round(b, 2)))
for row in sorted(rows, reverse=True):
    print(*row)
