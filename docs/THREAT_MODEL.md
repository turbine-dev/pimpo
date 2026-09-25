# Threat model

Zodim acts for people: it reads their email, sends messages and changes things. This page says what it defends against, how, and what it does not promise. Each defense has a test that fails if it breaks.

## Assets

- Accounts: email, calendar, chats, services with their tokens.
- The house's data: memory, receipts, routines.
- The owner's intent: rules, approvals and budget.

## Attackers

| Attacker | Can |
|---|---|
| A stranger who writes to you | put text in an email, page, calendar invite or photo that Zodim reads |
| A malicious routine or connector author | publish to the gallery, or offer a connector |
| Someone on your network | reach ports on your machine |
| Another person in the house | use Zodim with their own role |
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
| Network exposure | The server listens on loopback. Home mode listens only on a private address (never a public one) and is off by default. Remote access uses the Tailscale built into Zodim: Funnel ends TLS on this machine, so Tailscale never sees the traffic in the clear. Every way in needs a per-device token that can be revoked. | `TestDevicePairingAndRevocation`, `TestLANUsesOnlyHomeAddresses`, `TestPhoneLinksWithTailscaleAndHome` |
| Backups in someone else's storage | Cloud backups are the full export encrypted as a whole on this machine (scrypt + AES-GCM) before upload, so S3 or Google cannot read the database, memory or secrets. Google Drive access is limited to files Zodim created (`drive.file`). Storage keys and the passphrase stay in the vault and are never sent back to the interface. Retention deletes only files named like Zodim backups. | `TestSealedBackupsHideEverything`, `TestCloudBackupsToS3`, `TestS3SignsEveryRequest` |
| A third-party MCP server | Only the owner can add one. Zodim lists its tools and the owner sets each tool's risk; tools without hints count as irreversible and ask first. Packages are pinned to a version, local servers get a clean environment with only the variables given, remote servers need https, and keys live in the vault. Tools added later stay hidden; a tool that vanishes blocks the connector until it is reviewed again. | `TestAddMCPServerFromTheRegistry`, `TestImportedRemoteConnector`, `TestProbeSuggestsRisksFromHints` |
| A chat talked into acting | Chat messages run as explorations: reads are real, changes are simulated and shown. Confirming performs only the recorded actions of that one answer, once, through the policy and approvals like a routine; the agent cannot add actions at confirm time. A conversation is visible only to the person who started it. | `TestChatRehearsesThenDoesExactlyWhatItShowed` |
| An assistant stepping outside its role | An assistant's tools are a list the host enforces: it is shown only those tools and any other call is blocked and recorded. Its instructions go to the agent as a job description, never as a way around rules. Only the owner creates assistants. | `TestAssistantOnlyUsesItsCapabilities` |
| Someone else messaging the Discord, Slack or Signal bot | A channel answers only the account that paired with the owner's one-time code; everyone else is ignored, and numbered answers only resolve the choices of the last notice sent to the owner. Notices for other people never reach these channels. | `TestSignalLikeChannelPairsAndAnswersByNumber` |
| An API model running up a bill | An API model runs only with a price set by the owner; its tokens are counted into the daily budget on every turn and the loop stops at the call's cost limit. Keys stay in the vault. Its tool calls go through Zodim's MCP server, so rules and approvals apply exactly as with Claude Code. | `TestExploreWithAnAPIModel`, `TestAnthropicAgentLoop` |
| Forged webhooks | WhatsApp checks Meta's signature. The generic channel signs what it sends. | `TestWhatsAppChannel`, `TestGenericChannel` |
| Tampered history | Events are chained by hash and checked on import. | event store tests |
| A lost phone | Revoke the device in Settings; the owner's session is unaffected. | `TestDevicePairingAndRevocation` |

## Not promised

- A compromised computer. Anyone who controls the machine Zodim runs on controls Zodim.
- A model's judgment. Judgments are probabilities; anything irreversible waits for a person.
- A person who approves something harmful. The receipts show it, and undo works where the service allows.
- Services' own security (Google, Meta, Telegram).
