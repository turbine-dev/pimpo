#!/usr/bin/env python3
"""How the phone app reaches a Vigia at home without the owner installing anything. Writes decisions_remote.json."""
import json
from prioritize import PRODUCT, call

CTX = " Vigia is a free, open-source Go binary on the owner's computer, listening on localhost; non-technical owners; the phone companion needs a stable HTTPS link with a revocable per-device token; the project has no servers and wants none it must run or pay for; the link must survive restarts."
QS = {"remote": {"type": "choice", "instructions": "Which way should Vigia give each owner a personal link the phone app opens, with nothing to install?" + CTX,
  "criteria": {
    "tsnet_funnel": "Embed Tailscale in the binary (tsnet) and publish with Funnel: a stable https://vigia-<name>.<tailnet>.ts.net link after a one-time Tailscale login in the browser; free, end-to-end TLS terminated on the owner's machine",
    "cloudflare_quick": "Start a Cloudflare quick tunnel (cloudflared): a random https://*.trycloudflare.com link with no account, but it changes on every restart",
    "own_relay": "Run a project relay (e.g. relay.vigia.app/<id>) that forwards encrypted traffic to each home",
    "lan_only": "Only the home network: the phone pairs on the same Wi-Fi by local address, nothing outside"}}}
r = call({"product": PRODUCT}, QS)
json.dump(r, open("decisions_remote.json", "w"), indent=1)
a = r["answers"]["remote"]
print(a["choice"], sorted(((o, round(p, 2)) for o, p in a["probabilities"].items()), key=lambda x: -x[1]))
