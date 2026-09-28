# Writes the traces for the routines OpenClaw and Hermes users run most
# (see README.md). Run: python3 testdata/openclaw/make.py
import json, os, shutil

here = os.path.dirname(os.path.abspath(__file__))
T = []

def mail(id, frm, name, subject, snippet, date, labels=("INBOX", "UNREAD"), replied=False):
    return {"id": id, "from": frm, "from_name": name, "to": "eu@exemplo.com", "subject": subject, "snippet": snippet,
            "date": date, "labels": list(labels), "replied": replied, "unread": "UNREAD" in labels}

def ev(id, title, start, end, location="", attendees=(), cal="Trabalho"):
    return {"id": id, "title": title, "start": start, "end": end, "location": location, "attendees": list(attendees), "calendar": cal}

def sent(cap, text):
    return {"capability": cap, "args": {"text": text}, "result": {"ok": True}}

def expect(cap, count=None, contains=(), not_contains=()):
    e = {"capability": cap}
    if count is not None:
        e["count"] = count
    if contains:
        e["contains"] = list(contains)
    if not_contains:
        e["not_contains"] = list(not_contains)
    return e

def item(title, link, published, summary=""):
    return {"title": title, "link": link, "published": published, "summary": summary}

# 1. Morning briefing: weather, calendar, important mail, tasks.
T.append({
    "id": "oc-01-morning-brief",
    "request": "Todo dia útil às 7h me manda no Telegram um resumo do dia: previsão do tempo em São Paulo, minha agenda de hoje, os e-mails não lidos que forem importantes de verdade e minhas tarefas do Todoist para hoje ou atrasadas.",
    "now": "2026-09-24T07:00:00-03:00",
    "calls": [
        {"capability": "http.getJSON", "args": "https://api.open-meteo.com/v1/forecast?latitude=-23.55&longitude=-46.63&daily=temperature_2m_max,temperature_2m_min,precipitation_probability_max&timezone=America%2FSao_Paulo&forecast_days=1",
         "result": {"daily": {"time": ["2026-09-24"], "temperature_2m_max": [27.4], "temperature_2m_min": [16.1], "precipitation_probability_max": [70]}}},
        {"capability": "calendar.events", "args": {"from": "2026-09-24T00:00:00-03:00", "to": "2026-09-24T23:59:59-03:00"},
         "result": [ev("e1", "Daily do time", "2026-09-24T09:30:00-03:00", "2026-09-24T09:45:00-03:00", "Google Meet", ["time@acme.com"]),
                    ev("e2", "Almoço com a Júlia", "2026-09-24T12:30:00-03:00", "2026-09-24T13:30:00-03:00", "Rua Oscar Freire, 900", [], "Pessoal")]},
        {"capability": "gmail.search", "args": {"query": "in:inbox is:unread", "days": 1, "unread": True, "max": 20},
         "result": [mail("m1", "cfo@acme.com", "Renata Dias", "Aprovar orçamento de outubro", "Preciso do seu ok até meio-dia.", "2026-09-24T06:40:00-03:00"),
                    mail("m2", "ofertas@lojax.com.br", "Loja X", "Só hoje: 50% em tudo", "Aproveite as ofertas.", "2026-09-24T05:10:00-03:00", ("INBOX", "UNREAD", "CATEGORY_PROMOTIONS")),
                    mail("m3", "noreply@github.com", "GitHub", "[acme/app] CI falhou na main", "O build 1432 falhou em test/unit.", "2026-09-24T03:12:00-03:00")]},
        {"capability": "todoist.tasks", "args": {"filter": "today | overdue"},
         "result": [{"id": "t1", "content": "Renovar seguro do carro", "due": {"date": "2026-09-23"}, "priority": 4, "project_id": "p1", "labels": []},
                    {"id": "t2", "content": "Mandar fotos pro contador", "due": {"date": "2026-09-24"}, "priority": 1, "project_id": "p1", "labels": []}]},
        sent("telegram.send", "☀️ Bom dia! São Paulo: 16° a 27°, 70% de chance de chuva.\n📅 09:30 Daily do time · 12:30 Almoço com a Júlia\n📩 Aprovar orçamento de outubro (Renata Dias) · [acme/app] CI falhou na main\n✅ Renovar seguro do carro (atrasada) · Mandar fotos pro contador"),
    ],
    "judgments": {"important": {"m1": 0.95, "m2": 0.02, "m3": 0.81}},
    "outcome": "Uma mensagem no Telegram com o tempo do dia, os eventos de hoje, só os e-mails importantes e as tarefas de hoje ou atrasadas.",
    "expect": [expect("telegram.send", 1, ["27", "Daily do time", "Almoço com a Júlia", "Aprovar orçamento de outubro", "CI falhou", "Renovar seguro do carro", "Mandar fotos pro contador"], ["50% em tudo"])],
    "holdout": {
        "now": "2026-10-08T07:00:00-03:00",
        "responses": [
            {"capability": "http.getJSON", "result": {"daily": {"time": ["2026-10-08"], "temperature_2m_max": [31.2], "temperature_2m_min": [19.8], "precipitation_probability_max": [10]}}},
            {"capability": "calendar.events", "result": [ev("h1", "Entrevista candidato backend", "2026-10-08T10:00:00-03:00", "2026-10-08T11:00:00-03:00", "Sala 2")]},
            {"capability": "gmail.search", "result": [
                mail("hm1", "news@medium.com", "Medium", "Histórias para você", "Leia agora.", "2026-10-08T05:00:00-03:00", ("INBOX", "UNREAD", "CATEGORY_UPDATES")),
                mail("hm2", "juridico@acme.com", "Jurídico", "Contrato Globex precisa de assinatura", "Assinar hoje.", "2026-10-08T06:30:00-03:00")]},
            {"capability": "todoist.tasks", "result": [{"id": "ht1", "content": "Pagar IPVA", "due": {"date": "2026-10-08"}, "priority": 4, "project_id": "p1", "labels": []}]},
        ],
        "judgments": {"important": {"hm1": 0.03, "hm2": 0.96}},
        "expect": [expect("telegram.send", 1, ["31", "Entrevista candidato backend", "Contrato Globex precisa de assinatura", "Pagar IPVA"], ["Histórias para você", "Daily do time"])],
    },
})

# 2. Urgent email triage.
T.append({
    "id": "oc-02-urgent-mail",
    "request": "Fica de olho no meu e-mail e me avisa no Telegram só quando chegar algo urgente de verdade (algo que precisa de resposta ou ação hoje). O resto pode esperar.",
    "now": "2026-09-24T10:30:00-03:00",
    "calls": [
        {"capability": "gmail.search", "args": {"query": "in:inbox is:unread", "days": 1, "unread": True, "max": 20},
         "result": [mail("u1", "suporte@clienteabc.com", "Cliente ABC", "Sistema fora do ar desde 9h", "Nossas vendas pararam, precisamos de retorno agora.", "2026-09-24T10:12:00-03:00"),
                    mail("u2", "newsletter@thenews.cc", "the news", "O resumo de hoje", "As notícias do dia.", "2026-09-24T09:00:00-03:00", ("INBOX", "UNREAD", "CATEGORY_UPDATES")),
                    mail("u3", "pedro@acme.com", "Pedro Alves", "Ideia para o offsite", "Quando der, dá uma olhada.", "2026-09-24T08:44:00-03:00")]},
        sent("telegram.send", "🚨 Urgente: \"Sistema fora do ar desde 9h\" (Cliente ABC) — Nossas vendas pararam, precisamos de retorno agora."),
    ],
    "judgments": {"urgent": {"u1": 0.97, "u2": 0.01, "u3": 0.08}},
    "outcome": "Um alerta no Telegram para cada e-mail novo que é urgente; nada para os outros.",
    "expect": [expect("telegram.send", 1, ["Sistema fora do ar desde 9h"], ["O resumo de hoje", "Ideia para o offsite"])],
    "holdout": {
        "now": "2026-10-08T15:00:00-03:00",
        "responses": [{"capability": "gmail.search", "result": [
            mail("hu1", "banco@bancoalfa.com.br", "Banco Alfa", "Compra aprovada R$ 42,90", "Compra no cartão final 1234.", "2026-10-08T14:40:00-03:00"),
            mail("hu2", "marina@acme.com", "Marina Souza", "Fotos da festa", "Subi no drive.", "2026-10-08T13:02:00-03:00")]}],
        "judgments": {"urgent": {"hu1": 0.06, "hu2": 0.02}},
        "expect": [expect("telegram.send", 0)],
    },
})

# 3. Yesterday's inbox digest to Slack.
T.append({
    "id": "oc-03-inbox-slack",
    "request": "Dias úteis às 9h, manda no meu Slack um resumo dos e-mails que chegaram ontem: remetente e assunto, sem promoções nem redes sociais.",
    "now": "2026-09-24T09:00:00-03:00",
    "calls": [
        {"capability": "gmail.search", "args": {"query": "in:inbox after:2026/09/23 before:2026/09/24", "days": 2, "max": 50},
         "result": [mail("s1", "rh@acme.com", "RH Acme", "Nova política de home office", "A partir de outubro...", "2026-09-23T11:00:00-03:00", ("INBOX",)),
                    mail("s2", "promo@viagens.com", "Viagens.com", "Pacotes com 40% off", "Ofertas.", "2026-09-23T12:00:00-03:00", ("INBOX", "CATEGORY_PROMOTIONS")),
                    mail("s3", "notification@linkedin.com", "LinkedIn", "5 pessoas viram seu perfil", "Veja quem.", "2026-09-23T15:00:00-03:00", ("INBOX", "CATEGORY_SOCIAL")),
                    mail("s4", "tiago@fornecedor.com", "Tiago Rocha", "Proposta revisada", "Segue a proposta com o desconto.", "2026-09-23T18:20:00-03:00", ("INBOX",))]},
        sent("slack.send", "📬 E-mails de ontem:\n• RH Acme — Nova política de home office\n• Tiago Rocha — Proposta revisada"),
    ],
    "outcome": "Uma mensagem no Slack com remetente e assunto dos e-mails de ontem, sem promoções nem redes sociais.",
    "expect": [expect("slack.send", 1, ["Nova política de home office", "Proposta revisada", "Tiago Rocha"], ["40% off", "viram seu perfil"])],
    "holdout": {
        "now": "2026-10-08T09:00:00-03:00",
        "responses": [{"capability": "gmail.search", "result": [
            mail("hs1", "financeiro@acme.com", "Financeiro", "Fechamento de setembro", "Planilha anexa.", "2026-10-07T10:00:00-03:00", ("INBOX",)),
            mail("hs2", "deals@loja.com", "Loja", "Semana do cliente", "Descontos.", "2026-10-07T11:00:00-03:00", ("INBOX", "CATEGORY_PROMOTIONS")),
            mail("hs3", "velho@acme.com", "Velho", "Assunto de anteontem", "x", "2026-10-06T10:00:00-03:00", ("INBOX",))]}],
        "expect": [expect("slack.send", 1, ["Fechamento de setembro"], ["Semana do cliente", "Assunto de anteontem"])],
    },
})

# 4. Weekly AI news from Hacker News.
T.append({
    "id": "oc-04-ai-news",
    "request": "Toda segunda às 9h, me manda no Telegram as notícias sobre inteligência artificial que mais bombaram no Hacker News, com o link de cada uma.",
    "now": "2026-09-28T09:00:00-03:00",
    "calls": [
        {"capability": "rss.read", "args": {"url": "https://hnrss.org/best", "max": 30},
         "result": [item("Open-weight model beats GPT on coding benchmark", "https://news.ycombinator.com/item?id=1001", "2026-09-27T14:00:00Z"),
                    item("The history of the Unix pipe", "https://news.ycombinator.com/item?id=1002", "2026-09-27T10:00:00Z"),
                    item("Anthropic publishes interpretability results", "https://news.ycombinator.com/item?id=1003", "2026-09-26T18:00:00Z"),
                    item("Show HN: A tiny SQLite GUI", "https://news.ycombinator.com/item?id=1004", "2026-09-26T09:00:00Z")]},
        sent("telegram.send", "🤖 IA no Hacker News:\n• Open-weight model beats GPT on coding benchmark — https://news.ycombinator.com/item?id=1001\n• Anthropic publishes interpretability results — https://news.ycombinator.com/item?id=1003"),
    ],
    "judgments": {"about_ai": {"id=1001": 0.97, "id=1002": 0.02, "id=1003": 0.95, "id=1004": 0.04}},
    "outcome": "Uma mensagem com os títulos e links das histórias de IA do feed de melhores do Hacker News; as outras ficam de fora.",
    "expect": [expect("telegram.send", 1, ["Open-weight model beats GPT", "item?id=1001", "Anthropic publishes interpretability results"], ["Unix pipe", "SQLite GUI"])],
    "holdout": {
        "now": "2026-10-12T09:00:00-03:00",
        "responses": [{"capability": "rss.read", "result": [
            item("Why we left Kubernetes", "https://news.ycombinator.com/item?id=2001", "2026-10-11T14:00:00Z"),
            item("LLM agents that write their own tools", "https://news.ycombinator.com/item?id=2002", "2026-10-11T12:00:00Z")]}],
        "judgments": {"about_ai": {"id=2001": 0.03, "id=2002": 0.96}},
        "expect": [expect("telegram.send", 1, ["LLM agents that write their own tools", "item?id=2002"], ["Kubernetes"])],
    },
})

# 5. Meeting prep: the existing proof trace.
prep = json.load(open(os.path.join(here, "..", "proof", "07-meeting-prep.json")))
prep["id"] = "oc-05-meeting-prep"
T.append(prep)

# 6. Meetings in the next hour.
T.append({
    "id": "oc-06-next-meetings",
    "request": "A cada hora, das 8h às 18h, me avisa no Telegram das reuniões que começam na próxima hora, com horário, local e participantes. Se não tiver nenhuma, não manda nada.",
    "now": "2026-09-24T13:00:00-03:00",
    "calls": [
        {"capability": "calendar.events", "args": {"from": "2026-09-24T13:00:00-03:00", "to": "2026-09-24T14:00:00-03:00"},
         "result": [ev("n1", "Review do roadmap", "2026-09-24T13:30:00-03:00", "2026-09-24T14:30:00-03:00", "Sala Paulista", ["pm@acme.com", "cto@acme.com"])]},
        sent("telegram.send", "⏰ 13:30 Review do roadmap — Sala Paulista — pm@acme.com, cto@acme.com"),
    ],
    "outcome": "Um aviso para cada reunião que começa na próxima hora; silêncio quando não há nenhuma.",
    "expect": [expect("telegram.send", 1, ["Review do roadmap", "13:30", "Sala Paulista", "cto@acme.com"])],
    "holdout": {
        "now": "2026-10-08T10:00:00-03:00",
        "responses": [{"capability": "calendar.events", "result": [
            ev("hn1", "1:1 com a Ana", "2026-10-08T10:15:00-03:00", "2026-10-08T10:45:00-03:00", "Google Meet", ["ana@acme.com"]),
            ev("hn2", "Planejamento Q4", "2026-10-08T15:00:00-03:00", "2026-10-08T16:00:00-03:00", "Sala 1", ["time@acme.com"])]}],
        "expect": [expect("telegram.send", None, ["1:1 com a Ana", "10:15"], ["Planejamento Q4"])],
    },
})

# 7. GitHub repo watcher.
def issue(n, title, state, updated, pr=False, author="dev1"):
    return {"number": n, "title": title, "state": state, "author": author, "labels": [], "url": f"https://github.com/acme/app/issues/{n}", "updated": updated, "pull_request": pr}

T.append({
    "id": "oc-07-repo-watch",
    "request": "A cada 6 horas, olha o repositório acme/app no GitHub e me avisa no Discord das issues e PRs abertas que tiveram atividade nas últimas 6 horas, com o link.",
    "now": "2026-09-24T18:00:00-03:00",
    "calls": [
        {"capability": "github.issues", "args": {"repo": "acme/app", "state": "open", "max": 50},
         "result": [issue(412, "Login quebra no Safari", "open", "2026-09-24T19:40:00Z"),
                    issue(411, "Adicionar modo escuro", "open", "2026-09-24T16:10:00Z", True, "dev2"),
                    issue(398, "Melhorar docs de instalação", "open", "2026-09-20T12:00:00Z")]},
        sent("discord.send", "🐙 acme/app nas últimas 6h:\n• #412 Login quebra no Safari — https://github.com/acme/app/issues/412\n• PR #411 Adicionar modo escuro — https://github.com/acme/app/issues/411"),
    ],
    "outcome": "Um aviso no Discord com as issues e PRs abertas atualizadas nas últimas 6 horas; nada quando não houver.",
    "expect": [expect("discord.send", 1, ["Login quebra no Safari", "issues/412", "Adicionar modo escuro"], ["Melhorar docs de instalação"])],
    "holdout": {
        "now": "2026-10-08T12:00:00-03:00",
        "responses": [{"capability": "github.issues", "result": [
            issue(430, "Crash ao exportar PDF", "open", "2026-10-08T13:20:00Z"),
            issue(429, "Refatorar módulo de pagamentos", "open", "2026-10-07T09:00:00Z", True)]}],
        "expect": [expect("discord.send", 1, ["Crash ao exportar PDF"], ["Refatorar módulo de pagamentos"])],
    },
})

# 8. Weekly report.
T.append({
    "id": "oc-08-weekly-report",
    "request": "Toda sexta às 17h, me manda no Telegram um resumo da semana: as issues e PRs do acme/app fechados nos últimos 7 dias e as tarefas do Todoist que ficaram atrasadas.",
    "now": "2026-09-25T17:00:00-03:00",
    "calls": [
        {"capability": "github.issues", "args": {"repo": "acme/app", "state": "closed", "max": 50},
         "result": [issue(405, "Corrigir cálculo de frete", "closed", "2026-09-24T15:00:00Z"),
                    issue(401, "Migrar para Postgres 17", "closed", "2026-09-22T11:00:00Z", True),
                    issue(380, "Atualizar logo", "closed", "2026-09-10T09:00:00Z")]},
        {"capability": "todoist.tasks", "args": {"filter": "overdue"},
         "result": [{"id": "w1", "content": "Revisar contrato de aluguel", "due": {"date": "2026-09-22"}, "priority": 3, "project_id": "p1", "labels": []}]},
        sent("telegram.send", "📊 Semana:\nFechados no acme/app: #405 Corrigir cálculo de frete · PR #401 Migrar para Postgres 17\nAtrasadas: Revisar contrato de aluguel"),
    ],
    "outcome": "Uma mensagem com o que foi fechado no repositório nos últimos 7 dias e as tarefas atrasadas.",
    "expect": [expect("telegram.send", 1, ["Corrigir cálculo de frete", "Migrar para Postgres 17", "Revisar contrato de aluguel"], ["Atualizar logo"])],
    "holdout": {
        "now": "2026-10-09T17:00:00-03:00",
        "responses": [
            {"capability": "github.issues", "result": [issue(440, "Paginação na busca", "closed", "2026-10-08T10:00:00Z"), issue(420, "Erro antigo", "closed", "2026-09-28T10:00:00Z")]},
            {"capability": "todoist.tasks", "result": []}],
        "expect": [expect("telegram.send", 1, ["Paginação na busca"], ["Erro antigo", "Revisar contrato de aluguel"])],
    },
})

# 9. Bitcoin moves.
T.append({
    "id": "oc-09-crypto",
    "request": "De hora em hora, olha o preço do Bitcoin em reais e me avisa no Telegram se ele subiu ou caiu mais de 3% nas últimas 24 horas. Se variou menos, fica quieto.",
    "now": "2026-09-24T14:00:00-03:00",
    "calls": [
        {"capability": "http.getJSON", "args": "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=brl&include_24hr_change=true",
         "result": {"bitcoin": {"brl": 612345.0, "brl_24h_change": -4.27}}},
        sent("telegram.send", "📉 Bitcoin caiu 4.27% em 24h: R$ 612.345"),
    ],
    "outcome": "Um alerta com o preço e a variação quando ela passa de 3% para cima ou para baixo; nada caso contrário.",
    "expect": [expect("telegram.send", 1, ["4.27"])],
    "holdout": {
        "now": "2026-10-08T14:00:00-03:00",
        "responses": [{"capability": "http.getJSON", "result": {"bitcoin": {"brl": 598000.0, "brl_24h_change": 1.12}}}],
        "expect": [expect("telegram.send", 0)],
    },
})

# 10. B3 quotes.
def quote(sym, price, change):
    return {"symbol": sym, "regularMarketPrice": price, "regularMarketChangePercent": change}

T.append({
    "id": "oc-10-stocks",
    "request": "Todo dia útil às 10h30, me manda no Telegram a cotação de PETR4, ITUB4 e MXRF11 e destaca com ⚠️ as que variaram mais de 2% no dia.",
    "now": "2026-09-24T10:30:00-03:00",
    "calls": [
        {"capability": "http.getJSON", "args": "https://brapi.dev/api/quote/PETR4,ITUB4,MXRF11",
         "result": {"results": [quote("PETR4", 38.12, -0.8), quote("ITUB4", 35.4, 2.6), quote("MXRF11", 10.21, 0.1)]}},
        sent("telegram.send", "📈 Cotações:\nPETR4 R$ 38,12 (-0,8%)\n⚠️ ITUB4 R$ 35,40 (+2,6%)\nMXRF11 R$ 10,21 (+0,1%)"),
    ],
    "outcome": "Uma mensagem com as três cotações e um ⚠️ nas que variaram mais de 2%.",
    "expect": [expect("telegram.send", 1, ["PETR4", "ITUB4", "MXRF11", "⚠️"])],
    "holdout": {
        "now": "2026-10-08T10:30:00-03:00",
        "responses": [{"capability": "http.getJSON", "result": {"results": [quote("PETR4", 36.0, -3.1), quote("ITUB4", 35.2, 0.4), quote("MXRF11", 10.3, 0.2)]}}],
        "expect": [expect("telegram.send", 1, ["PETR4", "ITUB4", "MXRF11", "⚠️"])],
    },
})

# 11. arXiv to Obsidian.
T.append({
    "id": "oc-11-arxiv",
    "request": "Todo dia às 8h, olha os artigos novos de IA no arXiv e anota no Obsidian, na nota Pesquisa/arXiv.md, título e link dos que forem sobre agentes de IA.",
    "now": "2026-09-24T08:00:00-03:00",
    "calls": [
        {"capability": "rss.read", "args": {"url": "https://rss.arxiv.org/rss/cs.AI", "max": 50},
         "result": [item("Planning with Tool-Using Language Agents", "https://arxiv.org/abs/2609.01001", "2026-09-24T04:00:00Z", "We study agents that call tools..."),
                    item("A Survey of Graph Neural Networks for Chemistry", "https://arxiv.org/abs/2609.01002", "2026-09-24T04:00:00Z", "GNNs for molecules."),
                    item("Memory for Long-Horizon Web Agents", "https://arxiv.org/abs/2609.01003", "2026-09-24T04:00:00Z", "Agents that browse...")]},
        {"capability": "obsidian.append", "args": {"note": "Pesquisa/arXiv.md", "text": "## 2026-09-24\n- [Planning with Tool-Using Language Agents](https://arxiv.org/abs/2609.01001)\n- [Memory for Long-Horizon Web Agents](https://arxiv.org/abs/2609.01003)"}, "result": {"ok": True}},
    ],
    "judgments": {"about_agents": {"2609.01001": 0.96, "2609.01002": 0.03, "2609.01003": 0.93}},
    "outcome": "Uma entrada na nota Pesquisa/arXiv.md com título e link dos artigos sobre agentes.",
    "expect": [expect("obsidian.append", 1, ["Pesquisa/arXiv.md", "Planning with Tool-Using Language Agents", "2609.01003"], ["Graph Neural Networks"])],
    "holdout": {
        "now": "2026-10-08T08:00:00-03:00",
        "responses": [{"capability": "rss.read", "result": [
            item("Self-Correcting Coding Agents", "https://arxiv.org/abs/2610.02001", "2026-10-08T04:00:00Z", "Agents fix their code."),
            item("Diffusion Models for Audio", "https://arxiv.org/abs/2610.02002", "2026-10-08T04:00:00Z", "Audio generation.")]}],
        "judgments": {"about_agents": {"2610.02001": 0.95, "2610.02002": 0.02}},
        "expect": [expect("obsidian.append", 1, ["Self-Correcting Coding Agents"], ["Diffusion Models for Audio"])],
    },
})

# 12. Reddit digest.
T.append({
    "id": "oc-12-reddit",
    "request": "Todo dia às 20h, me manda no Telegram os 3 posts mais votados do dia em r/LocalLLaMA e em r/selfhosted, com link.",
    "now": "2026-09-24T20:00:00-03:00",
    "calls": [
        {"capability": "rss.read", "args": {"url": "https://www.reddit.com/r/LocalLLaMA/top/.rss?t=day", "max": 3},
         "result": [item("Qwen 4 32B runs on a single 4090", "https://www.reddit.com/r/LocalLLaMA/comments/a1", "2026-09-24T12:00:00Z"),
                    item("My llama.cpp benchmark results", "https://www.reddit.com/r/LocalLLaMA/comments/a2", "2026-09-24T10:00:00Z"),
                    item("Best embedding model in 2026?", "https://www.reddit.com/r/LocalLLaMA/comments/a3", "2026-09-24T09:00:00Z")]},
        {"capability": "rss.read", "args": {"url": "https://www.reddit.com/r/selfhosted/top/.rss?t=day", "max": 3},
         "result": [item("I replaced Google Photos with Immich", "https://www.reddit.com/r/selfhosted/comments/b1", "2026-09-24T13:00:00Z"),
                    item("Homelab tour 2026", "https://www.reddit.com/r/selfhosted/comments/b2", "2026-09-24T11:00:00Z"),
                    item("Caddy vs Traefik", "https://www.reddit.com/r/selfhosted/comments/b3", "2026-09-24T08:00:00Z")]},
        sent("telegram.send", "🧵 r/LocalLLaMA: Qwen 4 32B runs on a single 4090 · My llama.cpp benchmark results · Best embedding model in 2026?\n🏠 r/selfhosted: I replaced Google Photos with Immich · Homelab tour 2026 · Caddy vs Traefik"),
    ],
    "outcome": "Uma mensagem com os 3 posts do topo do dia de cada subreddit, com links.",
    "expect": [expect("telegram.send", 1, ["Qwen 4 32B", "comments/a1", "Immich", "Caddy vs Traefik"])],
    "holdout": {
        "now": "2026-10-08T20:00:00-03:00",
        "responses": [
            {"capability": "rss.read", "result": [item("New 8B model tops the leaderboard", "https://www.reddit.com/r/LocalLLaMA/comments/c1", "2026-10-08T12:00:00Z")]},
            {"capability": "rss.read", "result": [item("Backups: 3-2-1 explained", "https://www.reddit.com/r/selfhosted/comments/d1", "2026-10-08T12:00:00Z")]}],
        "expect": [expect("telegram.send", 1, ["New 8B model tops the leaderboard", "3-2-1"], ["Immich"])],
    },
})

# 13. New YouTube videos of the day.
T.append({
    "id": "oc-13-youtube",
    "request": "Todo dia às 19h, me manda no Telegram os vídeos publicados hoje nos canais do YouTube UCsBjURrPoezykLs9EqgamOA e UCbRP3c757lWg9M-U7TyEkXA, com link. Se não saiu nada, não manda.",
    "now": "2026-09-24T19:00:00-03:00",
    "calls": [
        {"capability": "rss.read", "args": {"url": "https://www.youtube.com/feeds/videos.xml?channel_id=UCsBjURrPoezykLs9EqgamOA", "max": 10},
         "result": [item("Rust in 100 seconds", "https://www.youtube.com/watch?v=vid1", "2026-09-24T15:00:00Z"),
                    item("I tried every AI IDE", "https://www.youtube.com/watch?v=vid0", "2026-09-20T15:00:00Z")]},
        {"capability": "rss.read", "args": {"url": "https://www.youtube.com/feeds/videos.xml?channel_id=UCbRP3c757lWg9M-U7TyEkXA", "max": 10},
         "result": [item("Why Postgres keeps winning", "https://www.youtube.com/watch?v=vid9", "2026-09-22T14:00:00Z")]},
        sent("telegram.send", "🎬 Vídeos de hoje:\n• Rust in 100 seconds — https://www.youtube.com/watch?v=vid1"),
    ],
    "outcome": "Uma mensagem só com os vídeos publicados hoje nos dois canais; nada quando não saiu vídeo.",
    "expect": [expect("telegram.send", 1, ["Rust in 100 seconds", "vid1"], ["every AI IDE", "Postgres keeps winning"])],
    "holdout": {
        "now": "2026-10-08T19:00:00-03:00",
        "responses": [
            {"capability": "rss.read", "result": [item("Old video", "https://www.youtube.com/watch?v=old1", "2026-10-01T15:00:00Z")]},
            {"capability": "rss.read", "result": [item("Old video two", "https://www.youtube.com/watch?v=old2", "2026-10-05T15:00:00Z")]}],
        "expect": [expect("telegram.send", 0)],
    },
})

# 14. Newsletter digest and archive.
T.append({
    "id": "oc-14-newsletters",
    "request": "Todo dia às 18h, junta as newsletters que chegaram hoje numa mensagem só no Telegram (remetente e assunto) e arquiva os e-mails originais.",
    "now": "2026-09-24T18:00:00-03:00",
    "calls": [
        {"capability": "gmail.search", "args": {"query": "in:inbox", "days": 1, "max": 50},
         "result": [mail("nl1", "hello@thenews.cc", "the news", "☕ Quinta-feira, 24", "Bom dia! Hoje...", "2026-09-24T06:00:00-03:00", ("INBOX", "UNREAD", "CATEGORY_UPDATES")),
                    mail("nl2", "dan@tldrnewsletter.com", "TLDR", "TLDR AI 2026-09-24", "Top stories in AI", "2026-09-24T07:00:00-03:00", ("INBOX", "UNREAD", "CATEGORY_UPDATES")),
                    mail("nl3", "carla@acme.com", "Carla", "Reunião amanhã", "Podemos mudar para 15h?", "2026-09-24T11:00:00-03:00")]},
        sent("telegram.send", "📰 Newsletters de hoje:\n• the news — ☕ Quinta-feira, 24\n• TLDR — TLDR AI 2026-09-24"),
        {"capability": "gmail.archive", "args": {"id": "nl1"}, "result": {"ok": True}},
        {"capability": "gmail.archive", "args": {"id": "nl2"}, "result": {"ok": True}},
    ],
    "judgments": {"newsletter": {"nl1": 0.97, "nl2": 0.98, "nl3": 0.02}},
    "outcome": "Uma mensagem com as newsletters do dia e cada uma arquivada; os outros e-mails ficam onde estão.",
    "expect": [expect("telegram.send", 1, ["TLDR AI 2026-09-24", "the news"], ["Reunião amanhã"]), expect("gmail.archive", 2, ["nl1", "nl2"], ["nl3"])],
    "holdout": {
        "now": "2026-10-08T18:00:00-03:00",
        "responses": [{"capability": "gmail.search", "result": [
            mail("hn1", "news@morningbrew.com", "Morning Brew", "Markets today", "Stocks...", "2026-10-08T06:00:00-03:00", ("INBOX", "UNREAD", "CATEGORY_UPDATES")),
            mail("hn2", "chefe@acme.com", "Chefe", "Prazo do relatório", "Até amanhã.", "2026-10-08T09:00:00-03:00")]}],
        "judgments": {"newsletter": {"hn1": 0.96, "hn2": 0.02}},
        "expect": [expect("telegram.send", 1, ["Markets today"], ["Prazo do relatório"]), expect("gmail.archive", 1, ["hn1"], ["hn2"])],
    },
})

# 15. Apartment listings filter.
T.append({
    "id": "oc-15-apartments",
    "request": "Todo dia às 8h30, olha os e-mails de anúncios de imóveis que chegaram nas últimas 24h e me avisa no Telegram só dos apartamentos para alugar com 2 quartos, até R$ 3.000, em Pinheiros.",
    "now": "2026-09-24T08:30:00-03:00",
    "calls": [
        {"capability": "gmail.search", "args": {"query": "from:(quintoandar.com.br OR zapimoveis.com.br)", "days": 1, "max": 30},
         "result": [mail("ap1", "alertas@quintoandar.com.br", "QuintoAndar", "Novo imóvel: Rua dos Pinheiros, 2 quartos", "Apartamento 2 quartos, 68 m², Pinheiros, aluguel R$ 2.850.", "2026-09-24T07:10:00-03:00"),
                    mail("ap2", "alertas@quintoandar.com.br", "QuintoAndar", "Novo imóvel: Rua Cardeal Arcoverde, 3 quartos", "Apartamento 3 quartos, Pinheiros, aluguel R$ 4.900.", "2026-09-24T07:12:00-03:00"),
                    mail("ap3", "alertas@zapimoveis.com.br", "ZAP", "Novo anúncio na Vila Madalena", "Apartamento 2 quartos, Vila Madalena, aluguel R$ 2.700.", "2026-09-24T06:50:00-03:00")]},
        sent("telegram.send", "🏠 Apartamento que bate com o que você procura:\n• Novo imóvel: Rua dos Pinheiros, 2 quartos — 68 m², R$ 2.850"),
    ],
    "judgments": {"matches": {"ap1": 0.95, "ap2": 0.04, "ap3": 0.07}},
    "outcome": "Um aviso só com os anúncios de 2 quartos até R$ 3.000 em Pinheiros; nada quando nenhum bate.",
    "expect": [expect("telegram.send", 1, ["Rua dos Pinheiros"], ["Cardeal Arcoverde", "Vila Madalena"])],
    "holdout": {
        "now": "2026-10-08T08:30:00-03:00",
        "responses": [{"capability": "gmail.search", "result": [
            mail("hap1", "alertas@zapimoveis.com.br", "ZAP", "Novo anúncio em Moema", "Apartamento 2 quartos, Moema, R$ 2.600.", "2026-10-08T07:00:00-03:00")]}],
        "judgments": {"matches": {"hap1": 0.05}},
        "expect": [expect("telegram.send", 0)],
    },
})

# 16. Daily journal in Obsidian.
T.append({
    "id": "oc-16-journal",
    "request": "Todo dia às 23h55, escreve no Obsidian, na nota Diário.md, um registro do dia com as reuniões que tive e as issues do acme/app que fechei hoje.",
    "now": "2026-09-24T23:55:00-03:00",
    "calls": [
        {"capability": "calendar.events", "args": {"from": "2026-09-24T00:00:00-03:00", "to": "2026-09-24T23:59:59-03:00"},
         "result": [ev("j1", "Daily do time", "2026-09-24T09:30:00-03:00", "2026-09-24T09:45:00-03:00"), ev("j2", "Mentoria com o Léo", "2026-09-24T16:00:00-03:00", "2026-09-24T17:00:00-03:00")]},
        {"capability": "github.issues", "args": {"repo": "acme/app", "state": "closed", "max": 30},
         "result": [issue(405, "Corrigir cálculo de frete", "closed", "2026-09-24T18:00:00Z"), issue(390, "Issue fechada semana passada", "closed", "2026-09-17T12:00:00Z")]},
        {"capability": "obsidian.append", "args": {"note": "Diário.md", "text": "## 2026-09-24\nReuniões: Daily do time, Mentoria com o Léo\nFechadas: #405 Corrigir cálculo de frete"}, "result": {"ok": True}},
    ],
    "outcome": "Uma entrada no Diário.md com as reuniões do dia e as issues fechadas hoje.",
    "expect": [expect("obsidian.append", 1, ["Diário.md", "Mentoria com o Léo", "Corrigir cálculo de frete"], ["semana passada"])],
    "holdout": {
        "now": "2026-10-08T23:55:00-03:00",
        "responses": [
            {"capability": "calendar.events", "result": [ev("hj1", "Workshop de segurança", "2026-10-08T14:00:00-03:00", "2026-10-08T16:00:00-03:00")]},
            {"capability": "github.issues", "result": [issue(441, "Timeout no upload", "closed", "2026-10-08T20:00:00Z")]}],
        "expect": [expect("obsidian.append", 1, ["Workshop de segurança", "Timeout no upload"], ["Mentoria com o Léo"])],
    },
})

# 17. Home Assistant: doors and windows left open.
def sensor(eid, state, name):
    return {"entity_id": eid, "state": state, "name": name, "changed": "2026-09-24T21:00:00Z"}

T.append({
    "id": "oc-17-home-open",
    "request": "Todo dia às 23h, confere no Home Assistant se ficou alguma porta ou janela aberta e me avisa no Telegram quais são. Se estiver tudo fechado, não precisa avisar.",
    "now": "2026-09-24T23:00:00-03:00",
    "calls": [
        {"capability": "ha.states", "args": {"domain": "binary_sensor"},
         "result": [sensor("binary_sensor.porta_da_frente", "off", "Porta da frente"), sensor("binary_sensor.janela_da_sala", "on", "Janela da sala"),
                    sensor("binary_sensor.porta_da_varanda", "on", "Porta da varanda"), sensor("binary_sensor.movimento_corredor", "off", "Movimento corredor")]},
        sent("telegram.send", "🚪 Ficou aberto: Janela da sala, Porta da varanda"),
    ],
    "outcome": "Um aviso listando as portas e janelas abertas; silêncio quando tudo está fechado.",
    "expect": [expect("telegram.send", 1, ["Janela da sala", "Porta da varanda"], ["Porta da frente", "Movimento corredor"])],
    "holdout": {
        "now": "2026-10-08T23:00:00-03:00",
        "responses": [{"capability": "ha.states", "result": [sensor("binary_sensor.porta_da_frente", "off", "Porta da frente"), sensor("binary_sensor.janela_da_sala", "off", "Janela da sala")]}],
        "expect": [expect("telegram.send", 0)],
    },
})

# 18. Habit check-in: asking works; the answer comes back through the chat.
T.append({
    "id": "oc-18-habits",
    "request": "Todo dia às 21h me pergunta no Telegram se eu treinei hoje e se bebi 2 litros de água.",
    "now": "2026-09-24T21:00:00-03:00",
    "calls": [sent("telegram.send", "💪 Check-in do dia: você treinou hoje? E bebeu os 2 litros de água?")],
    "outcome": "Uma pergunta por dia no Telegram sobre o treino e a água.",
    "expect": [expect("telegram.send", 1, ["trein", "água"])],
    "holdout": {"now": "2026-10-08T21:00:00-03:00", "responses": [], "expect": [expect("telegram.send", 1, ["trein", "água"])]},
})

# 19. Price drop through a JSON API (web pages are not readable yet).
T.append({
    "id": "oc-19-price-drop",
    "request": "De hora em hora, olha o preço do anúncio MLB3456789 no Mercado Livre e me avisa no Telegram se ficar abaixo de R$ 3.200.",
    "now": "2026-09-24T12:00:00-03:00",
    "calls": [
        {"capability": "http.getJSON", "args": "https://api.mercadolibre.com/items/MLB3456789",
         "result": {"id": "MLB3456789", "title": "Notebook Ultra 14 16GB 512GB", "price": 3099.0, "currency_id": "BRL", "permalink": "https://produto.mercadolivre.com.br/MLB-3456789"}},
        sent("telegram.send", "💸 Notebook Ultra 14 16GB 512GB está por R$ 3.099 (abaixo de R$ 3.200): https://produto.mercadolivre.com.br/MLB-3456789"),
    ],
    "outcome": "Um aviso com o preço e o link quando fica abaixo de R$ 3.200; nada acima disso.",
    "expect": [expect("telegram.send", 1, ["3.099", "MLB-3456789"])],
    "holdout": {
        "now": "2026-10-08T12:00:00-03:00",
        "responses": [{"capability": "http.getJSON", "result": {"id": "MLB3456789", "title": "Notebook Ultra 14 16GB 512GB", "price": 3499.0, "currency_id": "BRL", "permalink": "https://produto.mercadolivre.com.br/MLB-3456789"}}],
        "expect": [expect("telegram.send", 0)],
    },
})

# 20. Price on a shop's page (Hermes pricing monitor), read as a web page.
def page(price, title="Processador AMD Ryzen 7 5700X3D AM4 | KaBuM!"):
    return {"url": "https://www.kabum.com.br/produto/520369", "title": title, "description": "", "truncated": False,
            "text": "Processador AMD Ryzen 7 5700X3D\nÀ vista no PIX\nR$ " + price.replace(".", ",") + "\nEm até 10x sem juros",
            "data": [{"@type": "Product", "name": "Processador AMD Ryzen 7 5700X3D", "offers": {"@type": "Offer", "price": price, "priceCurrency": "BRL", "availability": "https://schema.org/InStock"}}],
            "links": []}

T.append({
    "id": "oc-20-price-page",
    "request": "De hora em hora, olha o preço do Ryzen 7 5700X3D na página https://www.kabum.com.br/produto/520369 e me avisa no Telegram se ficar abaixo de R$ 1.100, com o link.",
    "now": "2026-09-24T12:00:00-03:00",
    "calls": [
        {"capability": "web.read", "args": {"url": "https://www.kabum.com.br/produto/520369"}, "result": page("1049.90")},
        sent("telegram.send", "💸 Ryzen 7 5700X3D por R$ 1.049,90 (abaixo de R$ 1.100): https://www.kabum.com.br/produto/520369"),
    ],
    "outcome": "Um aviso com o preço e o link quando fica abaixo de R$ 1.100; nada acima disso.",
    "expect": [expect("telegram.send", 1, ["1.049,90", "kabum.com.br/produto/520369"])],
    "holdout": {
        "now": "2026-10-08T12:00:00-03:00",
        "responses": [{"capability": "web.read", "result": page("1249.00")}],
        "expect": [expect("telegram.send", 0)],
    },
})

# 21. Hacker News as a spoken podcast, in a language the owner picks.
HN = "https://hn.algolia.com/api/v1/search"
def story(i, title, points, comments):
    return {"objectID": str(i), "title": title, "url": f"https://example{i}.com/post", "points": points, "num_comments": comments, "author": f"user{i}", "created_at": "2026-09-24T02:00:00Z"}
def comments(*texts):
    return {"hits": [{"comment_text": t, "author": f"c{n}", "story_id": 1} for n, t in enumerate(texts)]}

T.append({
    "id": "oc-21-hn-podcast",
    "request": "Todo dia às 8h, pega as 3 histórias com mais pontos na primeira página do Hacker News, faz um resumo de cada uma com o que a comunidade está comentando e me manda como um podcast em áudio, em português (quero poder trocar o idioma depois).",
    "now": "2026-09-24T08:00:00-03:00",
    "calls": [
        {"capability": "http.getJSON", "args": HN + "?tags=front_page&hitsPerPage=10", "result": {"hits": [
            story(101, "Open-weight model beats GPT on coding", 910, 402), story(102, "The history of the Unix pipe", 450, 120),
            story(103, "Why SQLite is everywhere", 780, 300), story(104, "Show HN: A tiny text editor", 150, 40), story(105, "Postgres 19 released", 620, 210)]}},
        {"capability": "http.getJSON", "args": HN + "?tags=comment,story_101&hitsPerPage=5", "result": comments("The benchmark numbers look solid but it needs 80GB of VRAM.", "Finally an open model that competes.")},
        {"capability": "http.getJSON", "args": HN + "?tags=comment,story_103&hitsPerPage=5", "result": comments("SQLite is in every phone and browser.", "The test suite is the real product.")},
        {"capability": "http.getJSON", "args": HN + "?tags=comment,story_105&hitsPerPage=5", "result": comments("Async I/O by default is huge.", "Upgrade path looks painless.")},
        {"capability": "audio.send", "args": {"title": "Hacker News — 24/09", "language": "pt-BR", "text": "Bom dia! Estas são as três histórias com mais pontos no Hacker News hoje. Primeira: Open-weight model beats GPT on coding. Um modelo aberto superou o GPT num teste de programação; a comunidade elogia os números, mas lembra que ele precisa de 80 GB de memória de vídeo. Segunda: Why SQLite is everywhere. O texto explica por que o SQLite está em todo celular e navegador, e os comentários destacam a suíte de testes. Terceira: Postgres 19 released. A nova versão traz entrada e saída assíncronas por padrão, e quem comentou diz que atualizar é tranquilo. Até amanhã!"}, "result": {"ok": True}},
    ],
    "outcome": "Um podcast em áudio por dia com as 3 histórias de mais pontos do Hacker News, cada uma com o resumo e o que a comunidade comenta, no idioma escolhido (português por padrão).",
    "expect": [expect("audio.send", 1, ["Open-weight model beats GPT on coding", "Why SQLite is everywhere", "Postgres 19 released"], ["Unix pipe", "tiny text editor"])],
    "holdout": {
        "now": "2026-10-08T08:00:00-03:00",
        "responses": [
            {"capability": "http.getJSON", "result": {"hits": [story(201, "Rust in the Linux kernel, one year on", 700, 300), story(202, "I built a CPU in a spreadsheet", 820, 250),
                story(203, "Ask HN: What are you working on?", 300, 800), story(204, "The economics of AI chips", 640, 190)]}},
            {"capability": "http.getJSON", "result": comments("Spreadsheets are Turing complete after all.")},
            {"capability": "http.getJSON", "result": comments("Driver maintainers are happier.")},
            {"capability": "http.getJSON", "result": comments("Margins are the whole story.")}],
        "expect": [expect("audio.send", 1, ["I built a CPU in a spreadsheet", "Rust in the Linux kernel", "The economics of AI chips"], ["What are you working on"])],
    },
})

# 22. Habit check-in that acts on the answer (ask.owner).
T.append({
    "id": "oc-22-habit-answer",
    "request": "Toda noite às 21h me pergunta se eu treinei hoje (Sim ou Não) e, quando eu responder, me diz quantos treinos fiz nos últimos 7 dias.",
    "now": "2026-09-24T21:00:00-03:00",
    "calls": [{"capability": "ask.owner", "args": {"question": "Treinou hoje?", "options": ["Sim", "Não"], "key": "treino"}, "result": {"asked": "q1"}}],
    "outcome": "Uma pergunta por noite com Sim/Não; ao responder, uma mensagem com o total de treinos dos últimos 7 dias.",
    "expect": [expect("ask.owner", 1, ["Sim", "Não"])],
    "holdout": {
        "now": "2026-10-08T21:05:00-03:00",
        "responses": [],
        "event": {"answer": {"key": "treino", "question": "Treinou hoje?", "choice": "Sim", "index": 0, "asked": "2026-10-08T21:00:00-03:00"}},
        "expect": [expect("telegram.send", 1, ["1"]), expect("ask.owner", 0)],
    },
})

# 23. A shop's webhook tells about a new order.
T.append({
    "id": "oc-23-order-webhook",
    "request": "Quando minha loja mandar o webhook de pedido novo, me avisa no Telegram com o nome do cliente, o valor e os itens.",
    "now": "2026-09-24T14:12:00-03:00",
    "event": {"webhook": {"method": "POST", "query": {}, "received": "2026-09-24T14:12:00-03:00",
        "body": {"order_id": "1042", "customer": {"name": "Marina Souza"}, "total": 189.9, "currency": "BRL", "items": [{"name": "Caneca Pimpo", "qty": 2}, {"name": "Camiseta", "qty": 1}]}}},
    "calls": [sent("telegram.send", "🛒 Pedido 1042 de Marina Souza: R$ 189,90 — 2× Caneca Pimpo, 1× Camiseta")],
    "outcome": "Uma mensagem para cada pedido que a loja manda, com cliente, valor e itens.",
    "expect": [expect("telegram.send", 1, ["Marina Souza", "189,90", "Caneca Pimpo"])],
    "holdout": {
        "now": "2026-10-08T10:00:00-03:00",
        "responses": [],
        "event": {"webhook": {"method": "POST", "query": {}, "received": "2026-10-08T10:00:00-03:00",
            "body": {"order_id": "1107", "customer": {"name": "Rafael Lima"}, "total": 54.5, "currency": "BRL", "items": [{"name": "Adesivos", "qty": 5}]}}},
        "expect": [expect("telegram.send", 1, ["Rafael Lima", "54,50", "Adesivos"], ["Marina"])],
    },
})

out = here
for t in T:
    with open(os.path.join(out, t["id"] + ".json"), "w") as f:
        json.dump(t, f, ensure_ascii=False, indent=1)
print(len(T), "traces")
