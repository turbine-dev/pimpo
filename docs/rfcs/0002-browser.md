# RFC 0002: A browser the agent drives

- Author: denerFernandes
- Status: accepted (behind **Laboratório**, off by default)
- Discussion: roadmap item 4.1 ([ROADMAP.md](../ROADMAP.md))

## Problem

Many everyday tasks happen on sites with no API: a utility's bill, a government portal, the status of an order, a booking. `web.read` reads a page, but cannot click, fill a form or use the owner's login. Driving a browser is the power people use OpenClaw for most, and the one that most needs containing.

## Proposal

Capabilities that drive Pimpo's own Chrome, scoped by site like `web.read` (`browser.open:www.example.com`):

| Capability | Risk | What it does |
|---|---|---|
| `browser.open({url})` | read | opens a page and returns it: title, text, and its interactive elements, each with a `ref` |
| `browser.read({})` | read | the current page again |
| `browser.follow({ref})` | read | follows a link (an `<a href>`) to another page in scope |
| `browser.type({ref, text})` | reversible | types into a field, without submitting |
| `browser.choose({ref, value})` | reversible | picks an option or ticks a box |
| `browser.click({ref})` | irreversible | presses a button or anything that may submit, buy or send |

- **Scope**: every page a call reaches must be on a host the manifest (or the exploration's approvals) names; a link or redirect elsewhere is refused.
- **Risk**: pressing a button can do anything, so it is irreversible and asks first by default, naming the button, the page and what was typed. Reading and following links change nothing.
- **Explorations** read and navigate for real; typing and clicking are rehearsed, as for any change, and the compiled routine does them, with approval as the rules say.
- **Logins**: Pimpo's browser has its own profile, not the owner's. The owner signs in to the sites they want, once, in a visible window of that profile (**Ajustes › Laboratório › Abrir o navegador do Pimpo**); routines then use those sessions. The profile stays in the data folder and travels with backups like the vault does not: it is excluded from exports.
- **Content**: everything on a page is data. The text returned is capped and marked as the page's; the protection list's domains and patterns apply to every call.
- **Isolation**: one headless Chrome, one tab per run, closed after it; no downloads; pages cannot open new windows.

Like the code sandbox, it is a Labs feature, off by default.

## Safety

- *Actions without approval*: only `browser.click` can submit, and it is irreversible, so the rules ask first unless the owner chose otherwise for that routine.
- *Leaked secrets*: Pimpo never types a password from the vault into a page; logins happen by the owner's hand. A routine typing text it read elsewhere into a site is visible in its manifest (scope) and its receipts.
- *Instructions smuggled from content*: page text is returned as data; a page telling the agent to do something is the same as an email doing so, covered by the existing defenses and tests.
- *Reaching other sites*: scopes are checked on every navigation, redirects included.

Tests: an in-process site with forms and links; scope refusals, redirects out of scope, the element listing, typing without submitting, and a click that submits.

## Alternatives

- **The owner's own Chrome profile**: would reach every site they are signed in to. Rejected: the profile Pimpo drives must contain only what the owner chose to sign in to for it.
- **Playwright**: capable, but it brings its own browser downloads and a Node process. chromedp talks to an installed Chrome over the DevTools protocol from Go.
- **Screenshot-and-click with a vision model**: more general, far more expensive, and harder to compile into a routine. Element refs are cheaper and replayable.

## Migration

Nothing changes until the owner turns it on. Routines that use it declare `browser.*` capabilities with hosts, like any scoped capability.
