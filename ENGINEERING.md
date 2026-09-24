# Engineering Guidelines

This is how we build Vigia. It applies to everyone who sends code, docs or reviews, including people who use AI tools to help them.

The rules are short on purpose. If something here gets in the way of good work, open an issue and we'll change it. Until then, follow it.

## The three rules that matter most

1. **Nothing merges without tests.** If you changed behavior, a test proves it. If you fixed a bug, a test fails before your fix and passes after.
2. **You own every line you submit.** It doesn't matter who or what typed it. If you can't explain a line in review, it doesn't go in.
3. **Write like a person who respects the reader.** Code, comments, commits and docs should be plain, specific and short. No filler.

Everything below is detail on those three.

---

## 1. Testing

### What has to be tested

Everything that has behavior:

- Go packages: every exported function that does more than pass values through.
- Event handling: every event type we emit and every event type we consume.
- Connectors: every connector, against recorded API responses.
- SQLite: every migration, from every released version.
- The routine runtime: every capability binding, including calls outside the manifest, which must fail.
- The policy engine: every decision path (allow, make reversible, ask, block), including bad input.
- UI: every component with logic, and the main flows end to end.
- CLI: every command, including its error output.

Trivial code (a struct with no methods, a constant) doesn't need its own test. If you're unsure whether something is trivial, it isn't.

### How we test Go code

- Use the standard `testing` package. Table-driven tests where there's more than one case.
- Always run with the race detector: `go test -race ./...`. CI does.
- Prefer real dependencies over mocks. SQLite goes to a temp file (`t.TempDir()`). Git runs for real in a temp repo. These are fast enough.
- Use the **fake LLM** and **recorded connectors** for anything involving models or external APIs. They replay recorded responses, so tests are deterministic and cost nothing.
- Use golden files for event streams and CLI output. Update them with `go test ./... -update`, and review the diff before committing.
- No `time.Sleep` in tests. Wait on a channel, or inject a clock.
- Tests must not touch the network. Live tests against real agents live behind the `live` build tag and never run in CI by default.

```go
func TestHandoffRoundTrip(t *testing.T) {
    tests := []struct {
        name    string
        in      Handoff
        wantErr bool
    }{
        {"read inside manifest", readCall("gmail.search"), false},
        {"capability not declared", readCall("telegram.send"), true},
        {"irreversible without approval", deleteCall(), true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### How we test the UI

- Unit and component tests with Vitest and Testing Library. Test what the user sees and does, not internal state.
- End-to-end tests with Playwright against the real binary running the fake LLM. At minimum: create the first routine, approve an action, undo it, write a rule, read the receipts.

### Connector contract tests

Every connector ships with:

- recorded API responses under `testdata/<connector>/`;
- a contract test that replays them and checks the capability results and risk levels;
- an optional live test (`-tags live`) that calls the real API.

When a provider changes its API, record new responses, fix the connector, and keep the old recording if we still support that version.

### Bugs

A bug fix PR starts with a failing test. Commit the test first if it helps review. No test, no fix.

### Flaky tests

A flaky test is a bug. Fix it or delete it the same day. Don't add retries to make it pass.

### Coverage

We don't chase a coverage number. We do look at it in review: a new package with no tests won't be approved.

### Before you open a PR

```bash
make check
```

This runs `gofmt`, `go vet`, `staticcheck`, `go test -race ./...`, the UI linters and the UI tests. If it fails locally, it will fail in CI.

---

## 2. Language

Everything in the project is in English. No exceptions for "internal" things.

- Code: identifiers, log messages, error messages, CLI output.
- Comments and doc comments.
- Commit messages, branch names, PR titles and descriptions, review comments.
- Issues, discussions, docs, changelog, test names and test data.
- UI text. Translations can come later through proper i18n, never hard-coded strings in other languages.

Simple, clear English beats fancy English. Many contributors are not native speakers, so write for them: short sentences, common words, no idioms or jokes that don't translate.

---

## 3. Comments

Most code should need no comments. If you feel the need to explain what a block does, first try a better name or a smaller function.

Write a comment only when it says something the code can't:

- **Why** something is done in a non-obvious way: a workaround, a vendor quirk, a performance reason.
- **Constraints** the reader would otherwise break: ordering, locking, invariants.
- **Doc comments on exported API**, written for the caller, one or two sentences.
- A link to the issue or upstream bug when the code exists because of it.

Don't write:

- comments that restate the code (`// loop over agents`, `// return the result`);
- a doc comment on every private function;
- section banners (`// ===== Helpers =====`);
- changelogs or author names in comments (that's what Git is for);
- commented-out code;
- `TODO` without a linked issue.

```go
// Bad: says what the code already says.
// Check if the agent is paused and return an error if it is.
if a.paused {
    return ErrPaused
}

// Good: explains something the reader can't see.
// Claude Code only reads stdin between turns, so a pause request
// takes effect at the next turn boundary, not immediately.
func (a *claudeAdapter) Pause(ctx context.Context) error {
```

A good test for any comment: if you deleted it, would a competent reader lose information? If not, delete it.

In review, excessive comments are a valid reason to request changes.

---

## 4. Go code

- Format with `gofmt`. Lint with `go vet` and `staticcheck`. No exceptions without a comment explaining why.
- `context.Context` is the first argument of anything that does I/O or can block.
- Wrap errors with context: `fmt.Errorf("compile routine %s: %w", name, err)`. Lowercase, no trailing period.
- No `panic` outside `main` and truly impossible states.
- No package-level mutable state. Pass dependencies in.
- Define interfaces where they're used, not where they're implemented. Keep them small.
- Log with `log/slog`. Structured fields, not formatted strings.
- Standard library first. A new dependency needs a reason in the PR description.
- Package names are short nouns: `event`, `policy`, `routine`. Not `utils`, `common`, `helpers`, `manager`.

## 5. TypeScript and React

- TypeScript strict mode. No `any` unless the value really is untyped, and then narrow it right away.
- Types that cross the wire are generated from Go. Don't hand-write them.
- Function components and hooks. Keep components small; move logic into hooks you can test.
- Server state goes through TanStack Query. Live events go through the WebSocket store. Don't mix them.
- No new UI library without discussion.

## 6. Events and data

Events are the source of truth. Replay depends on them, so they have strict rules:

- Events are append-only. Never update or delete an event row.
- Every event has a `type`, a timestamp, an actor and a task. No exceptions.
- Changing an event's shape means adding fields, never renaming or removing them. If you must break it, add a new event type.
- Every schema change comes with a migration and a test that runs it on a database from the previous release.
- Routine manifests and policy files follow the same rules.

## 7. Agents and security

- A routine only reaches the world through the capabilities in its manifest, and every call passes the policy engine. Code that bypasses either is a security bug.
- Never log or store tokens, API keys or credentials, even in debug mode.
- No network calls except the ones the user asked for (agent CLIs, GitHub when configured).
- No telemetry by default. Anything we collect is local and opt-in.
- Anything an agent does on the user's behalf (commits, PRs, comments) is recorded as an event and visible in the timeline.

---

## 8. Using AI tools

Vigia is built with coding agents too, so of course you can use them here. The rules are the same as for code you wrote by hand, with a few additions because AI output has recognizable habits.

### You are the author

- Read every line of your diff before you push. All of it.
- Run it. Test it. Break it on purpose.
- If a reviewer asks "why is this here?", "the model added it" is not an answer.
- Don't paste AI-generated review comments or issue replies. Write your own.

### What AI-written code tends to look like, and what to do instead

Reviewers will push back on these. Fix them before review.

| Don't | Do |
|---|---|
| Comments that repeat the code (`// increment counter`) | Comment only the why, or nothing |
| A doc comment on every trivial function | Doc comments on exported API, written for the caller |
| Generic names: `data`, `result`, `item`, `handleThing`, `processData`, `Manager` | Names from the domain: `routine`, `capability`, `pendingApproval` |
| Checks for things that can't happen, defensive `nil` checks everywhere | Validate at the boundary, trust your own types inside |
| Abstractions with one implementation "for flexibility" | Write the concrete thing. Extract when the second case shows up |
| Config options nobody asked for | Hard-code it until someone needs it changed |
| Wrapping every error in a custom type | `fmt.Errorf` with `%w` until there's a reason |
| Dead code, commented-out code, unused parameters | Delete it. Git remembers |
| Placeholder `TODO: implement` left in | Implement it, or open an issue and link it |
| Logging on every line "for debugging" | Log what an operator would need, at the right level |
| Big rewrites of code near your change | Change what the task needs. Refactors go in their own PR |
| A new style that doesn't match the file | Match the code around you, even if you'd do it differently |

### Writing: docs, commits, PRs, comments

Write the way a senior engineer talks to a colleague.

- Say the specific thing. "Fix race when two agents finish at the same time" beats "Improve robustness of agent lifecycle handling".
- Short sentences. Active voice. Cut words that don't change the meaning.
- No emojis in code, commits, docs or PR descriptions.
- Don't bold half the paragraph. Don't turn every paragraph into bullets.
- Don't open with a summary of what you're about to say, and don't close with a summary of what you said.
- Avoid words that mean nothing in a technical context: *robust, seamless, comprehensive, leverage, utilize, delve, elevate, streamline, cutting-edge, powerful, crucial, ensure* (when "make sure" or nothing will do), *in order to*.
- No hedging ("this should hopefully...") and no cheerleading ("great improvement!").
- If you don't know something, say so plainly.

---

## 9. Commits and pull requests

### Commits

- Imperative subject, 50 characters or so, no period: `Block irreversible calls without an approval`.
- Body explains why, not what. The diff shows what.
- One logical change per commit. Squash your "fix typo" commits before review.

```text
Resume agent from human commit diff, not line numbers

Line numbers drift as soon as the agent edits the file again, so the
agent was sometimes told to preserve the wrong code. Sending the diff
of the human commit gives it something stable to work from.
```

### Pull requests

- Small. If it's over ~400 changed lines (excluding generated files and test data), split it or explain why not.
- The description answers three things: what problem this solves, how you solved it, how you tested it. A few sentences each is fine.
- Link the issue.
- Screenshots or a short recording for UI changes.
- Don't list every file you touched. Reviewers can see the diff.

### Review

- Review within a day or two. If you can't, say so.
- Comment on the code, not the person.
- Separate blocking comments from suggestions. Prefix suggestions with `nit:` or `optional:`.
- Approve when it's correct, tested and readable. Not when it's how you'd have written it.

---

## 10. Docs

- A user-facing feature isn't done until it's documented.
- Docs live next to the code in `docs/`. Examples in docs are tested or copied from tests.
- Explain concepts once, in one place, and link to them.
- Update the changelog in the same PR.

---

## Definition of done

A change is done when:

- [ ] tests cover the new or changed behavior, and a bug fix has a regression test;
- [ ] `make check` passes locally and CI is green;
- [ ] migrations and schema changes are tested against the previous release;
- [ ] you read the whole diff yourself and can explain every line;
- [ ] no dead code, placeholder TODOs or leftover debug logging;
- [ ] names, comments and style match the surrounding code;
- [ ] comments only where they explain why, no restating the code;
- [ ] everything is in English: code, comments, commits, PR, docs;
- [ ] docs and changelog are updated if users will notice;
- [ ] the PR says what, how and how it was tested.

If one of these doesn't apply, say so in the PR.
