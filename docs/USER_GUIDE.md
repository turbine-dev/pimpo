# Zodim user guide

## Getting around

**Início** is where the day starts: ask for anything, and see what needs you, what ran and what is next today, what you spent, and your recent chats. The side menu keeps what you use daily on top (Início, Rotinas, Atividade, Assistentes) and the rest under **Mais**; your conversations are listed below it. At the bottom, the bell opens what waits for you (approvals, stopped routines, tasks ready, parts with errors) so you can answer in place, and the Zodim menu has Ajustes, cost, **Ocupação do sistema** (⌘⇧D: how busy the computer and Zodim are, what is running now and the state of every channel, account and service), pairing a phone, the theme and help.

## First steps

1. **Install.** Use the desktop app, or run `curl -fsSL https://raw.githubusercontent.com/denerFernandes/zodim/main/scripts/install.sh | sh` and then `zodim serve`. Open the link it prints.
2. **Pick your safety level.** The welcome screen offers conservative, balanced (recommended) and liberal.
3. **Connect what you need** in Conexões: Telegram or WhatsApp to talk from your phone, then email and calendar. "Entrar com Google" connects both at once with your own Google client.
4. **Ask for something you do every week.** Use "Nova tarefa", or send a message on Telegram.

Want to see it first? Run `zodim serve --demo`: a sample mailbox and calendar, no accounts, no costs.

## Language

Zodim speaks Português, English, Español, Français, Deutsch, Italiano, 日本語, 简体中文, 한국어 and Русский. Choose in **Ajustes › Geral**: the app, the messages on Telegram and the other channels, approval requests, connector instructions and the dates routines write all follow it. Tasks are answered in the language you ask in. The phone's pairing screen follows the phone's language. This guide is in English only.

## Chat

**Conversar** is a conversation with Zodim in the app, with every past conversation on the side. Each message remembers the ones before it. Zodim reads for real (your calendar, your email), but any change it would make is only rehearsed and listed under the answer, with its risk. **Confirmar e fazer** does exactly those changes, once, each still under your rules and approvals; **Transformar em rotina** makes the task repeat on its own.

**Assistentes** (from the chat's side panel) are roles for the agent: a name, what its job is, and the tools it may use. Pick one when starting a conversation. Tap the microphone to speak instead of typing: the computer's own dictation is used when there is one, otherwise the audio is transcribed on this machine with whisper.cpp; a question you spoke is answered aloud, and **Ouvir** reads any answer. The limit is enforced by Zodim itself: the assistant only sees its tools, and any other call is blocked, whatever the text says. A routine made from that conversation uses only those tools too.

## How a task becomes a routine

1. **Exploration.** The agent does the task once. Anything that would change something (archive, send, delete) is only simulated, and you see exactly what would happen.
2. **Routine.** If you like the result, tap "Transformar em rotina". Zodim writes code with tests and checks it against what the agent did. You can read the code on the routine's page.
3. **On its own.** The routine runs on schedule without a model. If a service changes and the routine breaks, you get a notice with "Refazer", and the agent fixes it.

## Routines that react to something new

Ask for a reaction instead of a time ("me avise quando chegar e-mail da Ana", "quando esse site publicar algo novo") and the routine watches instead of running on a clock. Zodim checks the source every few minutes without any model, so the checks cost nothing, and runs the routine only with what it has not seen before; the first check only learns what is already there. Change how often it checks in the routine's settings. The empty checks stay out of **Atividade**; what was found and what the routine did show up as usual. The gallery's **E-mail de alguém importante** is one.

A routine can also write a little: "e me diga em uma frase o que ela pede", "sugira uma resposta". The part that must be composed is written by a small model (the one set for judgments) for each item, about a fraction of a cent each; everything that can be copied (sender, subject, date) stays plain code. Each text is checked against the daily limit first, shows up in **Atividade**, and treats the email as data, never as orders. What the routine then does with the text still passes your rules and approvals.

## Changing a routine without code

Every routine page starts with **Ajustes da rotina**:

- **Quando**: every day, weekdays, some days of the week, once a month, every few hours, or a cron expression under "Avançado".
- **The routine's own choices**: a city (search by name), a limit, a list of words, a currency, whatever the routine was made with. Zodim's compiler turns anything personal in your request into one of these, instead of writing it into the code.
- **Onde avisar**: one or more destinations. These can be your Telegram, other Telegram bots (a family group, for example), WhatsApp, Slack, Discord or email. With none chosen, Zodim uses your usual channel.

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

Zodim remembers what you tell it. Facts it read somewhere are marked "não confirmado" and never guide it until you confirm them. Every change is versioned: **Histórico › Voltar para aqui** undoes any change.

Search memory in plain words ("what can't I eat?"): with Jev set up, Zodim finds facts by meaning, not only by the words they share, and the agent uses the same search. Every night Zodim merges facts that say the same thing, never trading one you confirmed for one it read somewhere, and never merging facts that differ in a date, place or name. **Organizar** does it now, and **Histórico** undoes it.

## People

In **Pessoas**, invite family members as a member or a guest. They send the invite code to the bot and get their own memory and accounts. Choose who approves each person's requests: a guest's changes always wait for that person. Only the owner makes lasting rules.

## On the phone

1. In **Ajustes › Abrir no celular**, turn on one or both ways in:
   - **Em casa**: the phone reaches Zodim over your Wi-Fi. No account, nothing to install, and it stops working when you leave home.
   - **De qualquer lugar**: Tailscale runs inside Zodim. The first time, sign in to Tailscale (free) in the browser; Zodim then gets an `https://zodim.<your-network>.ts.net` link that works from anywhere. If Tailscale says Funnel or HTTPS is off, turn them on in its admin console as the message explains.
   - Already expose Zodim another way? Paste the address under **Usar outro endereço**.
2. Name the phone and tap **Gerar código**. With both ways on, the phone uses the home address when it can and the other one elsewhere.
3. Scan the QR code with the Zodim app, or paste the link.
4. Lost the phone? Tap the trash icon next to it. Only that phone loses access.

## A Zodim on another computer or server

Zodim can run on a machine that is always on (a home server, a VPS) while the desktop app just opens it. On that machine, generate a link in **Ajustes › Abrir no celular** as for a phone. On your computer, click the Zodim icon in the menu bar, choose **Conectar a outro Zodim…** and paste the link. The Zodim on your computer then stops, so the same Telegram bot and the same routines never run twice; its data stays where it was. **Usar o Zodim deste computador** in the same menu brings it back. While connected elsewhere, notifications come from that Zodim's channels (Telegram and others), not from this computer.

## Other chat channels

Besides Telegram and WhatsApp, you can talk to Zodim in private messages on **Discord**, **Slack** or **Signal**. Set one up in **Conexões** (each card says what to create and which token to paste), then send it `zodim` followed by the pairing code shown in Conexões. Only you are answered; strangers get nothing. These services have no buttons, so choices arrive numbered: answer `1`, `2`… Signal goes through a signal-cli daemon on your computer, so messages stay end-to-end encrypted up to it. iMessage and SMS are not supported: iMessage needs full disk access to read Messages, and SMS needs a paid service.

If a channel keeps failing for three minutes (Telegram, Discord, Slack or Signal), Zodim tells you on the others, with a computer notification and in **Precisa de você**, and again when it comes back. **Sistema** shows the failing channel in red with the error. WhatsApp is left out: a failed WhatsApp message is usually about that message (the 24-hour window, a blocked number), not an outage.

## More connectors

**Busca na web** in Connections lets Zodim search the internet, with a Brave Search API key (free for 2,000 searches a month) or the address of a SearXNG instance you trust. DuckDuckGo has no official API for web results; a SearXNG instance can include it among its sources.

**Conexões › Explorar** searches the official MCP registry: hundreds of servers for files, GitHub, databases, notes, maps and more. Choose one, fill in what it asks for, and **Ver as ferramentas** shows what it offers. Check which tools Zodim may use and how risky each one is (irreversible ones always ask you first), then install. **Adicionar manualmente** takes a command or an https address you already have. See [CONNECTORS.md](CONNECTORS.md) to write your own.

## Gallery

Browse ready-made routines, filtered by what they can touch. Zodim checks the author's signature, runs the routine's tests, and audits what it really calls before installing. To share one of yours, use **Publicar** on its page.

## Moving and backups

- **Ajustes › Backup automático na nuvem** keeps a copy of everything in your own storage, every day or every week, and keeps the last copies you choose:
  - **Amazon S3 or compatible** (Cloudflare R2, Backblaze B2, MinIO, Wasabi): give the bucket, the region (`auto` on R2), the service address when it is not Amazon, and an access key that can read, write, list and delete in that bucket.
  - **Google Drive**: connect Google in **Conexões** (with the Google Drive API turned on in your Google Cloud project) and allow Drive. Backups go to a "Zodim backups" folder; Zodim only sees files it created.
  - Each backup is encrypted on your computer with the passphrase you choose, database and memory included, so the storage service cannot read it. Keep the passphrase somewhere safe: without it no one can open the backups.
  - **Ver backups › Restaurar** brings one back; it takes effect when Zodim restarts, and what was there is kept aside. If a backup fails, you get a message.

- **Ajustes › Exportar e importar tudo** creates one file with everything, the keys sealed with a passphrase you choose. Import it on another computer. What was there before is kept aside.
- `zodim export file.zodim` and `zodim import file.zodim` do the same from the terminal.
- `zodim snapshots` and `zodim restore` go back to an automatic daily snapshot.
- Coming from OpenClaw or Hermes? Use **Ajustes › Trazer do OpenClaw ou do Hermes**, or `zodim migrate openclaw`.

**Local copies** (Ajustes › Backup): Zodim copies everything on this computer every day, before each update and before an import, and keeps the last ten. Pick one and choose **Voltar a esta**; it takes effect when Zodim is closed and reopened, and the current state is copied first, so it can be undone. After an update Zodim tells you once that the copy from before is there. Going back to a copy restores data, not the previous version of the app.

## Help, notifications and labs

**Ajuda** (at the bottom of the menu) answers questions about Zodim itself: it starts a chat, and the agent reads this guide to answer. In **Ajustes › Notificações** choose what reaches you outside the app: task results, failed routines and backup problems can be silenced (they stay in **Precisa de você**); approval requests always arrive. **Ajustes › Laboratório** turns off newer features: organizing memory every night, searching memory by meaning, and exploring the MCP registry.

## Models

By default Zodim uses Claude Code with your own subscription. In **Ajustes › Modelos** you can pick another model for each job (tasks and chat, writing routines, judgments): Anthropic, OpenAI or OpenRouter with your API key, or a local model through Ollama. An API model needs its price per million tokens before it runs, because the daily spending limit counts every call with it; **Testar** checks the key with a call capped at one cent. Rules and approvals do not change with the model: every tool call still goes through Zodim. With Anthropic, the instructions, tool list and conversation so far are kept in the provider's prompt cache between turns, so a long task pays about a tenth for what it already sent; the cost shown includes the cache's own prices.

## Costs

**Custo** shows what you spent today, this month and on what. The daily limit (Ajustes) is checked before every model call; routines barely spend anything.
