# Vigia user guide

## First steps

1. **Install.** Use the desktop app, or run `curl -fsSL https://raw.githubusercontent.com/denerFernandes/vigia/main/scripts/install.sh | sh` and then `vigia serve`. Open the link it prints.
2. **Pick your safety level.** The welcome screen offers conservative, balanced (recommended) and liberal.
3. **Connect what you need** in Conexões: Telegram or WhatsApp to talk from your phone, then email and calendar. "Entrar com Google" connects both at once with your own Google client.
4. **Ask for something you do every week.** Use "Nova tarefa", or send a message on Telegram.

Want to see it first? Run `vigia serve --demo`: a sample mailbox and calendar, no accounts, no costs.

## How a task becomes a routine

1. **Exploration.** The agent does the task once. Anything that would change something (archive, send, delete) is only simulated, and you see exactly what would happen.
2. **Routine.** If you like the result, tap "Transformar em rotina". Vigia writes code with tests and checks it against what the agent did. You can read the code on the routine's page.
3. **On its own.** The routine runs on schedule without a model. If a service changes and the routine breaks, you get a notice with "Refazer", and the agent fixes it.

## Changing a routine without code

Every routine page starts with **Ajustes da rotina**:

- **Quando**: every day, weekdays, some days of the week, once a month, every few hours, or a cron expression under "Avançado".
- **The routine's own choices**: a city (search by name), a limit, a list of words, a currency, whatever the routine was made with. Vigia's compiler turns anything personal in your request into one of these, instead of writing it into the code.
- **Onde avisar**: one or more destinations. These can be your Telegram, other Telegram bots (a family group, for example), WhatsApp, Slack, Discord or email. With none chosen, Vigia uses your usual channel.

To add more Telegram bots, go to **Conexões › Bots extras do Telegram**. Create the bot with @BotFather, paste its token, send it a message (or add it to a group), and tap **Detectar chat**.

## Approvals and rules

- **Precisa de você** lists what is waiting for you: approvals, finished explorations, problems. On the phone it is the first tab.
- Approval buttons: **Permitir** (this time), **Todos desta vez** (the rest of this run), **Sempre** (this routine, from now on), **Negar**.
- **Regras**: write rules in plain words ("nunca apague e-mails do meu chefe"). You see exactly what the rule will enforce, and a test against last week, before saving.
- Some things always ask, whatever the rules say: WhatsApp to other people, locks and alarms.

## Receipts and undo

Every action has a receipt: what was done, with which arguments, under which rule. Reversible actions can be undone from the receipt. Deletes go to the trash, and sent emails wait 10 minutes before leaving.

## Memory

Vigia remembers what you tell it. Facts it read somewhere are marked "não confirmado" and never guide it until you confirm them. Every change is versioned: **Histórico › Voltar para aqui** undoes any change.

## People

In **Pessoas**, invite family members as a member or a guest. They send the invite code to the bot and get their own memory and accounts. Choose who approves each person's requests: a guest's changes always wait for that person. Only the owner makes lasting rules.

## On the phone

1. In **Ajustes › Abrir no celular**, expose Vigia safely (for example `tailscale serve 7788`), paste the address and name the phone.
2. Scan the QR code with the Vigia app, or paste the link.
3. Lost the phone? Tap the trash icon next to it. Only that phone loses access.

## Gallery

Browse ready-made routines, filtered by what they can touch. Vigia checks the author's signature, runs the routine's tests, and audits what it really calls before installing. To share one of yours, use **Publicar** on its page.

## Moving and backups

- **Ajustes › Exportar e importar tudo** creates one file with everything, the keys sealed with a passphrase you choose. Import it on another computer. What was there before is kept aside.
- `vigia export file.vigia` and `vigia import file.vigia` do the same from the terminal.
- `vigia snapshots` and `vigia restore` go back to an automatic daily snapshot.
- Coming from OpenClaw or Hermes? Use **Ajustes › Trazer do OpenClaw ou do Hermes**, or `vigia migrate openclaw`.

## Costs

**Custo** shows what you spent today, this month and on what. The daily limit (Ajustes) is checked before every model call; routines barely spend anything.
