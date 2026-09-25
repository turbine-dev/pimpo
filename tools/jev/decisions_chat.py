#!/usr/bin/env python3
"""How in-app chat handles actions (send, delete, change). Writes decisions_chat.json."""
import json
from prioritize import PRODUCT, call

CTX = (" Today every request becomes an exploration: the agent reads for real, but every write (send a message, archive, delete, change a setting) is only"
       " simulated and recorded; the owner can then turn the exploration into a routine that performs the writes for real on a schedule, under the policy"
       " engine and approvals (irreversible actions always ask). Users coming from OpenClaw expect a chat that just does things now.")
QS = {"chat_actions": {"type": "choice", "instructions": "How should the new in-app chat handle writes the owner asks for in a message?" + CTX,
  "criteria": {
    "rehearse_confirm": "Rehearse as today, show the exact simulated actions under the answer, and a 'Confirm and do it' button that performs exactly those recorded actions once, each still checked by the policy engine and approvals",
    "live_with_approvals": "Run writes live during the chat, asking approval only for irreversible actions",
    "answers_only": "Chat only answers questions with reads; actions happen only by turning the exploration into a routine"}}}
r = call({"product": PRODUCT}, QS)
json.dump(r, open("decisions_chat.json", "w"), indent=1)
a = r["answers"]["chat_actions"]
print(a["choice"], sorted(((o, round(p, 2)) for o, p in a["probabilities"].items()), key=lambda x: -x[1]))
