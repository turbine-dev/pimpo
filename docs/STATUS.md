# Status

What is built, what each phase gate needs, and which gates are waiting on people or time rather than code.

| Phase | Built | Gate | Gate status |
|---|---|---|---|
| F0 Foundation and compiler proof | Runtime, compiler, event log, vault, server, UI shell | ≥ 8/10 recorded tasks compile on the first attempt and pass a holdout | **Passed: 8/10** ([proof](proof/README.md)) |
| F1 The compiled routine | Connectors (iCal, IMAP, HTTP, Telegram), exploration over MCP, scheduler, judgments (Jev, LLM, Ollama), daily budget, owner channel, UI with real data | 30 consecutive daily runs of the morning brief, ≥ 29 successful, ≤ $0.01 LLM cost per run, every failure alerted within 5 minutes | **Waiting on time:** needs 30 days of real use. The live end-to-end test passes with Claude (`go test -tags live -run Live ./internal/app`) |
| F2 Secure by construction | Rules engine (balanced preset, plain-language rules compiled and confirmed, tested against last week), approvals that pause the run, reversible forms (trash, cancellable outbox), undo, receipts, repairs that keep old tests, cost view with projection | 200-email scenario stopped and reversible; 50 injections leak nothing; runaway loop stops within budget | **Passed** (automated in CI: `go test -run Gate ./internal/app`) |
| F3 The UI that tells the story | Demo mode (`vigia serve --demo`), first-run guide with autonomy presets, compile moment with tests and capabilities, decisions shown in explorations, version diffs, command palette (⌘K), "all this time" and "always" approvals with a suggestion after 3, skeletons and an error boundary, installable PWA, WCAG AA palette | 4 of 5 new users complete four tasks without help, each within 10 minutes | **Waiting on people:** usability sessions. The same four tasks pass as browser tests (`make e2e`), plus WCAG AA checks on every page in both themes |
