#!/usr/bin/env python3
"""Settle the technical decisions of phases F6 to F12 with Jev. Writes decisions_later.json."""
import json
from prioritize import PRODUCT, call

CTX = " The product is a free, open-source, self-hosted Go single binary run by non-technical people at home; safety and low maintenance for a solo founder matter more than feature breadth."
QS = {
    "whatsapp": {"type": "choice", "instructions": "How should the `product` connect to WhatsApp as a second channel?" + CTX,
                 "criteria": {"cloud_api": "Official WhatsApp Business Cloud API: the user creates a Meta app and a business number; stable and allowed, but setup takes time and messages outside a 24h window need paid templates",
                              "local_bridge": "Unofficial local bridge (whatsmeow, linked device on the user's own number): easy setup, but against WhatsApp's terms with a real risk of the number being banned",
                              "both": "Both, the official API as default and the bridge behind a warning"}},
    "gallery_signing": {"type": "choice", "instructions": "How should routines in the public gallery be signed and verified by clients?" + CTX,
                        "criteria": {"sigstore": "Sigstore keyless signing (OIDC identity, transparency log), verified in Go with sigstore-go",
                                     "minisign": "Author-held Ed25519 keys (minisign style) listed in the index, verified with the Go standard library",
                                     "ssh": "Git SSH signatures of the index repository only"}},
    "voice": {"type": "choice", "instructions": "How should voice notes (speech to text) work?" + CTX,
              "criteria": {"local_whisper": "Local whisper.cpp model downloaded on demand", "provider_api": "The user's model provider speech API", "local_then_api": "Local whisper when installed, provider API otherwise"}},
    "pdf": {"type": "choice", "instructions": "How should invoices and quotes be rendered to PDF?" + CTX,
            "criteria": {"pure_go": "A pure Go PDF library with simple templates", "headless_browser": "HTML templates printed by a headless Chromium", "typst": "Typst templates with the typst binary"}},
    "ocr": {"type": "choice", "instructions": "How should photos of receipts and handwritten quotes be read?" + CTX,
            "criteria": {"vision_llm": "The user's multimodal model, with the answer checked by rules", "tesseract": "Local Tesseract OCR", "both": "Tesseract first, the model when confidence is low"}},
    "push": {"type": "choice", "instructions": "How should the phone companion get instant approval notifications without an intermediary server run by the project?" + CTX,
             "criteria": {"ntfy": "ntfy (self-hostable, or the public ntfy.sh with a random topic and encrypted payload)", "telegram_only": "Keep Telegram as the push channel, the app only for review", "web_push": "Standard Web Push from the PWA with VAPID keys"}},
    "connector_sdk": {"type": "choice", "instructions": "What form should third-party connectors take?" + CTX,
                      "criteria": {"go_compiled": "Go packages implementing an interface, compiled into the binary by contribution", "mcp_process": "Separate processes speaking MCP over stdio, with declared capabilities and risk", "wasm": "WebAssembly modules run in-process with wazero"}},
    "protection_network": {"type": "choice", "instructions": "How should the shared protection list (malicious skills, exfiltration domains, dangerous action patterns) be distributed?" + CTX,
                           "criteria": {"signed_static": "A signed static file in a public git repository, pulled daily, contributions by pull request", "p2p": "Peer-to-peer gossip between installs", "server": "A small hosted API run by the project"}},
}
r = call({"product": PRODUCT}, QS)
json.dump(r, open("decisions_later.json", "w"), indent=1)
for k, a in r["answers"].items():
    print(k, a["choice"], sorted(((o, round(p, 2)) for o, p in a["probabilities"].items()), key=lambda x: -x[1]))
