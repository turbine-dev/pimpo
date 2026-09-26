#!/usr/bin/env python3
"""How often each screen would be opened, to shape navigation. Writes decisions_layout.json."""
import json
from concurrent.futures import ThreadPoolExecutor
from prioritize import PRODUCT, call

USER = ("A person who set up Pimpo a few weeks ago and now uses it every day: routines run on their own, they chat with it, "
        "approve risky actions, and sometimes add a connection. Compare with OpenClaw's app, whose sidebar is: home composer, agents, "
        "dashboards, systems, automations, plugins, and the list of chat sessions, with an inbox and a user menu at the bottom.")
SCREENS = {
    "chat": "Chat with Pimpo (new request, follow-ups, confirm actions)",
    "chat_history": "The list of past chat sessions",
    "inbox": "Inbox: approvals waiting, failed routines, alerts",
    "routines": "Routines list: what runs on its own, next run, health",
    "routine_detail": "One routine's page: change schedule and parameters, runs, code",
    "run_history": "History of all routine runs",
    "gallery": "Gallery of ready-made routines to install",
    "connections": "Connections: channels, accounts, services, MCP catalog",
    "receipts": "Receipts: everything Pimpo did, with undo",
    "rules": "Rules written in plain language",
    "cost": "Cost dashboard",
    "memory": "Memory: what Pimpo knows about the owner",
    "people": "People in the house",
    "assistants": "Assistants (roles with limited tools)",
    "settings": "Settings",
    "help": "Help",
    "system": "System status: is everything running, CPU, memory, disk, queues, connectors",
    "today": "A home summary of today: what ran, what is next, what needs you",
}
Q = {"freq": {"type": "score", "instructions": "For `user`, how often would they open `screen`?",
      "criteria": ["Rarely: once a month or less, or only during setup", "Sometimes: a few times a month",
                   "Often: a few times a week", "Daily: every day or several times a day"]}}

def score(item):
    k, text = item
    return k, call({"product": PRODUCT, "user": USER, "screen": text}, Q)

with ThreadPoolExecutor(8) as ex:
    res = dict(ex.map(score, SCREENS.items()))
json.dump(res, open("decisions_layout.json", "w"), indent=1)
for k, r in sorted(res.items(), key=lambda kv: -kv[1]["answers"]["freq"]["score"]):
    print(f'{r["answers"]["freq"]["score"]:.2f} {k}')
