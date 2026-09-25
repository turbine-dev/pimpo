# Threat model

Vigia acts for people: it reads their email, sends messages and changes things. This page says what it defends against, how, and what it does not promise. Each defense has a test that fails if it breaks.

## Assets

- Accounts: email, calendar, chats, services with their tokens.
- The house's data: memory, receipts, routines.
- The owner's intent: rules, approvals and budget.

## Attackers

| Attacker | Can |
|---|---|
| A stranger who writes to you | put text in an email, page, calendar invite or photo that Vigia reads |
| A malicious routine or connector author | publish to the gallery, or offer a connector |
| Someone on your network | reach ports on your machine |
| Another person in the house | use Vigia with their own role |
| A thief with your phone | open the paired app |

## Defenses

| Threat | Defense | Test |
|---|---|---|
| Prompt injection in content read | External content is data. Irreversible actions always need approval. Explorations only simulate changes. | `TestGateInjectionLeaksNoSecret` |
| Poisoned memory | Anything the agent notes is low trust and never reaches later instructions. Facts are single lines, and topics cannot write outside the memory. | `TestGatePoisonedMemoryNeverBecomesInstruction` (50 attacks) |
| A destructive request after context was lost | Rules live outside the model, and deletes go to the trash. | `TestGateTwoHundredEmails` |
| Runaway cost or loops | The budget is checked before every model call, with call limits per run. | `TestGateLoopStopsAtBudget` |
| Leaking secrets | Secrets sit in an encrypted vault whose key is in the OS keychain. Models see capabilities, never credentials. Backups seal secrets with a passphrase. | `TestExportImportRoundTrip`, vault tests |
| A routine doing more than it says | The runtime exposes only declared capabilities. The gallery signs content and audits each routine with every capability reachable. | `TestAuditFindsUndeclaredCalls`, `TestTamperingIsCaught` |
| A connector doing more than it says | It runs as a separate process with a clean environment, exactly its declared tools, timeouts, and risk-based rules. | `TestConnectorsAreHeldToTheirManifest` |
| Known exfiltration endpoints and patterns | A signed protection list is checked before every action, including other agents' actions through the Guard. | `TestGuardBlocksListedThings`, `TestGuardForOtherAgents` |
| Another person in the house | Memory, accounts, chats and approvals are separate per person. Guests' changes always wait for their responsible person. | `TestGateFamilyIsolation` |
| Messages to third parties and physical actions | `whatsapp.send_to` and `ha.critical` always ask, whatever the rules say. | `TestWhatsAppToOthersAlwaysAsks` |
| Network exposure | The server listens on loopback. Home mode listens only on a private address (never a public one) and is off by default. Remote access uses the Tailscale built into Vigia: Funnel ends TLS on this machine, so Tailscale never sees the traffic in the clear. Every way in needs a per-device token that can be revoked. | `TestDevicePairingAndRevocation`, `TestLANUsesOnlyHomeAddresses`, `TestPhoneLinksWithTailscaleAndHome` |
| Forged webhooks | WhatsApp checks Meta's signature. The generic channel signs what it sends. | `TestWhatsAppChannel`, `TestGenericChannel` |
| Tampered history | Events are chained by hash and checked on import. | event store tests |
| A lost phone | Revoke the device in Settings; the owner's session is unaffected. | `TestDevicePairingAndRevocation` |

## Not promised

- A compromised computer. Anyone who controls the machine Vigia runs on controls Vigia.
- A model's judgment. Judgments are probabilities; anything irreversible waits for a person.
- A person who approves something harmful. The receipts show it, and undo works where the service allows.
- Services' own security (Google, Meta, Telegram).
