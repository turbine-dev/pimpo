# Pimpo user guide

This guide walks through the app. For the command line, settings and the data folder see [Configuration](CONFIGURATION.md); for how routines work inside and how to write one see [Routines](ROUTINES.md); to add services see [Connectors](CONNECTORS.md).

Pimpo was called Zodim, and Vigia before that. An existing install moves over on its own the first time Pimpo opens (with the old app closed): data, keys, backups and paired phones keep working.

## Getting around

**Início** is where the day starts: ask for anything, and see what needs you, what ran and what is next today, what you spent, and your recent chats. The side menu keeps what you use daily on top (Início, Rotinas, Atividade, Assistentes) and the rest under **Mais**; your conversations are listed below it. At the bottom, the bell opens what waits for you (approvals, stopped routines, tasks ready, parts with errors) so you can answer in place, and the Pimpo menu has Ajustes, cost, **Ocupação do sistema** (⌘⇧D: how busy the computer and Pimpo are, what is running now and the state of every channel, account and service), pairing a phone, the theme and help.

## First steps

1. **Install.** Use the desktop app from the [releases page](https://github.com/turbine-dev/pimpo/releases) (macOS, Windows, Linux), or run `curl -fsSL https://raw.githubusercontent.com/turbine-dev/pimpo/main/scripts/install.sh | sh` and then `pimpo serve`. Open the link it prints.
2. **Choose the model that thinks for Pimpo.** The welcome screen shows what this computer already has (Claude Code, Codex, opencode, a local model in Ollama or LM Studio) or takes one API key (Anthropic, OpenAI, Gemini, DeepSeek, OpenRouter and others). Pimpo picks the models for each job and tests them. A model is only needed to learn a task the first time; the routines it makes run without one.
3. **Pick your safety level.** The welcome screen offers conservative, balanced (recommended) and liberal.
4. **Connect what you need** in Conexões: Telegram or WhatsApp to talk from your phone, then email and calendar. "Entrar com Google" connects both at once with your own Google client.
5. **Ask for something you do every week.** Use "Nova tarefa", or send a message on Telegram.

Want to see it first? Run `pimpo serve --demo`: a sample mailbox and calendar, no accounts, no costs.

## Pimpo, the mascot

Pimpo is a tuxedo kitten, black with a white blaze and muzzle. He is off until you turn him on in **Ajustes › Geral** (or, in the desktop app, **Pimpo na área de trabalho** in the menu bar).

In the desktop app he lives in his own small window, independent of the app: drag him anywhere on the screen, over any program; he stays there with the Pimpo window closed, follows you across desktops and comes back where you left him. He only goes away when you turn him off or quit Pimpo. In a browser he stays inside the page, since a web page cannot draw outside it.

He shows how things are going: he blinks and swishes his tail while all is well, looks from side to side while a task runs, perks up when something needs you, smiles when a routine works, droops his ears when one fails, and falls asleep after a few quiet minutes. Now and then, when all is calm, he plays for a few seconds: chasing a butterfly, batting a ball of yarn, yawning and stretching, or washing his paw (never while a notice is open, and not when the system asks for reduced motion). Notices appear in his speech bubble with **Ver** and **Depois**; approvals stay until you answer. Click him for a small menu (new task, what needs you, give a treat, silence for an hour, hide or turn off) and hover to pet him. Give him a treat and he jumps to catch a little fish in the air, then chews happily and purrs (the purr is made by the app on the spot, and only plays when you feed him). Turn him off and he waves his paw and trots off the screen. He only shows what already happened; he never acts on his own.

## Language

Pimpo speaks Português, English, Español, Français, Deutsch, Italiano, 日本語, 简体中文, 한국어 and Русский. Choose in **Ajustes › Geral**: the app, the messages on Telegram and the other channels, approval requests, connector instructions and the dates routines write all follow it. Tasks are answered in the language you ask in. The phone's pairing screen follows the phone's language. This guide is in English only.

## Chat

**Conversar** is a conversation with Pimpo in the app, with every past conversation on the side. Each message remembers the ones before it. Pimpo reads for real (your calendar, your email), but any change it would make is only rehearsed and listed under the answer, with its risk. **Confirmar e fazer** does exactly those changes, once, each still under your rules and approvals; **Transformar em rotina** makes the task repeat on its own.

**Assistentes** (from the chat's side panel) are roles for the agent: a name, what its job is, and the tools it may use. Pick one when starting a conversation. Tap the microphone to speak instead of typing: the computer's own dictation is used when there is one, otherwise the audio is transcribed on this machine with whisper.cpp; a question you spoke is answered aloud, and **Ouvir** reads any answer. The limit is enforced by Pimpo itself: the assistant only sees its tools, and any other call is blocked, whatever the text says. A routine made from that conversation uses only those tools too.

## How a task becomes a routine

1. **Exploration.** The agent does the task once. Anything that would change something (archive, send, delete) is only simulated, and you see exactly what would happen.
2. **Routine.** If you like the result, tap "Transformar em rotina". Pimpo writes code with tests and checks it against what the agent did. You can read the code on the routine's page.
3. **On its own.** The routine runs on schedule without a model. If a service changes and the routine breaks, you get a notice with "Refazer", and the agent fixes it.

## Routines that react to something new

Ask for a reaction instead of a time ("me avise quando chegar e-mail da Ana", "quando esse site publicar algo novo") and the routine watches instead of running on a clock. Pimpo checks the source every few minutes without any model, so the checks cost nothing, and runs the routine only with what it has not seen before; the first check only learns what is already there. Change how often it checks in the routine's settings. The empty checks stay out of **Atividade**; what was found and what the routine did show up as usual. The gallery's **E-mail de alguém importante** is one.

A routine can also write a little: "e me diga em uma frase o que ela pede", "sugira uma resposta". The part that must be composed is written by a small model (the one set for judgments) for each item, about a fraction of a cent each; everything that can be copied (sender, subject, date) stays plain code. Each text is checked against the daily limit first, shows up in **Atividade**, and treats the email as data, never as orders. What the routine then does with the text still passes your rules and approvals.

## Routines that remember, and routines built from others

A routine can keep small values between runs: yesterday's price, the items it already sent, a weekly total. Ask naturally ("only tell me if the dollar went up since last time") and the routine keeps what it needs. The routine's **Memória** tab shows what it kept and clears it; nothing is kept from a run that failed, and at most 64 KB.

A new routine can also run a routine you already have and use what it returns ("every morning, count today's appointments using my agenda routine"). The new one must declare everything the other touches, so reusing a routine never widens what the owner approved; if the other routine later needs more, the new one stops and asks to be redone. Routines can use others up to three levels deep and never in a loop. Gallery routines cannot use others, since your routines do not exist on other machines.

## Routines in a repository

**Rotinas › Repositório** keeps your routines as files in a folder you choose, ideally a git repository: `routines/<id>/routine.js` (the code), `routine.json` (name, description, manifest) and `tests.json`. **Enviar rotinas para a pasta** writes them and makes a local commit; **Publicar (git push)** sends them to the remote only when you click it. **Buscar mudanças** runs `git pull --ff-only` (a local edit is never overwritten) and lists every new or changed routine with its tests already run and what it would start being able to touch. Nothing is installed until you click **Instalar** or **Atualizar**, and it is checked again at that moment; a routine that fails its tests, or has none, cannot be installed. Pimpo looks at the repository every 15 minutes and tells you once when something changed. Routines there can be reviewed in pull requests like any code.

## Changing a routine without code

Every routine page starts with **Ajustes da rotina**:

- **Quando**: every day, weekdays, some days of the week, once a month, every few hours, or a cron expression under "Avançado".
- **The routine's own choices**: a city (search by name), a limit, a list of words, a currency, whatever the routine was made with. Pimpo's compiler turns anything personal in your request into one of these, instead of writing it into the code.
- **Onde avisar**: one or more destinations. These can be your Telegram, other Telegram bots (a family group, for example), WhatsApp, Slack, Discord or email. With none chosen, Pimpo uses your usual channel.

To add more Telegram bots, go to **Conexões › Bots extras do Telegram**. Create the bot with @BotFather, paste its token, send it a message (or add it to a group), and tap **Detectar chat**.

## Run history

**Rotinas › Execuções** lists every run of every routine, newest first, with its cost, how many calls it made and how long it took. **Com falha** shows only what went wrong, with the error. A routine can run as often as every 5 minutes (**A cada alguns minutos** in its schedule).

## Approvals and rules

- **Precisa de você** lists what is waiting for you: approvals, finished explorations, problems. On the phone it is the first tab.
- Approval buttons: **Permitir** (this time), **Todos desta vez** (the rest of this run), **Sempre** (this routine, from now on), **Negar**.
- **Regras**: write rules in plain words ("nunca apague e-mails do meu chefe"). You see exactly what the rule will enforce, and a test against last week, before saving.
- Some things always ask, whatever the rules say: WhatsApp to other people, locks and alarms.

## Receipts and undo

Every action has a receipt: what was done, with which arguments, under which rule. Reversible actions can be undone from the receipt. Deletes go to the trash, and sent emails wait 10 minutes before leaving.

## Memory

Pimpo remembers what you tell it. Facts it read somewhere are marked "não confirmado" and never guide it until you confirm them. Every change is versioned: **Histórico › Voltar para aqui** undoes any change.

Search memory in plain words ("what can't I eat?"): with Jev set up, Pimpo finds facts by meaning, not only by the words they share, and the agent uses the same search. Every night Pimpo merges facts that say the same thing, never trading one you confirmed for one it read somewhere, and never merging facts that differ in a date, place or name. **Organizar** does it now, and **Histórico** undoes it.

## People

In **Pessoas**, invite family members as a member or a guest. They send the invite code to the bot and get their own memory and accounts. Choose who approves each person's requests: a guest's changes always wait for that person. Only the owner makes lasting rules.

## On the phone

1. In **Ajustes › Abrir no celular**, turn on one or both ways in:
   - **Em casa**: the phone reaches Pimpo over your Wi-Fi. No account, nothing to install, and it stops working when you leave home.
   - **De qualquer lugar**: Tailscale runs inside Pimpo. The first time, sign in to Tailscale (free) in the browser; Pimpo then gets an `https://pimpo.<your-network>.ts.net` link that works from anywhere. If Tailscale says Funnel or HTTPS is off, turn them on in its admin console as the message explains.
   - Already expose Pimpo another way? Paste the address under **Usar outro endereço**.
2. Name the phone and tap **Gerar código**. With both ways on, the phone uses the home address when it can and the other one elsewhere.
3. Scan the QR code with the Pimpo app, or paste the link.
4. Lost the phone? Tap the trash icon next to it. Only that phone loses access.

## A Pimpo on another computer or server

Pimpo can run on a machine that is always on (a home server, a VPS) while the desktop app just opens it. On that machine, generate a link in **Ajustes › Abrir no celular** as for a phone. On your computer, click the Pimpo icon in the menu bar, choose **Conectar a outro Pimpo…** and paste the link. The Pimpo on your computer then stops, so the same Telegram bot and the same routines never run twice; its data stays where it was. **Usar o Pimpo deste computador** in the same menu brings it back. While connected elsewhere, notifications come from that Pimpo's channels (Telegram and others), not from this computer.

## Audio

A routine or task can read a text aloud and send it to you as audio (`audio.send`): a podcast of the day's news, a briefing to hear on the way. The voice is your Mac's own, in the language you choose (Portuguese, English, Spanish, French and the other languages macOS has), so the text never leaves the computer; for better voices, add Premium or Enhanced ones in System Settings › Accessibility › Spoken Content and Pimpo picks them. With a voice downloaded in **Ajustes › Modelos › Baixar modelos** (Kokoro, natural voices in Portuguese, English, Spanish, French and Italian, or a Piper voice per language), Pimpo reads with it instead, still on this computer; **Ouvir** plays a sample in each language. **Ajustes › Modelos › Voz dos áudios** sets the default voice for routines and, apart, for **Ouvir** in the chat: automatic (a downloaded voice, else the system's), a downloaded voice, the system voice, the browser's own (chat only, instant), OpenAI (`tts-1` or `tts-1-hd` with your OpenAI key, billed per character and counted in the daily limit) or ElevenLabs (with your ElevenLabs key and plan credits). The recording arrives on Telegram with a player; other channels get a note, and **Precisa de você › Áudios recentes** keeps the last ones.

## Webhooks

Any routine can be started by another service calling a secret address: on its page, turn on **Disparar por webhook**. Use it from an iPhone Shortcut ("when I leave home, send me the day's brief"), IFTTT, Zapier, GitHub or a form. What is sent (JSON, form fields or text) reaches the routine as `event.webhook` with `method`, `query` and `body`, so a routine can act on it ("when a new order arrives, tell me who bought what"). Ask for such a routine in a chat and Pimpo makes it start by webhook. Addresses work on this computer and on the home network; for internet services, turn on Tailscale with Funnel. Anyone with the address starts the routine, so treat it like a password; **Trocar endereço** replaces it at once. A paused routine does not start, a call carries at most 256 KB, and a routine starts at most 30 times a minute this way.

## Google Sheets

With Google connected, tasks and routines can read a range of a spreadsheet and add rows to it (`sheets.read`, `sheets.append`): log expenses, habits or orders, or read a list to act on. Name the spreadsheet by its address. Turn on the Google Sheets API in your Google Cloud project; if you connected Google before this, sign in again to allow spreadsheets. Added rows can be undone in **Atividade**.

## Spotify

**Conexões › Spotify** lets tasks and routines see what is playing, play a song, playlist, album, artist or podcast by name, pause and set the volume, on any of your Spotify devices ("toca meu podcast de notícias na caixa da sala às 8h", "pausa a música quando começar uma reunião"). Create an app at developer.spotify.com, add the address Pimpo shows under Redirect URIs, and paste its Client ID; no secret is needed. Controlling playback needs Spotify Premium.

## Apple Reminders, Notes and Calendar

On a Mac, **Conexões › Lembretes, Notas e Calendário da Apple** lets tasks and routines use Apple's own apps: add a reminder that rings on your iPhone and Watch, mark one done, list the open ones; search your notes and add to a note in Pimpo's folder; read the Mac's Calendar. Give the Notes folder Pimpo writes in (created if missing) and, if you like, the default Reminders list. The first time, macOS asks to let Pimpo use each app (System Settings › Privacy & Security › Automation). Reminders added and notes changed can be undone in **Atividade**. When this is connected, "me lembra amanhã às 9h" goes to the Reminders app.

## Routines that ask you

A routine can ask you something and act on your answer: "toda noite me pergunta se treinei e conta os treinos da semana", "pergunta antes de arquivar". The question arrives with its options as buttons on Telegram, numbered on the other channels, and in **Precisa de você**; your answer runs the routine again, which records it or does what you chose. A new question replaces the same one still unanswered, and a question expires after 24 hours.

## Reminders

Ask in any chat, in the app or on a channel: "daqui a 30 minutos me lembra de conferir o deploy", "amanhã às 9h me lembra de ligar para a Ana". Pimpo sets a reminder that goes out once, where your notices go, and is gone; nothing is turned into a routine. Pending reminders are listed at the top of **Rotinas**, where each can be cancelled. One that falls due while Pimpo is closed goes out when it opens, saying it is late. What repeats ("toda segunda…") is a routine instead.

## Other chat channels

Messages you send on Telegram, WhatsApp, Discord, Slack or Signal continue one conversation, so a follow-up like "and tomorrow?" knows what came before. Each conversation also appears under Conversas in the app, where you can pick it up. Send /new (or /novo) to start over; after three quiet hours a new conversation starts on its own. While a task runs, Telegram, Discord and Signal show Pimpo typing (Slack and WhatsApp do not offer that to bots in direct messages).

Besides Telegram and WhatsApp, you can talk to Pimpo in private messages on **Discord**, **Slack** or **Signal**. Set one up in **Conexões** (each card says what to create and which token to paste), then send it `pimpo` followed by the pairing code shown in Conexões. Only you are answered; strangers get nothing. These services have no buttons, so choices arrive numbered: answer `1`, `2`… Signal goes through a signal-cli daemon on your computer, so messages stay end-to-end encrypted up to it. iMessage and SMS are not supported: iMessage needs full disk access to read Messages, and SMS needs a paid service.

If a channel keeps failing for three minutes (Telegram, Discord, Slack or Signal), Pimpo tells you on the others, with a computer notification and in **Precisa de você**, and again when it comes back. **Sistema** shows the failing channel in red with the error. WhatsApp is left out: a failed WhatsApp message is usually about that message (the 24-hour window, a blocked number), not an outage.

## More connectors

**Web pages:** a task or routine can read any web page (`web.read`): its title, its text without menus' scripts and styles, the structured data shops publish (the product's price, its availability) and its links. Like any web read, the first visit to a site asks you once. Some big shops (Amazon, Mercado Livre, Zoom) refuse automated reads; KaBuM and most smaller sites answer.

**Busca na web** in Connections lets Pimpo search the internet, with a Brave Search API key (free for 2,000 searches a month), a Perplexity API key (about US$5 per thousand searches, billed by Perplexity) or the address of a SearXNG instance you trust. DuckDuckGo has no official API for web results; a SearXNG instance can include it among its sources.

**Conexões › Explorar** searches the official MCP registry: hundreds of servers for files, GitHub, databases, notes, maps and more. Choose one, fill in what it asks for, and **Ver as ferramentas** shows what it offers. Check which tools Pimpo may use and how risky each one is (irreversible ones always ask you first), then install. **Adicionar manualmente** takes a command or an https address you already have. See [CONNECTORS.md](CONNECTORS.md) to write your own.

**A service with a REST API** needs no program and no recompiling: describe its requests in a `connector.json` (the address, the key it needs, and for each capability the method, path and what to keep from the answer) and send it in **Conexões › Instalar conector (.json · .zip)**. Pimpo checks it, runs its contract and asks for the key. [CONNECTORS.md](CONNECTORS.md#json-connectors) explains the format; `examples/connectors/hnsearch` is a complete one. If the service publishes an OpenAPI (Swagger) description, **Conexões › Por OpenAPI** writes the file for you: give its address, choose the operations and how risky each one is, fill in the key and install.

## Gallery

Browse ready-made routines, filtered by what they can touch. Pimpo checks the author's signature, runs the routine's tests, and audits what it really calls before installing. To share one of yours, use **Publicar** on its page.

## Skills

Skills written for OpenClaw, Hermes or agentskills.io (a folder with a `SKILL.md`) work in Pimpo as assistants. In **Assistentes › Skills**, paste a GitHub link to the skill's folder or send its `.zip`. Pimpo shows what the skill is, its full text, and which capabilities its text seems to need, all unchecked except those; **you choose what it may use**, and a skill granted nothing can only give advice. Then pick it like any assistant in a chat.

A skill is text someone else wrote, so Pimpo never treats it as your words: every action still passes your rules and approvals, the scripts it carries are not run, a skill the protection list reports is refused, and a skill changed on disk stops until you install it again.

## Suggestions

Once a day Pimpo may notice something you repeat and offer a routine: "you pay this bill every month; want a routine for it?". The suggestion arrives in **Precisa de você** and on your channel, and says what Pimpo would do. **Sim, aprenda** starts it the usual way (you watch it once and approve it); **Não, obrigado** makes sure it does not come back. For this Pimpo looks only at who writes to you and about what, your upcoming events and your routines, never at what emails say. Turn it off in **Ajustes › Notificações › Sugestões**.

## Updates

The desktop app updates itself: when a new version is ready it says so at the top of the app and in the menu bar, and **Atualizar e reiniciar** installs it. Before a new version touches anything, Pimpo keeps a copy of your data. **Ajustes › Geral › Atualizações** looks for updates now, turns on beta versions, and goes back to the version you had before, with your data as it was then. On a server, run `pimpo update` (and `pimpo update --rollback` to go back).

## Moving and backups

- **Ajustes › Backup automático na nuvem** keeps a copy of everything in your own storage, every day or every week, and keeps the last copies you choose:
  - **Amazon S3 or compatible** (Cloudflare R2, Backblaze B2, MinIO, Wasabi): give the bucket, the region (`auto` on R2), the service address when it is not Amazon, and an access key that can read, write, list and delete in that bucket.
  - **Google Drive**: connect Google in **Conexões** (with the Google Drive API turned on in your Google Cloud project) and allow Drive. Backups go to a "Pimpo backups" folder; Pimpo only sees files it created.
  - Each backup is encrypted on your computer with the passphrase you choose, database and memory included, so the storage service cannot read it. Keep the passphrase somewhere safe: without it no one can open the backups.
  - **Ver backups › Restaurar** brings one back; it takes effect when Pimpo restarts, and what was there is kept aside. If a backup fails, you get a message.

- **Ajustes › Exportar e importar tudo** creates one file with everything, the keys sealed with a passphrase you choose. Import it on another computer. What was there before is kept aside.
- `pimpo export file.pimpo` and `pimpo import file.pimpo` do the same from the terminal.
- `pimpo snapshots` and `pimpo restore` go back to an automatic daily snapshot.
- `pimpo routines import FOLDER` installs routines from a folder in the repository's layout (`routines/<id>/`), after the same checks as the repository: manifest, their own tests and an audit. They arrive paused; review their settings and resume each one. `--active` installs them running.
- Coming from OpenClaw or Hermes? Use **Ajustes › Trazer do OpenClaw ou do Hermes**, or `pimpo migrate openclaw`.

**Local copies** (Ajustes › Backup): Pimpo copies everything on this computer every day, before each update and before an import, and keeps the last ten. Pick one and choose **Voltar a esta**; it takes effect when Pimpo is closed and reopened, and the current state is copied first, so it can be undone. After an update Pimpo tells you once that the copy from before is there. Going back to a copy restores data, not the previous version of the app.

## Check-up

In **Ocupação do sistema** (the Pimpo menu, or ⌘⇧D), **Verificar tudo** tests every part for real, right now: each chat channel answers, email and calendar can be read, every model a job may use answers one word (API models, up to a cent each; Claude Code is only looked for, so your usage limit is not spent), each service passes its own check, local copies and cloud backups are recent, and there is room on the disk. Problems come first, each with what to do and a link to where to do it.

## Help, notifications and labs

**Ajuda** (at the bottom of the menu) answers questions about Pimpo itself: it starts a chat, and the agent reads this guide to answer. In **Ajustes › Notificações** choose what reaches you outside the app: task results, failed routines and backup problems can be silenced (they stay in **Precisa de você**); approval requests always arrive. **Ajustes › Laboratório** turns off newer features: organizing memory every night, searching memory by meaning, and exploring the MCP registry.

## Models

**Ajustes › Modelos** (also reached from **Conexões › Inteligência**) says which model does each job: doing tasks and chatting, writing routines, and answering the routines' yes-or-no questions and short texts.

- **On this computer:** Pimpo finds Claude Code (your Claude subscription), Codex from the ChatGPT app (your ChatGPT subscription), and a running Ollama or LM Studio with their models, which are free and private; an Ollama with no models says how to get one. Codex runs with its own shell, browser, computer control, apps and image viewer turned off and your Codex settings ignored, so it only reaches the world through Pimpo's tools; this was checked against the real CLI with a file, an image and a website it could not reach. **opencode** lends Pimpo the providers you signed in to there (GitHub Copilot, OpenCode Go, OpenRouter, DeepSeek…): choose models under **opencode › Escolher**. Every opencode tool (shell, files, web, sub-agents, skills) and any MCP server of your own opencode config is denied, only Pimpo's tools are allowed, and each run's session is deleted afterwards; checked against the real CLI with a file, a shell command and a website it could not reach. Copilot, OpenCode Go and OAuth sign-ins count as subscription; OpenCode's free models only run inside OpenCode itself, so they are not listed. Chat apps (ChatGPT, Qwen, Claude) don't let other programs use their models, and Qwen Code goes through DashScope: connect the Qwen (DashScope) provider with the same key so its cost is counted.
- **Providers:** Anthropic, OpenAI, Google Gemini, OpenRouter, Qwen (Alibaba DashScope), DeepSeek, Groq, Mistral, xAI, or any server that speaks OpenAI's chat API. Paste the provider's key and its models appear with their prices, taken from OpenRouter's public catalog of list prices. A model the catalog does not know needs its price typed in, because the spending limit counts every call and Pimpo never guesses a price.
- **Test and use:** each model answers one real word (capped at one cent) before it is added, with the time it took or a plain reason when it fails (key refused, out of credits, usage limit, model not available, service down).
- **Fallbacks:** each job can have up to four models that take over, in order, when its model fails for a provider reason (for example Claude Code's usage limit). You hear once when a job falls back and once when its model answers again. Your own spending limit is never worked around this way.

**Choosing the model in a chat.** The pill under the message box picks the model for that conversation: **Automático** (the default) or any model above. Under each answer you see which model replied and why, for example "Claude Code · haiku · automático: pedido simples".

- **Automatic choice** (Ajustes › Modelos › Escolha automática nas conversas): each request is weighed before it is answered. Quick ones (a fact, one calendar item, a short reply) go to a light model, heavy ones (many steps across services, a long or delicate text) to a strong one, and everything else stays on the tasks model. Jev weighs the request when it is set up, and only a clear verdict (60% or more) moves it; otherwise a few simple rules do, and any doubt stays on the tasks model. With Claude Code the light model is haiku and the strong one opus; with provider models, the cheapest and the dearest of your list. You can pick them yourself, or turn the automatic choice off. After 80% of the daily spending limit the strong model is no longer used.
- **Thinking level:** the same pill has **Raciocínio**: Automático, low, medium, high or max. Automatic follows the same weighing: low on quick requests, high on heavy ones (not after 80% of the daily limit), and the job's level otherwise. **Ajustes › Modelos** sets each job's level next to its model; leave it on the default to use the model's own. Claude Code and the Anthropic API take all four levels; Codex calls max "xhigh"; OpenAI, OpenRouter, Gemini, Groq, xAI and local servers stop at high; DeepSeek, Mistral and Qwen choose thinking by model instead. A model that does not accept a level answers without it.
- **Channels:** on Telegram, WhatsApp and the others, send `/modelo` to see the model in use, `/modelo opus` (or any model of your list) to fix one, and `/modelo auto` to go back to the automatic choice. `/pensar` (or `/think`) does the same for the thinking level: `/pensar alto`, `/pensar auto`.
- **Routines** run without a model, except for their yes-or-no questions and short texts. A routine that has them shows **Modelo dos julgamentos e textos** in its settings; leave it on the default (the model for judgments) or pick another for that routine alone, and its thinking level beside it.

Rules and approvals do not change with the model: every tool call still goes through Pimpo. With Anthropic, the instructions, tool list and conversation so far are kept in the provider's prompt cache between turns, so a long task pays about a tenth for what it already sent; the cost shown includes the cache's own prices.

## Downloading models

**Ajustes › Modelos › Baixar modelos** downloads models that run on this computer and shows each download as it happens, with its size, progress and a cancel button:

- **Voices** for reading aloud (the voice engine comes with the first one). Every file comes from a pinned address and is checked against its SHA-256 before it is unpacked, and unpacks only inside its own folder (`~/.pimpo/local`). They are not included in backups; download them again on a new computer.
- **Speech to text** (optional): a Whisper model (base, small or turbo) understands your Telegram voice notes and dictation in the app right here; without one, Pimpo uses whisper.cpp if it is installed. Voice notes in OGG need ffmpeg (`brew install ffmpeg`).
- **Language models** through Ollama (which must be installed and open), with suggestions that fit this computer's memory, or any model name. Once downloaded, choose it under **Neste computador › Ollama**.

From the terminal, `pimpo local list` shows the catalog and what is installed, and `pimpo local install ID` downloads with the same checks. The list comes from a JSON catalog built into Pimpo (`internal/local/catalog.json`). A `catalog.json` of your own in `~/.pimpo/local` replaces it; every entry needs an https address, its size and its SHA-256.

## Costs

**Custo** shows what you spent today, this month and on what. Claude Code signed in with a Claude plan and Codex signed in with ChatGPT are paid by your subscription: they spend no money and do not count toward the daily limit. **Pela assinatura** shows what the same work would have cost on the API, for reference. The daily limit (Ajustes) is checked before every model call; routines barely spend anything.
