# Zodim user guide

## First steps

1. **Install.** Use the desktop app, or run `curl -fsSL https://raw.githubusercontent.com/denerFernandes/zodim/main/scripts/install.sh | sh` and then `zodim serve`. Open the link it prints.
2. **Pick your safety level.** The welcome screen offers conservative, balanced (recommended) and liberal.
3. **Connect what you need** in Conexões: Telegram or WhatsApp to talk from your phone, then email and calendar. "Entrar com Google" connects both at once with your own Google client.
4. **Ask for something you do every week.** Use "Nova tarefa", or send a message on Telegram.

Want to see it first? Run `zodim serve --demo`: a sample mailbox and calendar, no accounts, no costs.

## Chat

**Conversar** is a conversation with Zodim in the app, with every past conversation on the side. Each message remembers the ones before it. Zodim reads for real (your calendar, your email), but any change it would make is only rehearsed and listed under the answer, with its risk. **Confirmar e fazer** does exactly those changes, once, each still under your rules and approvals; **Transformar em rotina** makes the task repeat on its own.

## How a task becomes a routine

1. **Exploration.** The agent does the task once. Anything that would change something (archive, send, delete) is only simulated, and you see exactly what would happen.
2. **Routine.** If you like the result, tap "Transformar em rotina". Zodim writes code with tests and checks it against what the agent did. You can read the code on the routine's page.
3. **On its own.** The routine runs on schedule without a model. If a service changes and the routine breaks, you get a notice with "Refazer", and the agent fixes it.

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

## More connectors

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

## Costs

**Custo** shows what you spent today, this month and on what. The daily limit (Ajustes) is checked before every model call; routines barely spend anything.
