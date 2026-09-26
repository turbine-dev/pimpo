# Extending Pimpo

Every way to extend Pimpo works with the binary you already have: no recompiling, no fork. Each extension point is a stable file format or HTTP API, and each is held to the same safety promises as Pimpo's own code.

| Extend | With | Details |
|---|---|---|
| New services | **Connectors**: any language, MCP over stdio, a `connector.json` | [CONNECTORS.md](CONNECTORS.md) |
| New chat apps | **Channel API**: a signed webhook out, two endpoints in | below |
| New judgment models | **Judge API**: one HTTP endpoint | below |
| Ready-made automations | **Gallery routines**: a signed JSON file, with typed settings (`params`) people change without code | [gallery/README.md](../gallery/README.md) |
| Other agents | **Guard API**: ask before running a tool | [guard/README.md](../guard/README.md) |
| Threat intelligence | **Protection list**: signed entries | [protection/README.md](../protection/README.md) |

## Authentication

Every API call carries a device token: `Authorization: Bearer <token>`. Create one in **Ajustes › Abrir no celular** by giving the device a name (for example "Matrix bridge"). It can be revoked there at any time.

## Channel API

A bridge for Matrix, Signal, SMS or a smart speaker.

**Out.** Set the bridge's address with `PUT /api/channel/webhook {"url": "https://bridge.example/pimpo"}`. The answer carries a `secret`. From then on, every notice (results, approvals, alerts) is POSTed to the bridge:

```json
{"to": "owner", "text": "🟠 Posso fazer isto?\n…", "actions": [{"label": "Permitir", "data": "approve:ab12"}, {"label": "Negar", "data": "deny:ab12"}], "sent": "2026-09-24T10:00:00Z"}
```

Check `X-Pimpo-Signature: sha256=<hex>`, which is the HMAC-SHA256 of the body with the secret. `to` is the person the notice is for (`owner`, or a person id from People).

**In.**

- `POST /api/channel/message {"text": "resuma meus e-mails", "person": "owner"}`: a request, answered with `{"reply": "…"}`.
- `POST /api/channel/button {"data": "approve:ab12", "person": "owner"}`: a button tap. People can only answer what they are allowed to answer, as on Telegram.

## Judge API

Point **Ajustes › Modelo local** at any server answering:

```
POST /judge   {"question": "Is this email a bill?", "item": {…}}   →   {"p": 0.93}
```

`p` is the probability of yes. `tools/judge/serve.py` is the reference, and serves Pimpo's fine-tuned 0.6B model with MLX. With the local backend selected, unsure answers (p between 0.1 and 0.9) go to Jev or your model: see `tools/judge/results`.

## Guard API

```
POST /api/guard/check  {"agent": "myagent", "tool": "bash", "params": {"command": "ls"}, "session": "run-1"}
→ {"decision": "allow" | "ask" | "block", "reason": "…", "capability": "guard.exec"}
```

The decision uses the owner's rules and the protection list, and lands in Receipts. The OpenClaw and Hermes plugins in `guard/` are complete examples.

## Stability

These formats and endpoints follow semantic versioning from v1.0: fields are added, never removed or repurposed, within a major version. Breaking changes need an RFC.
