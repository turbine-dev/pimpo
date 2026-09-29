# Threat model

Pimpo acts for people: it reads their email, sends messages and changes things. This page says what it defends against, how, and what it does not promise. Each defense has a test that fails if it breaks.

## Assets

- Accounts: email, calendar, chats, services with their tokens.
- The house's data: memory, receipts, routines.
- The owner's intent: rules, approvals and budget.

## Attackers

| Attacker | Can |
|---|---|
| A stranger who writes to you | put text in an email, page, calendar invite or photo that Pimpo reads |
| A malicious routine or connector author | publish to the gallery, or offer a connector |
| Someone on your network | reach ports on your machine |
| Another person in the house | use Pimpo with their own role |
| A thief with your phone | open the paired app |

## Defenses

| Threat | Defense | Test |
|---|---|---|
| Prompt injection in content read | External content is data. Irreversible actions always need approval. Explorations only simulate changes. | `TestGateInjectionLeaksNoSecret` |
| Poisoned memory | Anything the agent notes is low trust and never reaches later instructions. Facts are single lines, and topics cannot write outside the memory. | `TestGatePoisonedMemoryNeverBecomesInstruction` (50 attacks) |
| A destructive request after context was lost | Rules live outside the model, and deletes go to the trash. | `TestGateTwoHundredEmails` |
| Runaway cost or loops | The budget is checked before every model call, with call limits per run. | `TestGateLoopStopsAtBudget` |
| Sites the agent drives (`browser.*`) | Off until the owner turns it on; Pimpo's own Chrome profile with Chrome's sandbox kept on; every page, redirects included, on a host the run opened within its scope; pressing a button is irreversible and asks first; passwords are never typed; page text returned as the site's words ([RFC 0002](rfcs/0002-browser.md)). | `TestBrowseFillAndSubmit`, `TestBrowserOnlyWhenTurnedOnAndOnlyWhereAllowed` |
| iMessage and the unofficial WhatsApp | Both answer only the paired owner. iMessage reads Messages' database read-only, only messages that arrive after it starts, and sends with the text as an argument, never inside the script. The unofficial WhatsApp starts only when the owner chose it in Labs and never approves: numbered answers are ignored and notices say to choose elsewhere. | `TestIMessageHandsOverNewMessagesOnly`, `TestUnofficialWhatsAppNeverApproves`, `TestUnofficialWhatsAppWaitsForLabs` |
| Long jobs | A job runs only after the owner sees the plan and starts it, within a budget the owner sets (at most $20) and the day's limit. Each part is an exploration limited to the capabilities the plan gave it (unknown ones are dropped, none means none), so it reads for real and writes only as proposals. The parts' results reach the report as data. | `TestJobPlansRunsPartsAndReports`, `TestJobStopsAtItsBudget`, `TestJobResumesAfterARestart` |
| Voice conversation | The microphone is open only while the owner has the conversation on, and speech is transcribed on the owner's Pimpo (Whisper), never by the browser's cloud recognition. A spoken answer is a new message and never an approval: proposed actions wait for a tap, so someone talking near the device, or audio played to it, cannot approve anything. | `ui/src/test/conversation.test.ts` |
| The phone as a node | Each paired phone reports only what it shares, chosen on the phone itself; a report from a phone that does not share it is refused. Automations use a separate key that can report events and nothing else, and each phone's key and login are revoked with the phone. Positions are compared with the owner's places and dropped; photos stay on the computer and their text reaches routines as data, never as instructions. | `TestPhoneArrivingHomeAndAPhotoOfABill` |
| Learned preferences | Learned only from the owner's own requests and decisions, never from content the agent read; shown to the agent as learned and not confirmed, so they shape answers but never pass a rule or approve an action; the owner confirms or removes each, and a removed one is never learned again. | `TestLearnsOnlyFromTheOwnersOwnWords` |
| Code the agent runs (`code.run`) | Off until the owner turns it on; a fresh Docker container per call with no network, a read-only root, all Linux capabilities dropped, no-new-privileges, an unprivileged user, no environment, and limits on time, memory, CPU, processes and output ([RFC 0001](rfcs/0001-code-sandbox.md)). | `TestEveryRunIsIsolated`, `TestCodeRunsOnlyWhenTurnedOn`, `TestLive` (with Docker) |
| A malicious skill (SKILL.md) | A skill is third-party text the agent loads as reference, never as the owner's words; once loaded, the rest of that exploration reaches only the capabilities the owner granted it (none means none), its scripts are never run, the protection list refuses known ones by hash, and a skill changed on disk stops working. | `TestInstallASkill`, `TestProtectionListRefusesASkill` |
| A crafted email subject steering suggestions | Suggestions see metadata only, told it is data; they never act: accepting one shows the request and starts an ordinary exploration, which simulates changes. Each round is capped at $0.02 within the daily limit. | `TestSuggestionsFromMetadataOnly` |
| Leaking secrets | Secrets sit in an encrypted vault whose key is in the OS keychain. Models see capabilities, never credentials. Backups seal secrets with a passphrase. | `TestExportImportRoundTrip`, vault tests |
| A routine doing more than it says | The runtime exposes only declared capabilities. The gallery signs content and audits each routine with every capability reachable. | `TestAuditFindsUndeclaredCalls`, `TestTamperingIsCaught` |
| A connector doing more than it says | It runs as a separate process with a clean environment, exactly its declared tools, timeouts, and risk-based rules. | `TestConnectorsAreHeldToTheirManifest` |
| Known exfiltration endpoints and patterns | A signed protection list is checked before every action, including other agents' actions through the Guard. | `TestGuardBlocksListedThings`, `TestGuardForOtherAgents` |
| Another person in the house | Memory, accounts, chats and approvals are separate per person. Guests' changes always wait for their responsible person. | `TestGateFamilyIsolation` |
| Messages to third parties and physical actions | `whatsapp.send_to` and `ha.critical` always ask, whatever the rules say. | `TestWhatsAppToOthersAlwaysAsks` |
| Network exposure | The server listens on loopback. Home mode listens only on a private address (never a public one) and is off by default. Remote access uses the Tailscale built into Pimpo: Funnel ends TLS on this machine, so Tailscale never sees the traffic in the clear. Every way in needs a per-device token that can be revoked. | `TestDevicePairingAndRevocation`, `TestLANUsesOnlyHomeAddresses`, `TestPhoneLinksWithTailscaleAndHome` |
| Backups in someone else's storage | Cloud backups are the full export encrypted as a whole on this machine (scrypt + AES-GCM) before upload, so S3 or Google cannot read the database, memory or secrets. Google Drive access is limited to files Pimpo created (`drive.file`). Storage keys and the passphrase stay in the vault and are never sent back to the interface. Retention deletes only files named like Pimpo backups. | `TestSealedBackupsHideEverything`, `TestCloudBackupsToS3`, `TestS3SignsEveryRequest` |
| A third-party MCP server | Only the owner can add one. Pimpo lists its tools and the owner sets each tool's risk; tools without hints count as irreversible and ask first. Packages are pinned to a version, local servers get a clean environment with only the variables given, remote servers need https, and keys live in the vault. Tools added later stay hidden; a tool that vanishes blocks the connector until it is reviewed again. | `TestAddMCPServerFromTheRegistry`, `TestImportedRemoteConnector`, `TestProbeSuggestsRisksFromHints` |
| A chat talked into acting | Chat messages run as explorations: reads are real, changes are simulated and shown. Confirming performs only the recorded actions of that one answer, once, through the policy and approvals like a routine; the agent cannot add actions at confirm time. A conversation is visible only to the person who started it. | `TestChatRehearsesThenDoesExactlyWhatItShowed` |
| An assistant stepping outside its role | An assistant's tools are a list the host enforces: it is shown only those tools and any other call is blocked and recorded. Its instructions go to the agent as a job description, never as a way around rules. Only the owner creates assistants. | `TestAssistantOnlyUsesItsCapabilities` |
| Someone else messaging the Discord, Slack or Signal bot | A channel answers only the account that paired with the owner's one-time code; everyone else is ignored, and numbered answers only resolve the choices of the last notice sent to the owner. Notices for other people never reach these channels. | `TestSignalLikeChannelPairsAndAnswersByNumber` |
| An API model running up a bill | An API model runs only with a price set by the owner; its tokens are counted into the daily budget on every turn and the loop stops at the call's cost limit. Keys stay in the vault. Its tool calls go through Pimpo's MCP server, so rules and approvals apply exactly as with Claude Code. | `TestExploreWithAnAPIModel`, `TestAnthropicAgentLoop` |
| An email that tries to steer a routine's written text | A routine's write step only returns text: the item is passed as data with a system prompt that forbids following it, the output is capped in length and recorded, and anything the routine does with it goes through the policy and approvals like any other call. Each text is checked against the budget first and a run may ask for at most 20. | `TestWriteStep`, `TestWriteIsBudgetedAndRecorded`, `TestWriteStepPrompt` |
| Forged webhooks | WhatsApp checks Meta's signature. The generic channel signs what it sends. | `TestWhatsAppChannel`, `TestGenericChannel` |
| Tampered history | Events are chained by hash and checked on import. | event store tests |
| A lost phone | Revoke the device in Settings; the owner's session is unaffected. | `TestDevicePairingAndRevocation` |

## Not promised

- A compromised computer. Anyone who controls the machine Pimpo runs on controls Pimpo.
- A model's judgment. Judgments are probabilities; anything irreversible waits for a person.
- A person who approves something harmful. The receipts show it, and undo works where the service allows.
- Services' own security (Google, Meta, Telegram).
