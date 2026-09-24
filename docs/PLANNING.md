# Vigia — Planejamento completo

> **Nome provisório.** Premissas: gratuito e open source, local-first, sem prazos (cada fase termina num gate).
> Prioridades e decisões de UI foram avaliadas com o **Jev** (TypeSafe System One). Os dados brutos estão em `jev/`, e o método no Apêndice A.
> Validação na comunidade: **em stand by** por decisão do Dener.

---

## 0. Resumo em uma página

**O que é:** um agente pessoal de IA, gratuito e local, que faz o que o OpenClaw e o Hermes fazem (conversa pelo Telegram, lê e-mail e agenda, automatiza tarefas), mas foi **desenhado a partir das falhas deles**.

**A ideia central — o agente que vira software:**
1. Na primeira vez, o agente resolve a tarefa com um LLM e pede aprovação para o que não tem volta.
2. Quando dá certo, ele **compila a tarefa numa rotina**: código legível, com testes e uma lista do que pode tocar.
3. A rotina roda sozinha no horário, **sem LLM**, por centavos. Um modelo de julgamento só entra em decisões pontuais ("este e-mail é importante?").
4. Se algo muda, a rotina para, avisa e propõe um conserto que você aprova.

**Por que é melhor que os dois:** eles passam **todo** pedido pelo LLM, toda vez. É daí que vêm a falta de confiabilidade (motivo nº 1 de abandono), o custo (até $131 por dia) e as regras esquecidas (o caso dos 200 e-mails apagados). O Vigia também é **seguro por construção**: regras fora do modelo, capacidades mínimas, aprovação no celular, chaves invisíveis, recibo e desfazer para tudo, e limite de gasto real (nenhum dos dois tem).

**A interface** (validada com o Jev): **chat para conversar e aprovar, mais uma interface web local para ver e controlar**. A tela principal é **Rotinas**. O momento que faz alguém dizer "uau" é **ver a tarefa que acabou de fazer virar uma rotina, com o custo caindo para zero**.

**O caminho completo (seção 8):** 13 fases, do primeiro commit até um projeto maduro. Primeiro construir o núcleo (F0–F4: compilador, segurança, UI, três usos perfeitos). Depois abrir para o mundo (F5–F8: instalação, migração, família, galeria, conectores). Por fim ampliar e amadurecer (F9–F12: pequeno negócio, apps nativos, rede de proteção, longevidade).

**Primeiro marco:** o resumo matinal roda 30 dias seguidos como rotina compilada, com custo de LLM próximo de zero e nenhuma falha silenciosa.

---

## 1. O problema, com evidência

| Dor | Evidência | Causa estrutural nos concorrentes |
|---|---|---|
| Não é confiável | Motivo nº 1 de abandono nos tópicos do Hacker News sobre o OpenClaw; "um cron job faz o mesmo" | Todo pedido é re-decidido por um LLM |
| Caro e imprevisível | Heartbeat de $18,75 por noite; relatos de até $131 por dia (Notebookcheck, docs do OpenClaw) | Contexto inteiro reenviado a cada turno; nenhum limite de gasto (O#42475, H#6839) |
| Esquece regras e faz estrago | Uma pesquisadora da Meta teve mais de 200 e-mails apagados depois que a compactação do contexto removeu "confirme antes" (TechCrunch, 23/02/2026) | Regras moram só no contexto do modelo |
| Inseguro | 135 mil instâncias expostas, CVEs 9,9; skills maliciosas no ClawHub; banido na Meta, no Google e na Amazon | Aprovações desligadas por padrão; skills com poder total; chaves legíveis pelo agente (O#7916) |
| Não dá para ver nem desfazer | O log de auditoria do OpenClaw não guarda argumentos; o Hermes não tem tabela de auditoria; pedidos de rollback sem resposta (H#12238, O#153257) | Observabilidade e reversibilidade não fazem parte do projeto |
| Difícil de manter | Cerca de 25% de chance de uma atualização quebrar a entrega; mais de 5 mil issues abertas em cada projeto | Pilhas grandes (Node, Python, Docker) e ritmo acelerado |

**O espaço está livre:** o OpenClaw entrou em "modo de estabilização" (#5799) e fecha pedidos de funcionalidade automaticamente. O Hermes deixa governança parada em `needs-decision`. Pedidos de aprovação por ferramenta, auditoria e limite de gasto tiveram PRs fechados sem merge.

---

## 2. Para quem

**Persona 1 — Marina, a entusiasta queimada (primeiro alvo)**
- Desenvolvedora ou profissional técnica. Instalou o OpenClaw ou o Hermes num Mac mini ou numa VPS.
- Usou para resumo matinal e triagem de e-mail. Desistiu porque falhava sem avisar e a conta de API assustou. Ou tem medo desde que leu sobre os e-mails apagados.
- Quer: que funcione todo dia, saber quanto vai custar, ver o que o agente fez e desfazer.
- Onde está: r/openclaw, Hacker News, Discords dos projetos.

**Persona 2 — Carlos, o dono de pequeno negócio (segundo alvo)**
- Não técnico. Resolve tudo pelo celular.
- Quer: orçamento a partir de uma foto, fatura em PDF, cobrança de clientes, sem medo de o agente mandar a mensagem errada.
- Precisa: instalação sem terminal (F5) e aprovações claras.

### 2.1 Pesquisa com usuários, contínua e privada
O Jev apontou "falar com usuários reais" como a maior lacuna em todas as revisões (até 0,97). A **divulgação pública está em stand by**, mas pesquisa privada não é divulgação. Proposta, sujeita à decisão nº 5:

| Fase | Pesquisa | Com quem | O que decide |
|---|---|---|---|
| F1 | 5 entrevistas mostrando o momento da compilação (protótipo ou vídeo) | Pessoas que usaram e largaram OpenClaw ou Hermes | Se a ideia das rotinas resolve a dor delas; quais três usos priorizar |
| F2 | 3 sessões sobre aprovações e regras | As mesmas pessoas | O texto das aprovações e o limite de incômodo |
| F3 | Teste de usabilidade com roteiro fixo (o gate da fase) | 5 pessoas novas | Se a UI conta a história sozinha |
| F4 | Diário de uso por 2 semanas | 3 pessoas usando de verdade | Falhas reais, confiança, custo percebido |
| F5 | Beta fechado de instalação e migração | 10 pessoas, técnicas e não técnicas | Se está pronto para a v1.0 |
| F9 | Acompanhamento de 3 pequenos negócios | Donos de pequenos negócios | O que o backoffice precisa ter |

Cada rodada termina com um registro curto (o que se aprendeu e o que muda no plano), guardado junto ao código.

**Fora do alvo, por enquanto:** empresas com exigências de compliance (Microsoft e NVIDIA estão lá) e desenvolvedores que querem um agente de código (Claude Code e Codex resolvem).

---

## 3. Princípios de produto

1. **Uma rotina vale mais que mil conversas.** O que se repete vira software. O LLM é para o que é novo.
2. **Nada irreversível sem você.** E quase tudo é reversível.
3. **As regras são suas, não do modelo.** Moram fora do contexto e o agente não pode editá-las.
4. **Tudo tem recibo.** Se aconteceu, dá para ver o quê, por quê, quanto custou e como desfazer.
5. **Nunca falhar em silêncio.** Toda falha vira um aviso no celular.
6. **Custo previsível.** Orçamento duro, projeção mensal, sem surpresas.
7. **Poucas coisas, perfeitas.** Três usos impecáveis antes de 150 integrações.
8. **Explicar como para uma pessoa.** Nada de jargão na interface. "Vai apagar 212 e-mails da pasta Trabalho", não `gmail.messages.batchDelete`.

---

## 4. Funcionalidades priorizadas (avaliadas com o Jev)

Escala de 0 a 3 para valor para o usuário, diferenciação contra o OpenClaw e o Hermes, risco técnico e necessidade no MVP. A prioridade é `0,45×valor + 0,35×diferenciação − 0,2×risco`.

### 4.1 Núcleo do MVP (necessidade ≥ 2,2), dividido em duas entregas

O resumo matinal só **lê** dados e **envia para você**. Não tem ação irreversível. Por isso a prova técnica (F1) pode ser pequena, e o pacote de segurança completo vem logo depois (F2), antes de qualquer caso que escreva ou apague.

**F1 — a prova (v0.1)**

| Funcionalidade | Valor | Difer. | Risco | MVP | Versão na F1 |
|---|---|---|---|---|---|
| Compilador de rotinas | 2,97 | 2,63 | 1,73 | 2,98 | Completo; é a prova |
| Agendador | 2,94 | 1,31 | 1,05 | 2,99 | Horário fixo (cron) |
| Resumo matinal | 2,76 | 1,35 | 1,53 | 2,99 | Agenda + e-mails importantes |
| Modo exploração | 2,19 | 1,19 | 1,62 | 2,88 | Só com capacidades de leitura e envio para o dono |
| Canal Telegram | 2,76 | 1,17 | 1,30 | 2,79 | Conversa e avisos |
| Capacidades por rotina | 2,94 | 2,33 | 1,19 | 2,69 | Manifesto checado no runtime |
| **Limite de gasto** | 2,95 | 2,88 | 1,01 | 2,58 | Limite diário simples, checado antes de cada chamada (**maior prioridade geral: 2,13**) |
| Passos de julgamento calibrados | 2,56 | 2,55 | 1,59 | 2,35 | Um julgamento: "e-mail importante?" |
| Vigia de saúde | 2,70 | 2,29 | 1,17 | 2,26 | Toda falha vira aviso |
| UI: Rotinas (mínima) | 2,61 | 2,45 | 0,89 | 1,63 | Cartão com execuções, custo e capacidades; escolhida pelo Jev como tela principal |

**F2 — seguro por construção (v0.2)**

| Funcionalidade | Valor | Difer. | Risco | MVP | Por que na F2 |
|---|---|---|---|---|---|
| Aprovação no celular | 2,91 | 1,92 | 0,88 | 2,92 | Necessária a partir do primeiro caso que escreve ou apaga |
| Motor de regras fora do modelo | 2,95 | 2,74 | 1,63 | 2,74 | Idem; resolve o caso dos 200 e-mails |
| Corretor de credenciais | 2,79 | 2,45 | 1,36 | 2,52 | Na F1, as poucas credenciais ficam no cofre local |
| Autoconserto por diff | 2,46 | 2,68 | 1,93 | 2,44 | Risco alto; na F1, a falha só avisa |
| Desfazer | 2,81 | 2,53 | 1,53 | 2,36 | Só faz sentido com ações de escrita |
| Auditoria com argumentos | 2,80 | 2,64 | 1,35 | 2,32 | Na F1, o log de eventos já grava; a F2 adiciona o encadeamento e a verificação |
| **UI: Recibos** | 2,90 | 2,57 | 1,03 | 2,30 | **Segunda maior prioridade geral (2,00)**; nasce junto com o desfazer |

### 4.2 v1 (alto valor ou diferenciação, fora do núcleo do MVP)
- UI: Rotinas completa (detalhe com código, testes e histórico).
- UI: Custo (1,82), UI: Regras (1,69), UI: Exploração ao vivo (1,62) e UI: Onboarding (1,51).
- Sandbox ligado por padrão (1,49), memória com origem (1,69) e modelo local de julgamento (1,68).
- Triagem de e-mail (1,49), anti-incômodo nas aprovações (1,52) e atualização segura (1,31).
- Migração do OpenClaw e do Hermes (1,52): diferenciação alta (2,69), mas não é necessária no MVP (0,43).
- Memória versionada (1,53) e UI: Memória (1,39).

### 4.3 Depois
- Backoffice de pequeno negócio (1,26): valor bom, mas exige a F5 (instalação sem terminal).
- Canais WhatsApp, e-mail e voz; modo família (0,90); app móvel nativo (0,68); app desktop (0,83).
- Plugin Guard para OpenClaw e Hermes (1,08) e rede de proteção compartilhada (1,14).
- Usar assinatura do Claude ou do ChatGPT como cérebro (0,75) e importar skills do ecossistema (0,70).

**Duas surpresas honestas do Jev:**
- **Usar a assinatura existente** teve valor baixo para o nosso alvo (0,95), apesar de ser a issue mais votada do Hermes. Faz sentido: com rotinas compiladas, o custo de LLM já cai perto de zero. Fica para depois.
- **Qualquer provedor por API** teve diferenciação quase nula (0,30). É paridade necessária, não argumento de venda.

---

## 5. UX e UI

### 5.1 Decisões validadas com o Jev
- **Superfícies:** chat (Telegram) para conversar, receber avisos e aprovar, mais uma **interface web local instalável (PWA)** para ver e controlar. Confiança 1,00 contra "só chat", "app desktop" e "app móvel". **App desktop fora do MVP** (P=0,16).
- **Tela principal:** **Rotinas** (0,53, contra Regras 0,26 e Recibos 0,14).
- **Momentos "uau", em ordem:**
  1. **A compilação** (0,59): ver a tarefa que acabou de fazer virar uma rotina legível, testada e com lista de capacidades, e ver o custo cair para cerca de $0.
  2. **O bloqueio** (0,40): uma ação perigosa bloqueada e explicada no celular, com desfazer em um toque.

### 5.2 Princípios de interface
1. **Mostrar o que o agente sabe fazer como objetos concretos**, não como conversa. Uma rotina é um cartão com nome, horário, saúde, custo e capacidades, não um prompt perdido no histórico.
2. **Toda ação tem três camadas de detalhe:** a frase humana ("Arquivou 14 newsletters"), o resumo técnico (ferramenta e alvo) e os argumentos completos, sob demanda.
3. **Cor significa risco, não decoração.** Verde para leitura, âmbar para escrita reversível, vermelho para irreversível. A mesma linguagem aparece no Telegram (emoji) e na web.
4. **Nunca uma tela vazia sem próximo passo.** O estado vazio de Rotinas é "Peça algo que você faz toda semana".
5. **O custo sempre à vista**, discreto, em cada rotina e cada exploração.
6. **Movimento com propósito:** a compilação é animada (a conversa se condensa num cartão de rotina). É o único lugar com animação marcante, porque é o momento do produto.

### 5.3 Arquitetura de informação (web)

```text
┌ Barra lateral ─────────┐
│  Rotinas        (início)│  cartões das rotinas + "Nova tarefa"
│  Precisa de você   (3)  │  aprovações e alertas pendentes
│  Recibos                │  linha do tempo de tudo que aconteceu
│  Regras                 │  regras em linguagem natural
│  Custo                  │  gasto, orçamento, projeção
│  Memória                │  o que o agente sabe, com origem
│  Conexões               │  Gmail, agenda, Telegram, modelos
│  Ajustes                │
└─────────────────────────┘
 Topo: busca (⌘K) · orçamento de hoje ($0,04 de $1,00) · status de saúde
```

### 5.4 As telas

**Rotinas (início)**
- Grade de cartões. Cada cartão mostra: nome ("Resumo matinal"), próximo horário ("amanhã 07:00"), as últimas 14 execuções como pontinhos coloridos, o custo do mês ("$0,03"), os ícones das capacidades (📧 ler · 📅 ler · ✈️ enviar) e o estado de saúde.
- Um cartão em "exploração" (ainda não compilado) aparece tracejado, com o botão "Transformar em rotina".
- No topo: "Esta semana suas rotinas economizaram ~$4,20 em LLM" (a projeção comparada a rodar a mesma coisa com LLM toda vez).
- **Estado vazio:** "Peça algo que você faz toda semana", com três sugestões clicáveis (resumo matinal, triagem de e-mail, lembrete de contas).

**Detalhe da rotina**
- Abas: **Visão geral** (execuções, custo, saúde), **Código** (legível, com destaque de sintaxe e as chamadas de capacidade realçadas), **Testes** (entradas gravadas e resultado de cada teste), **Capacidades** (o que pode e o que não pode, com o motivo), **Histórico** (versões e diffs de cada conserto, com quem aprovou) e **Execuções** (cada execução com o recibo).
- Ações: rodar agora, pausar, editar horário, desfazer a última versão.

**Nova tarefa e exploração ao vivo**
- Uma caixa grande: "O que você quer que eu faça?".
- Durante a exploração, uma **linha do tempo ao vivo**: cada passo aparece com a frase humana, o nível de risco (cor) e o custo acumulado. Ações irreversíveis param com um cartão de aprovação inline.
- Ao final, se deu certo, o **momento da compilação**:
  1. "Isso funcionou. Quer que eu faça isso sozinho todo dia às 7h?"
  2. A linha do tempo se condensa num cartão de rotina (animação).
  3. É mostrado, lado a lado: "Hoje: $0,31 com LLM → Rotina: ~$0,002 por dia", as capacidades pedidas e os testes gerados (todos verdes).
  4. Botões: "Ativar rotina", "Ver código", "Ajustar".

**Precisa de você**
- Lista de pedidos de aprovação, alertas e dúvidas do agente, idêntica ao que chegou no Telegram.
- Cada pedido mostra o que vai acontecer em linguagem humana, a origem (qual rotina ou conversa), o risco e as opções: permitir uma vez, sempre, ou negar.
- Anti-incômodo: pedidos parecidos agrupados ("12 e-mails para arquivar — revisar todos") e a sugestão "Você aprovou isto 5 vezes. Tornar permanente?".

**Recibos**
- Linha do tempo agrupada por dia e por rotina ou conversa. Cada item: frase humana, ícone de risco, custo e um botão **Desfazer** quando reversível ("Desfazer até 07:12").
- Filtros: só irreversíveis, só bloqueados, por rotina, por conexão.
- Clicar num item abre os argumentos completos e a regra que permitiu ou bloqueou.
- Os bloqueios aparecem destacados: "Bloqueado: apagar 212 e-mails — regra 'nunca apague e-mail sem me perguntar'".

**Regras**
- Lista de regras em linguagem natural ("Nunca apague e-mail sem me perguntar", "Não gaste mais de $1 por dia", "Só mande mensagens para mim").
- Ao escrever uma regra nova: a política compilada aparece ao lado, em formato legível, para conferir. Depois, **"Testar contra a última semana"**: "esta regra teria bloqueado 3 ações", com a lista.
- Regras criadas pelo agente não existem. Só o usuário cria e edita.

**Custo**
- Barra do orçamento de hoje e do mês, gasto por rotina, por conversa e por modelo, e a projeção da conta mensal.
- "Maiores gastos": qual exploração ou conserto custou mais.
- Aviso quando uma rotina começa a custar mais que o normal (sinal de que algo mudou).

**Memória**
- Arquivos legíveis (o que o agente sabe sobre você, suas preferências, contatos), cada fato com a origem ("você disse em 12/09", "e-mail de desconhecido — baixa confiança").
- Histórico de versões e restauração.

**Conexões**
- Contas conectadas (Gmail, Google Agenda, Telegram), o modelo de LLM configurado e o backend de julgamento.
- Cada conexão mostra quais rotinas a usam e com qual capacidade.
- As chaves nunca aparecem: só "conectado em 12/09 · usado por 3 rotinas".

**Onboarding (primeira execução)**
1. "Oi, eu sou o Vigia." Três telas curtas explicando: rotinas, regras e recibos.
2. Conectar o Telegram (QR code ou link do bot).
3. Conectar o modelo (chave de API, ou modelo local via Ollama).
4. "O que eu posso fazer sem te perguntar?": três opções simples (conservador, equilibrado, liberal), que viram regras editáveis.
5. Orçamento diário, com um valor sugerido.
6. A primeira tarefa: "Quer um resumo da sua agenda amanhã às 7h?", que leva direto ao momento da compilação.
- **Meta:** a primeira rotina ativa em menos de 5 minutos.

### 5.5 O design das mensagens no Telegram
O Telegram é metade da interface e segue o mesmo sistema:
- **Aprovação:**
  ```text
  🔴 Ação irreversível — Triagem de e-mail
  Vai apagar 212 e-mails da pasta "Trabalho" (2019–2024).
  Regra: "nunca apague e-mail sem me perguntar"
  [Permitir uma vez] [Mover para a lixeira] [Negar]
  ```
- **Recibo diário** (opcional): "Hoje: 3 rotinas rodaram, 1 ação bloqueada, $0,04 gastos. Ver recibos →".
- **Falha:** "⚠️ O Resumo matinal não rodou: o Gmail recusou o acesso. Preciso que você reconecte. [Abrir Conexões]".
- **Autoconserto:** "🛠 A API do clima mudou. Preparei um conserto (3 linhas). [Ver diff] [Aprovar] [Ignorar]".
- Sempre com botões. Texto curto. Um link para o detalhe na web.

### 5.6 Linguagem visual
- **Identidade:** calma e confiável. É um produto sobre controle, não sobre magia. Escuro por padrão, com modo claro.
- **Cores semânticas:** verde (leitura, sucesso), âmbar (escrita reversível, atenção), vermelho (irreversível, bloqueio, falha) e azul (exploração em andamento). Cores neutras em todo o resto.
- **Tipografia:** uma sans legível para a interface e uma mono para código e argumentos.
- **Componentes:** cartão de rotina, linha do tempo, cartão de aprovação, selo de capacidade, selo de risco, visualizador de diff, barra de orçamento, sparkline de execuções.
- **Movimento:** discreto em toda parte. Marcante apenas na compilação (condensação da linha do tempo em cartão) e no bloqueio (o cartão treme e fica vermelho).
- **Acessibilidade:** contraste AA, navegação completa por teclado, cor nunca como único sinal (sempre ícone e texto) e `prefers-reduced-motion` respeitado.
- **Responsivo:** a web funciona no celular (PWA). No celular, a barra lateral vira barra inferior com Rotinas, Precisa de você e Recibos.

### 5.7 Métricas de UX
- Tempo até a primeira rotina ativa: menos de 5 minutos.
- Pedidos de aprovação por dia: menos de 5 (acima disso as pessoas desligam).
- Taxa de "Transformar em rotina" aceita depois de uma exploração bem-sucedida.
- Desfazer usado em menos de 2% das ações (se for mais, o agente está errando demais).
- Tarefas de usabilidade (encontrar o que o agente fez ontem, desfazer uma ação, criar uma regra) resolvidas sem ajuda por 4 de 5 pessoas testadas.

---

## 6. Arquitetura técnica

### 6.1 Visão geral

```text
 Telegram ◀────────────┐                     ┌──────▶ Web UI (PWA, React)
                       │                     │
 ┌─────────────────────┴─────────────────────┴───────────────────────────┐
 │ vigia — um único binário Go                                            │
 │                                                                         │
 │  Gateway de canais ──▶ Conversa ──▶ Exploração (loop de ferramentas) ──┐ │
 │                                          │                             │ │
 │                                          ▼                             │ │
 │                               Compilador de rotinas                    │ │
 │                    (código + testes + manifesto de capacidades)        │ │
 │                                          │                             │ │
 │  Agendador ──▶ Runtime de rotinas (JavaScript no goja, Go puro) ─────────┤ │
 │                                                                        ▼ │
 │                     Motor de políticas  ◀── regras do usuário            │
 │              (capacidades · regras · risco · orçamento · aprovação)     │
 │                                          │                               │
 │         ┌───────────────┬────────────────┼────────────────┐              │
 │         ▼               ▼                ▼                ▼              │
 │   Conectores      Corretor de      Camada de LLM    Julgamento           │
 │ (Gmail, agenda,   credenciais     (provedores,     (Jev · modelo local  │
 │  clima, HTTP)     (cofre local)    proxy de gasto)   · LLM barato)       │
 │                                                                          │
 │  Armazenamento: SQLite (eventos só-acréscimo, auditoria encadeada)       │
 │  Memória: arquivos Markdown versionados (git embutido, go-git)           │
 │  Vigia de saúde · API HTTP + WebSocket para a UI                         │
 └──────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Stack
- **Backend:** Go (um binário, compilação cruzada para macOS, Linux e Raspberry Pi). SQLite via `modernc.org/sqlite` (sem cgo).
- **Runtime de rotinas:** `goja`, um interpretador de JavaScript em Go puro (sem cgo), escolhido com o Jev por ser a mudança que mais aumenta a chance de fechar a F1 (0,66). A rotina não tem acesso a rede, disco nem processo: só enxerga as funções de capacidade injetadas. Tempo limitado por interrupção e memória limitada por execução. Se um dia for preciso isolamento mais forte, QuickJS em WebAssembly (`wazero`) entra como alternativa, com a mesma interface.
- **UI:** React, TypeScript e Vite, embutida no binário. Componentes acessíveis (primitivos do Radix), estilos com tokens de design próprios, animações com Motion. Atualização em tempo real por WebSocket.
- **Canais:** Telegram Bot API (MVP). WhatsApp depois.
- **LLM:** provedores por API (Anthropic, OpenAI, OpenRouter) e modelos locais via Ollama. Toda chamada passa pelo proxy de gasto interno.
- **Julgamento:** interface única com três backends: **Jev** (probabilidades calibradas, recomendado quando há chave), **modelo local pequeno** (gratuito e offline; treinado com MLX e servido via Ollama ou llama.cpp) e **LLM barato** (fallback).
- **Memória:** arquivos Markdown num repositório git embutido (`go-git`), para histórico e restauração.

### 6.3 Modelo de dados (principais entidades)
- `event`: log só-acréscimo de tudo (tipo, ator, dados, hash do anterior) → auditoria e recibos.
- `routine`: nome, versão atual, código, testes, manifesto, agenda, estado.
- `routine_version`: código, testes, manifesto, diff, motivo e quem aprovou.
- `run`: execução de rotina ou exploração (início, fim, resultado, custo, ações).
- `action`: cada chamada de capacidade (ferramenta, argumentos, risco, decisão da política, reversível?, desfeita?).
- `rule`: texto do usuário, política compilada, ativa?.
- `approval`: pedido, contexto, decisão, canal e quando.
- `connection`: tipo, escopo e referência ao segredo no cofre (nunca o valor).
- `budget`: limites por dia, mês e rotina.

### 6.4 Rotinas compiladas em detalhe
**Formato**
```js
// routine: morning-brief v3
export const manifest = {
  schedule: "0 7 * * *",
  capabilities: ["gmail.read:inbox", "calendar.read:primary", "http.get:api.open-meteo.com", "telegram.send:owner"],
  judgments: { important: "Is this email important for the owner today?" },
};

export default async function run({ gmail, calendar, http, telegram, judge }) {
  const events = await calendar.today();
  const mails = await gmail.unread({ since: "24h" });
  const important = [];
  for (const m of mails) {
    const j = await judge.important(m);          // probabilidade calibrada
    if (j.p >= 0.7) important.push(m);
    else if (j.p >= 0.4) important.push({ ...m, unsure: true });
  }
  const weather = await http.getJSON("https://api.open-meteo.com/v1/forecast?...");
  await telegram.send(formatBrief(events, important, weather));
}
```

**Ciclo de vida**
1. **Exploração:** o LLM usa as mesmas funções de capacidade, e cada chamada é gravada (entrada e saída).
2. **Compilação:** o LLM recebe o registro da exploração e gera código, manifesto e testes. Os testes reproduzem as entradas gravadas e verificam as saídas e as chamadas.
3. **Verificação:** o código roda no runtime isolado contra os testes. O manifesto é conferido: nenhuma chamada fora das capacidades declaradas é possível.
4. **Revisão:** o usuário vê o cartão (capacidades, custo estimado, testes) e ativa.
5. **Execução:** o agendador roda sem LLM. Cada chamada passa pelo motor de políticas.
6. **Quebra:** um erro de capacidade, uma mudança de formato de API ou um julgamento incerto demais (limiar configurável) interrompem a execução, avisam e abrem uma exploração de conserto.
7. **Conserto:** o LLM propõe um diff, e os testes (antigos e novos) precisam passar. O usuário aprova o diff e nasce uma nova versão.

**O que não vira rotina:** tarefas únicas ou abertas demais. Elas ficam no modo exploração, igualmente protegidas pelo motor de políticas.

### 6.4.1 Especificação: quando uma tarefa é compilável
O compilador só propõe uma rotina quando **todas** as condições valem. As condições são checadas por código, e as que dependem de julgamento passam pelo backend de julgamento:
1. **Repetição:** o usuário pediu recorrência ("todo dia", "sempre que chegar"), ou a mesma intenção apareceu 2 ou mais vezes.
2. **Sucesso verificado:** a exploração terminou sem erro, sem ação negada e com o resultado aprovado pelo usuário (um 👍 ou "ficou bom").
3. **Estrutura estável:** as chamadas de capacidade da exploração formam uma sequência que pode ser escrita como código com laços e condições. Não depende de ler texto livre para decidir **qual ferramenta** chamar em seguida.
4. **Julgamentos isoláveis:** toda decisão que exige interpretação cabe numa pergunta fechada (sim ou não, escolha entre opções, nota), com a entrada bem definida (um e-mail, um evento). Perguntas abertas ("escreva uma resposta") não são julgamento: viram uma chamada de LLM explícita e orçada dentro da rotina, marcada no cartão como "usa LLM".
5. **Testável:** as entradas gravadas permitem reproduzir a execução sem a rede.

**Medida de sucesso do compilador:** a fração de explorações elegíveis cuja rotina gerada passa nos próprios testes na primeira tentativa, e a fração de rotinas que rodam 30 dias sem conserto. Ambas são medidas no benchmark de rotinas (seção 7).

### 6.7 Privacidade e dados
- **Tudo fica local:** banco, memória, recibos e segredos ficam na máquina do usuário. Não existe servidor do Vigia.
- **O que sai da máquina:** apenas o que vai para o provedor de LLM e o backend de julgamento escolhidos. A UI mostra, em cada recibo, **o que foi enviado para qual provedor**.
- **Minimização:** a exploração envia ao LLM só o necessário (por exemplo, o assunto e o remetente antes do corpo). As rotinas compiladas não enviam nada ao LLM, exceto nos passos de julgamento ou nas chamadas "usa LLM" declaradas.
- **Redação:** segredos nunca entram em prompts (o corretor garante). Padrões sensíveis (cartões, documentos) são mascarados antes do envio, quando configurado.
- **Sem telemetria por padrão.** Relatório de erro só com consentimento explícito e conteúdo revisável antes do envio.
- **Apagar é apagar:** um comando remove todos os dados de uma conexão (e-mails em cache, memória derivada, recibos com conteúdo), mantendo só o esqueleto da auditoria.

### 6.5 Motor de políticas
- Toda chamada de capacidade, seja de exploração, de rotina ou de skill, passa por ele **antes** de executar. Não há caminho alternativo, porque as funções de capacidade são a única porta para o mundo.
- Avalia na ordem:
  1. capacidade declarada;
  2. regras do usuário;
  3. orçamento;
  4. classificação de risco (leitura, escrita reversível, irreversível).
- A decisão é **permitir**, **tornar reversível** (por exemplo, apagar vira lixeira), **pedir aprovação** ou **bloquear**.
- Regras em linguagem natural são compiladas pelo LLM para uma política declarativa (por exemplo, CEL) e **conferidas pelo usuário** na tela de Regras. A execução usa só a política compilada, determinística.
- As regras não passam pelo contexto do modelo para serem obedecidas. Um lembrete curto é incluído no prompt da exploração apenas para o modelo planejar melhor.

### 6.6 Modelo de ameaças (resumo)

| Ameaça | Defesa |
|---|---|
| Injeção de prompt num e-mail lido | Conteúdo externo é marcado como não confiável; capacidades mínimas; ações irreversíveis sempre aprovadas; o conteúdo não vira memória de instrução |
| Skill maliciosa | Roda no runtime isolado com capacidades mínimas declaradas; sem shell, sem rede fora da lista |
| Vazamento de chaves | O agente só vê apelidos; o corretor injeta o valor na saída e o valor nunca volta para o modelo |
| O agente edita as próprias regras ou o manifesto | Regras e manifestos só mudam por ação do usuário na UI ou no Telegram, com recibo |
| Gasto descontrolado ou loop | Orçamento checado antes de cada chamada; limites de passos por exploração |
| Interface exposta na internet | Escuta só em localhost por padrão; acesso remoto só via túnel autenticado; token obrigatório |
| Adulteração dos recibos | Eventos encadeados por hash; verificação na UI |

---

## 7. Engenharia e qualidade
- **Regras:** tudo em inglês no código; todo comportamento testado; sem excesso de comentários; nada com cara de gerado por IA.
- **Testes:**
  - unitários em todo pacote Go, com `-race`;
  - testes negativos do runtime (a rotina tenta acessar fora do manifesto e falha);
  - testes de contrato dos conectores com respostas gravadas;
  - testes de UI com Vitest e Playwright nos fluxos principais (primeira rotina, aprovação, desfazer, regra);
  - testes de ponta a ponta com um LLM simulado.
- **Cenários obrigatórios de regressão:** o incidente dos 200 e-mails (regra dada, contexto compactado, pedido destrutivo) precisa ser bloqueado; a injeção de prompt tentando exfiltrar chaves precisa falhar; a API que muda de formato precisa virar alerta e conserto, nunca silêncio.
- **Medição contínua:** um benchmark próprio de rotinas (taxa de sucesso, custo por execução, intervenções humanas), rodado a cada versão.

---

## 8. Roadmap completo

Do primeiro commit até um projeto maduro e sustentável. Sem prazos: a ordem importa, a duração não. Cada fase tem **objetivo, entregas (produto, UI, tecnologia), qualidade, gate, métricas e dependências**. Uma fase só começa quando o gate da anterior foi atingido. A exceção é o que está marcado como paralelo.

```text
            ┌──────────── Construir o núcleo ────────────┐
F0  Fundação e prova do compilador
F1  A rotina compilada ─────────────────────────────▶ v0.1  experimental
F2  Seguro por construção ──────────────────────────▶ v0.2
F3  A UI que conta a história ──────────────────────▶ v0.3
F4  Três usos perfeitos ────────────────────────────▶ v0.4
            ┌──────────── Abrir para o mundo ────────────┐
F5  Instalação sem terminal + migração ─────────────▶ v1.0  primeira versão estável
F6  Memória com origem + família + WhatsApp ────────▶ v1.1
F7  Galeria de rotinas e skills com capacidades ────▶ v1.2
F8  Mais canais e conectores ───────────────────────▶ v1.3
            ┌──────────── Ampliar e amadurecer ──────────┐
F9  Pequeno negócio completo ───────────────────────▶ v1.4
F10 Apps nativos (desktop e celular) ───────────────▶ v1.5
F11 Rede de proteção compartilhada + Guard ─────────▶ v2.0
F12 Longevidade: SDK, idiomas, auditoria, LTS ──────▶ v2.x
```

---

### F0 · Fundação e prova do compilador
- **Objetivo:** provar a aposta central antes de construir o produto, e montar a base.
- **Produto:** nenhum ainda. Só a prova.
- **Tecnologia:**
  - **Prova do compilador:** um script que recebe o registro de uma exploração (chamadas e respostas gravadas) e gera rotina, manifesto e testes, executados num runtime goja mínimo. Testado com **10 tarefas gravadas**: resumo matinal, lembrete de contas, triagem simples, alerta de e-mail de uma pessoa, resumo semanal da agenda, e variações.
  - Repositório novo, licença Apache-2.0, CI (Go e UI), ENGINEERING.md.
  - Esqueleto do binário: servidor HTTP e WebSocket, SQLite com eventos encadeados, cofre local de segredos.
  - Canal Telegram mínimo (enviar, receber, botões).
- **UI:** tokens de design, cores semânticas e o layout com barra lateral. Protótipos navegáveis (baixa fidelidade) das telas Rotinas, Momento da compilação e Recibos, para guiar o resto.
- **Qualidade:** testes da prova com as 10 tarefas; teste negativo (a rotina tenta sair do manifesto e falha).
- **Gate:** pelo menos 8 das 10 tarefas geram uma rotina que passa nos próprios testes na primeira tentativa, sem acessar nada fora do manifesto. Se não chegar lá, o plano é revisto **antes** de construir o resto.
- **Métricas:** taxa de compilação na primeira tentativa; custo de compilar cada tarefa.
- **Depende de:** nada.

### F1 · A rotina compilada → v0.1 (experimental)
- **Objetivo:** uma rotina real rodando todo dia, de verdade, no celular do Dener.
- **Produto:**
  - modo exploração com capacidades de leitura e envio para o dono;
  - compilador integrado ao produto;
  - agendador (horário fixo);
  - um passo de julgamento ("e-mail importante?");
  - vigia de saúde: toda falha vira aviso com o botão "refazer com o agente";
  - limite de gasto diário simples.
- **UI:** tela Rotinas (cartão com execuções, custo e capacidades) e o momento da compilação em versão simples.
- **Tecnologia:**
  - conectores Google Agenda (leitura), Gmail (leitura, via IMAP com senha de app) e Telegram (envio);
  - runtime goja com limites de tempo e memória;
  - backend de julgamento plugável (Jev ou LLM barato).
- **Qualidade:** testes de contrato dos conectores com respostas gravadas; teste de falha de API simulada que precisa gerar aviso; benchmark de rotinas (custo e falhas) contra a mesma tarefa com LLM em toda execução.
- **Gate:** 30 execuções diárias consecutivas do resumo matinal, das quais pelo menos 29 bem-sucedidas; custo médio de LLM de no máximo $0,01 por execução; e 100% das falhas geraram aviso no Telegram em até 5 minutos (verificado pelo log de eventos).
- **Métricas:** execuções bem-sucedidas / total; custo por execução; tempo entre a falha e o aviso.
- **Paralelo:** 5 conversas privadas com usuários (decisão pendente nº 5).

### F2 · Seguro por construção → v0.2
- **Objetivo:** poder ligar tarefas que escrevem e apagam sem medo.
- **Produto:**
  - motor de políticas completo (capacidades, regras, orçamento, risco), com as decisões permitir, tornar reversível, pedir aprovação e bloquear;
  - regras em linguagem natural compiladas e conferidas pelo usuário;
  - aprovações no Telegram (seção 5.5);
  - corretor de credenciais e domínios por capacidade;
  - limite de gasto com projeção mensal;
  - auditoria encadeada com argumentos;
  - desfazer para e-mail (lixeira, envio adiado);
  - autoconserto por diff, que precisa passar nos testes antigos e novos antes de ir para aprovação.
- **UI:** Precisa de você, Recibos (com desfazer e o destaque de bloqueios), e a versão básica de Regras e Custo.
- **Tecnologia:** políticas em CEL; cofre com criptografia local (chave derivada do sistema operacional: Keychain no macOS, libsecret no Linux); envio adiado com fila persistente.
- **Qualidade:** cenários obrigatórios de regressão (200 e-mails, injeção de prompt para exfiltrar chaves, loop contra o orçamento); fuzzing do avaliador de políticas; primeira revisão de segurança externa voluntária (pedido à comunidade de segurança quando o stand by acabar).
- **Gate:** três testes automatizados passam no CI: (1) o cenário dos 200 e-mails termina com zero e-mails apagados, um pedido de aprovação enviado e o desfazer restaurando tudo; (2) 50 tentativas diferentes de injeção de prompt para exfiltrar chaves resultam em zero chaves em qualquer saída; (3) um loop proposital termina com gasto menor ou igual ao orçamento.
- **Métricas:** ações irreversíveis sem aprovação (zero); desvio do orçamento (zero); falsos bloqueios por semana.

### F3 · A UI que conta a história → v0.3
- **Objetivo:** alguém que nunca viu o produto entende a diferença em 5 minutos.
- **Produto:** anti-incômodo (agrupamento, "tornar permanente" e classificador de risco).
- **UI:**
  - momento da compilação completo (animação, comparação de custo, testes e capacidades);
  - exploração ao vivo com aprovação inline;
  - Regras com "testar contra a última semana";
  - Custo completo;
  - detalhe da rotina com código, testes, histórico e diffs;
  - onboarding com a primeira rotina em menos de 5 minutos;
  - sistema de design completo (componentes documentados num catálogo visual);
  - acessibilidade AA, layout móvel e PWA instalável.
- **Tecnologia:** Playwright nos fluxos principais; testes visuais de regressão dos componentes.
- **Qualidade:** teste de usabilidade com 5 pessoas (pode ser feito em privado, sem divulgação).
- **Gate:** com um roteiro fixo e sessões gravadas, 4 de 5 participantes que nunca viram o produto completam as quatro tarefas sem nenhuma intervenção do moderador, cada uma em até 10 minutos cronometrados: criar a primeira rotina, achar o que o agente fez ontem, desfazer uma ação e criar uma regra.
- **Métricas:** tempo até a primeira rotina; pedidos de aprovação por dia (menos de 5); taxa de aceite do "Transformar em rotina".

### F4 · Três usos perfeitos → v0.4
- **Objetivo:** o Vigia substitui de verdade o que o Dener usava antes.
- **Produto:**
  - triagem de e-mail (rascunhos, arquivar, cancelar inscrições; sempre reversível);
  - um terceiro uso escolhido pelo Dener (lembrete de contas, resumo semanal ou acompanhamento de pacotes);
  - memória versionada;
  - atualização segura com snapshot e volta automática.
- **UI:** tela Memória; painel "o que mudou desde ontem" na tela Rotinas.
- **Tecnologia:** modelo local de julgamento (treinado com MLX, servido via Ollama ou llama.cpp) avaliado contra um conjunto rotulado; memória em git embutido (`go-git`).
- **Qualidade:** conjunto rotulado de julgamentos com pelo menos 300 exemplos por julgamento; calibração medida (a confiança dita bate com o acerto).
- **Gate:** durante 30 dias consecutivos, as três rotinas (resumo matinal, triagem de e-mail e o terceiro uso definido no início da fase) rodam em todos os horários agendados; o log de eventos mostra pelo menos 97% de execuções bem-sucedidas, menos de 2% de ações desfeitas, e zero falhas sem aviso.
- **Métricas:** desfazer usado em menos de 2% das ações; acerto e calibração dos julgamentos local e Jev.

### F5 · Instalação sem terminal e migração → v1.0 (primeira versão estável)
- **Objetivo:** qualquer pessoa instala e quem usa OpenClaw ou Hermes migra sem dor.
- **Produto:**
  - instalador para macOS (app de barra de menu que embute o binário) e script de uma linha para Linux, Raspberry Pi e VPS;
  - `vigia migrate openclaw|hermes`: skills, memória, canais e agendamentos, com o relatório "o que cada skill podia fazer e o que pode agora";
  - skills importadas rodam com capacidades mínimas e, quando se repetem, viram rotinas;
  - OAuth do Google guiado na UI (cliente próprio do usuário), substituindo o IMAP como padrão.
- **UI:** assistente de instalação; relatório de migração; página "O que o Vigia garante e o que não garante".
- **Tecnologia:** assinatura dos binários (notarização no macOS); canal de atualização estável e beta; esquema de eventos e de rotinas congelado em v1, com migrações testadas.
- **Qualidade:** matriz de testes em macOS, Ubuntu e Raspberry Pi OS; testes de migração com instalações reais anonimizadas do OpenClaw e do Hermes.
- **Gate:** 3 de 3 participantes não técnicos concluem a instalação em até 15 minutos sem editar arquivos, com as regras padrão ativas; 3 instalações reais (OpenClaw ou Hermes) migram com um comando e pelo menos 80% das skills funcionam sem ajuste. *(Precisa do fim do stand by.)*
- **Métricas:** tempo de instalação; tempo de migração; skills importadas que funcionam sem ajuste.

### F6 · Memória com origem, família e WhatsApp → v1.1
- **Objetivo:** o agente não pode ser envenenado pelo que lê, e serve a uma casa inteira.
- **Produto:**
  - origem em toda memória, com a de baixa confiança isolada das instruções;
  - vários usuários com papéis (dono, membro, convidado), credenciais e memória por pessoa;
  - regras por pessoa ("as crianças não podem comprar nada");
  - WhatsApp como segundo canal.
- **UI:** a memória mostra a origem e a confiança de cada fato; tela Pessoas; aprovação encaminhada para quem é responsável.
- **Tecnologia:** WhatsApp via API oficial (Cloud API) ou ponte local, a decidir pelo custo e pelo risco de bloqueio; isolamento de dados por usuário no banco.
- **Qualidade:** suíte de ataques de envenenamento de memória (e-mails e páginas maliciosas); testes de isolamento entre usuários.
- **Gate:** uma suíte de 50 ataques de envenenamento de memória (e-mails e páginas maliciosas) termina com 100% bloqueados; os testes de isolamento entre usuários passam 100% (nenhum acesso a dados ou credenciais de outro).
- **Métricas:** ataques da suíte bloqueados (100%); pedidos encaminhados ao responsável certo.

### F7 · Galeria de rotinas e skills com capacidades → v1.2
- **Objetivo:** as pessoas compartilham rotinas prontas com segurança, o oposto do ClawHub.
- **Produto:**
  - galeria de rotinas: código legível, manifesto de capacidades, testes e histórico;
  - instalar uma rotina mostra exatamente o que ela pode tocar;
  - assinatura dos autores e reprodutibilidade (o mesmo código gera o mesmo hash);
  - denúncias, com remoção por revisão;
  - skills do ecossistema (SKILL.md) adaptadas para rodar com capacidades mínimas.
- **UI:** navegador da galeria com filtros por capacidade ("mostrar só rotinas que não enviam nada para fora"), página de cada rotina e botão instalar com a revisão de capacidades.
- **Tecnologia:** índice estático num repositório git público (sem servidor próprio); assinatura com Sigstore; verificação no cliente.
- **Qualidade:** análise automática das rotinas enviadas (capacidades declaradas versus usadas); revisão humana para capacidades sensíveis.
- **Gate:** 50 rotinas publicadas pela comunidade; o verificador (que roda os testes de cada rotina num runtime instrumentado e compara as capacidades chamadas com as declaradas no manifesto) reporta zero divergências em 100% delas; e toda denúncia confirmada levou à remoção em até 72 horas, medida pelo histórico do repositório do índice. *(Precisa de comunidade.)*
- **Métricas:** rotinas instaladas; denúncias confirmadas; tempo de revisão.

### F8 · Mais canais e conectores → v1.3
- **Objetivo:** cobrir os lugares onde as pessoas já vivem, sem perder a qualidade.
- **Produto e tecnologia:**
  - canais: e-mail (conversar com o agente por e-mail), notas de voz (entrada e saída), Discord e Slack pessoais, e notificações push do PWA;
  - conectores: veja o catálogo na seção 10;
  - **SDK de conectores:** um conector é um pacote que declara as capacidades que oferece, com os níveis de risco de cada uma e testes de contrato obrigatórios.
- **UI:** tela Conexões com catálogo, estado e capacidades de cada conector.
- **Qualidade:** cada conector só entra com testes de contrato, capacidades classificadas por risco e um exemplo de rotina.
- **Gate:** 15 conectores com testes de contrato verdes contra a API real em 4 semanas consecutivas.
- **Métricas:** conectores ativos por usuário; falhas de conector por semana.

### F9 · Pequeno negócio completo → v1.4
- **Objetivo:** o Carlos (persona 2) roda o backoffice pelo celular.
- **Produto:**
  - orçamento a partir de foto ou áudio, fatura em PDF e acompanhamento de clientes (um CRM leve);
  - lembretes de cobrança (sempre aprovados antes de enviar);
  - links de pagamento gerados, **nunca** pagamentos executados pelo agente;
  - exportação contábil (CSV e formatos comuns).
- **UI:** painel do negócio (clientes, orçamentos, faturas, a receber) e modelos de documento editáveis.
- **Tecnologia:** geração de PDF local; OCR ou visão para fotos; modelos de documento como rotinas.
- **Qualidade:** testes com documentos reais anonimizados; nenhuma mensagem para cliente sem aprovação (auditoria).
- **Gate:** 3 pequenos negócios usando por 30 dias, cada um com pelo menos 20 documentos gerados, e a auditoria mostrando zero mensagens para clientes sem aprovação. *(Precisa do fim do stand by.)*
- **Métricas:** documentos gerados; tempo economizado declarado; mensagens enviadas sem aprovação (zero).

### F10 · Apps nativos → v1.5
- **Objetivo:** aprovação e controle em um toque, mesmo sem Telegram.
- **Produto e UI:**
  - app de desktop (Tauri) com ícone na barra, notificações e início automático;
  - app de celular complementar (iOS e Android) focado em três coisas: aprovações, recibos e saúde;
  - pareamento seguro com o Vigia de casa (QR code), sem servidor intermediário.
- **Tecnologia:** comunicação cifrada ponta a ponta via túnel (por exemplo, Tailscale ou um relé cego opcional); notificações push.
- **Qualidade:** testes de pareamento e revogação; auditoria das permissões dos apps.
- **Gate:** em 20 aprovações medidas, a mediana do aviso ao toque é menor que 3 segundos; o pareamento e a revogação passam nos testes automatizados em iOS, Android, macOS, Windows e Linux.
- **Métricas:** tempo de aprovação; uso do app contra o Telegram.
- **Observação:** o Jev avaliou apps nativos como de baixa prioridade para o público inicial (0,68 e 0,83). Por isso vêm tarde, quando já houver usuários pedindo.

### F11 · Rede de proteção compartilhada e Guard → v2.0
- **Objetivo:** cada usuário protege os outros, e quem ficou no OpenClaw ou no Hermes também se beneficia.
- **Produto:**
  - rede opcional: assinaturas de skills maliciosas, domínios de exfiltração e padrões de ações perigosas, compartilhados de forma anônima;
  - plugin **Guard** para OpenClaw e Hermes, usando os ganchos oficiais (`before_tool_call`, `pre_tool_call`) com o mesmo motor de políticas.
- **UI:** "Proteção da comunidade" nos Recibos ("bloqueado: skill marcada por 12 usuários").
- **Tecnologia:** lista assinada distribuída como arquivo estático; contribuição com privacidade diferencial; sem conteúdo pessoal.
- **Qualidade:** processo de contestação de falsos positivos; testes de contrato do Guard contra cada versão suportada do OpenClaw e do Hermes.
- **Gate:** pelo menos 100 instalações que enviaram ou baixaram a lista assinada nos últimos 30 dias (contadas pelo índice público, sem identificar ninguém); pelo menos 1 bloqueio registrado num recibo cuja regra veio da lista compartilhada; menos de 5% das entradas da lista contestadas e removidas como falso positivo; e o Guard passa nos testes de contrato das duas versões mais recentes do OpenClaw e do Hermes. *(Precisa de comunidade.)*
- **Métricas:** ameaças bloqueadas pela rede; falsos positivos contestados.

### F12 · Longevidade → v2.x
- **Objetivo:** o projeto sobrevive e melhora sem depender de uma pessoa só.
- **Entregas:**
  - SDK público e estável para conectores, julgamentos e canais;
  - idiomas: português e inglês desde a F3; espanhol e outros pela comunidade;
  - auditoria de segurança independente, com relatório publicado;
  - versões de suporte longo (LTS) com correções de segurança;
  - governança aberta: mantenedores além do fundador, processo de RFC e código de conduta;
  - documentação completa: usuário, desenvolvedor de conectores e modelo de ameaças.
- **Gate:** pelo menos 3 mantenedores além do fundador, cada um com 10 ou mais PRs revisados por mês durante 3 meses; uma auditoria independente publicada; e correções de segurança lançadas em até 7 dias após o relato na versão LTS.
- **Métricas:** colaboradores ativos; tempo de resposta a relatos de segurança; versões LTS mantidas.

---

## 9. Mapa completo de funcionalidades

Todas as 46 funcionalidades avaliadas pelo Jev, com a fase em que entram. Os números são a prioridade calculada (`0,45×valor + 0,35×diferenciação − 0,2×risco`).

| Fase | Funcionalidades |
|---|---|
| F0 | Prova do compilador · base do binário · Telegram mínimo · tokens de design |
| F1 | Compilador de rotinas (1,91) · Agendador (1,57) · Resumo matinal (1,41) · Modo exploração (1,08) · Canal Telegram (1,39) · Capacidades (1,90) · Limite de gasto diário (2,13) · Julgamentos calibrados (1,73) · Vigia de saúde (1,78) · UI Rotinas mínima (1,85) · Qualquer provedor por API (0,42) · Binário único (1,40) |
| F2 | Motor de regras (1,96) · Aprovação no celular (1,81) · Corretor de credenciais (1,84) · Auditoria com argumentos (1,91) · Desfazer (1,84) · Autoconserto (1,66) · Sandbox por padrão (1,49) · UI Recibos (2,00) · UI Precisa de você (1,41) · UI Regras e Custo básicas |
| F3 | UI Custo completa (1,82) · UI Regras com teste (1,69) · UI Exploração ao vivo (1,62) · UI Onboarding (1,51) · Anti-incômodo (1,52) · UI PWA (0,85) |
| F4 | Triagem de e-mail (1,49) · Memória versionada (1,53) · UI Memória (1,39) · Modelo local de julgamento (1,68) · Atualização segura (1,31) |
| F5 | Migração (1,52) · Importar skills do ecossistema (0,70) |
| F6 | Memória com origem (1,69) · Modo família (0,90) · Canal WhatsApp (0,81) |
| F7 | Galeria de rotinas (nova, derivada da F5) |
| F8 | Canal e-mail (0,78) · Voz (0,95) · Assinatura do Claude ou do ChatGPT como cérebro (0,75) · SDK de conectores |
| F9 | Backoffice de pequeno negócio (1,26) |
| F10 | App desktop (0,83) · App de celular (0,68) |
| F11 | Rede de proteção compartilhada (1,14) · Plugin Guard (1,08) |

---

## 10. Catálogo de conectores por fase

Cada conector declara capacidades com nível de risco: 🟢 leitura, 🟡 escrita reversível, 🔴 irreversível.

| Fase | Conector | Capacidades |
|---|---|---|
| F1 | Google Agenda | 🟢 ler eventos |
| F1 | Gmail (IMAP) | 🟢 ler mensagens |
| F1 | Telegram | 🟡 enviar ao dono |
| F1 | HTTP (lista de domínios) | 🟢 GET em domínios declarados |
| F2 | Gmail | 🟡 arquivar, marcar, rascunho · 🔴 enviar, apagar (tornados reversíveis: envio adiado, lixeira) |
| F4 | Google Agenda | 🟡 criar evento (reversível) |
| F4 | Arquivos locais | 🟢 ler pastas declaradas · 🟡 escrever com snapshot |
| F5 | Gmail (OAuth), Outlook e IMAP genérico | leitura e escrita como acima |
| F6 | WhatsApp | 🟡 enviar ao dono · 🔴 enviar a terceiros (sempre aprovado) |
| F8 | Notion, Obsidian | 🟢 ler · 🟡 escrever com histórico |
| F8 | Todoist, Google Tasks, Apple Lembretes | 🟢 ler · 🟡 criar e concluir |
| F8 | Home Assistant | 🟢 ler estados · 🟡 ações reversíveis · 🔴 ações físicas críticas (fechaduras, alarme) sempre aprovadas |
| F8 | GitHub | 🟢 ler issues e PRs · 🟡 comentar |
| F8 | RSS e páginas web | 🟢 ler |
| F8 | Clima, rastreio de encomendas, câmbio | 🟢 ler |
| F9 | Planilhas (Google Sheets, CSV) | 🟢 ler · 🟡 escrever com histórico |
| F9 | Geração de PDF e OCR local | 🟢 local |
| F9 | Links de pagamento (Stripe, Mercado Pago) | 🟡 gerar link · pagamentos **nunca** executados pelo agente |

**Regra permanente:** nenhum conector de banco ou corretora com capacidade de mover dinheiro. Apenas leitura, e só depois da F12, com auditoria.

---

## 11. Operação do projeto

**Versões e lançamentos**
- Versionamento semântico. Esquemas (eventos, rotinas, manifestos, políticas) congelados em v1 e só crescem.
- Canais estável e beta. Notas de versão em linguagem humana ("o que muda para você").
- Binários assinados e reprodutíveis; notarização no macOS.

**Atualizações**
- Snapshot automático antes de cada atualização e volta automática se o serviço não responder bem (F4).
- Atualização nunca automática sem consentimento; aviso pelo Telegram com um resumo.

**Suporte e relatos**
- Relato de bug pela própria UI, que monta um pacote anonimizado e revisável (sem conteúdo pessoal) antes de enviar.
- Modelos de issue no GitHub; triagem semanal; o Jev pode ajudar a classificar issues (duplicata, bug, pedido), sempre com revisão humana.
- Discussões e perguntas num fórum público (GitHub Discussions), não em chat efêmero.

**Segurança**
- SECURITY.md com canal privado de relato e prazo público de resposta.
- Correções de segurança publicadas com aviso e versão LTS (F12).
- Cenários de regressão de segurança no CI (seção 7).

**Documentação**
- Guia do usuário (em português e inglês), referência de conectores, guia do SDK e modelo de ameaças.
- Página honesta "O que o Vigia garante e o que não garante".

**Termos de uso e riscos legais**
- **WhatsApp:** usar só a API oficial (Cloud API). Pontes não oficiais arriscam o banimento do número do usuário e ficam fora do projeto.
- **Gmail e Google:** respeitar a política de dados de usuário das APIs do Google; o cliente OAuth é do próprio usuário, então os dados nunca passam por um app do projeto. A verificação do Google só entra se um dia houver um app compartilhado.
- **Telegram:** bots seguem os termos da plataforma; o bot é do próprio usuário (criado com o BotFather no onboarding).
- **Provedores de LLM:** o usuário usa as próprias chaves e aceita os termos do provedor. O Vigia mostra, em cada recibo, o que foi enviado para qual provedor.
- **Responsabilidade:** licença Apache-2.0 sem garantia, e a página "O que o Vigia garante e o que não garante" deixa claro que decisões irreversíveis são sempre do usuário.
- **Galeria e rede de proteção (F7 e F11):** política de conteúdo, processo de remoção e contestação publicados antes de abrir.

**Idiomas**
- Interface e mensagens do Telegram em português e inglês desde a F3; estrutura pronta para a comunidade traduzir.

---

## 12. Comunidade e sustentabilidade (tudo gratuito)

- **Licença:** Apache-2.0 para todo o código. Nenhuma versão paga, nenhum recurso bloqueado.
- **Custo zero para operar o projeto:** sem servidores próprios. Galeria e rede de proteção são arquivos estáticos em repositórios públicos, e o usuário paga só o próprio LLM (ou usa um modelo local).
- **Sustentação voluntária:** GitHub Sponsors e Open Collective, com gastos transparentes (por exemplo, a auditoria de segurança). Sem investidores que exijam monetização.
- **Governança:** fundador como mantenedor principal até a F11; na F12, mantenedores adicionais, processo de RFC e decisões abertas.
- **Chegada ao público (quando o stand by acabar):** relatório técnico honesto (o incidente dos 200 e-mails reproduzido e bloqueado; custo de rotina contra LLM toda vez), demonstração em vídeo do momento da compilação, e posts nos lugares onde as personas estão (Hacker News, r/openclaw, comunidades brasileiras de tecnologia).

---

## 13. Métricas

| Métrica | Alvo |
|---|---|
| Custo de LLM por execução de rotina | Próximo de zero; comparado à mesma tarefa com LLM em toda execução |
| Execuções com falha silenciosa | Zero |
| Execuções bem-sucedidas em 30 dias | Mais de 99% |
| Ações irreversíveis sem aprovação | Zero |
| Pedidos de aprovação por dia | Menos de 5 |
| Desvio entre gasto e orçamento | Zero ou negativo |
| Explorações bem-sucedidas que viram rotina | Mais de 50% das tarefas repetidas |
| Tempo até a primeira rotina | Menos de 5 minutos |
| Tempo para migrar do OpenClaw ou do Hermes | Menos de 5 minutos |

---

## 14. Riscos e respostas

| Risco | Resposta |
|---|---|
| **Compilar tarefas em rotinas confiáveis não funcionar bem** (a aposta central) | É a F1, com um caso só e gate de 30 dias. Se falhar, o produto ainda vale como "agente seguro por construção", mas perde o grande diferencial, e o plano é reavaliado antes de seguir |
| Conectores do Google exigem verificação do app para escopos sensíveis do Gmail | Cada usuário cria o próprio cliente OAuth (guiado na UI) ou usa IMAP com senha de app; avaliar a verificação só quando houver usuários |
| Poucas tarefas se repetem | Tarefas únicas ficam na exploração, já protegida. O foco inicial são justamente os usos repetitivos (resumo, triagem) |
| APIs externas mudam | Vigia de saúde detecta e o autoconserto propõe um diff; nunca silêncio |
| Aprovação demais vira incômodo | Anti-incômodo e classificador de risco; meta de menos de 5 pedidos por dia |
| Comunidades enormes dos concorrentes | Não competir em quantidade: conquistar quem se queimou, com migração em 1 comando |
| Eles copiam a ideia | Exigiria reescrever o núcleo deles (tudo passa pelo LLM) e criar segurança por capacidades; mesmo assim, velocidade e foco |
| Dependência do Jev para julgamentos | Backend plugável; o modelo local e o LLM barato garantem que o produto funciona sem ele |
| Falso senso de segurança | Documentar com honestidade o que cada camada garante e o que não garante |

---

## 15. Decisões pendentes (do Dener)

1. **Nome definitivo** (checar marca e domínio).
2. **Backend de julgamento padrão:** Jev (melhor calibração, exige chave) ou modelo local (gratuito, offline, menos preciso). Recomendação: **Jev quando houver chave, modelo local como padrão gratuito** a partir da F4.
3. **Linguagem das rotinas:** JavaScript no goja (os LLMs escrevem JS bem; Go puro) ou Starlark. Recomendação: **JavaScript no goja**, a mudança que o Jev mais recomendou para o MVP.
4. **Linguagem das políticas compiladas:** CEL (padrão do Google, simples e seguro) ou uma DSL própria. Recomendação: **CEL**.
5. **Conversas privadas com usuários durante a F1:** o Jev apontou "falar com usuários reais" como a peça que mais falta no plano (0,84). Isso é diferente da divulgação pública, que está em stand by: seriam 5 conversas privadas com pessoas que usaram OpenClaw ou Hermes, mostrando o momento da compilação. Recomendação: **fazer**, sem anúncio público.
6. **Acesso ao Gmail:** IMAP com senha de app (simples, funciona hoje) ou OAuth com cliente próprio de cada usuário (mais seguro, configuração mais chata). Recomendação: **IMAP na F1 e OAuth guiado na F3**.
7. **Quando sair do stand by** da divulgação na comunidade (a F5 e a F7 dependem disso).

---

## 16. Visão de longo prazo

**Onde o Vigia quer chegar:** ser o jeito padrão e confiável de ter um agente pessoal. Não pelo número de integrações, mas porque é o único em que as pessoas **confiam para deixar rodando sozinho**.

- **Do agente que conversa para o agente que vira software:** com o tempo, a maior parte do que o agente faz por uma pessoa são rotinas compiladas, legíveis e baratas. O LLM fica para o que é novo.
- **Uma biblioteca aberta de rotinas auditáveis:** cada rotina com código, testes e capacidades declaradas, construída pela comunidade (F7). É o oposto de um mercado de skills que rodam com poder total.
- **Proteção que cresce com o uso:** a rede compartilhada (F11) é o ativo que nenhum agente fabrica sozinho.
- **Modelos de julgamento pequenos e locais:** cada vez mais decisões rodam de graça, offline, no computador da pessoa, com os dados dela.
- **O que o Vigia nunca vai fazer:** vender dados, cobrar por segurança, mover dinheiro sozinho, ou esconder o que o agente fez.

---

## Apêndice A — Como o Jev foi usado

- **Priorização:** 46 funcionalidades candidatas, cada uma avaliada em quatro perguntas de escala 0–3 (valor para o usuário-alvo, diferenciação contra o OpenClaw e o Hermes, risco para um fundador sozinho, necessidade para o MVP), com o produto, o público e os concorrentes como contexto. Script: `jev/prioritize.py`; resultados em `jev/features.json`.
- **Decisões de UI:** quatro perguntas de escolha e sim/não (superfície principal, tela principal, momento "uau", necessidade de app desktop). Resultados em `jev/ui.json`.
- **Revisão do plano:** cada seção avaliada quanto a concretude, decisões em aberto e consistência com o MVP (`jev/review.py`, `jev/review.json`); depois cada fase do roadmap avaliada quanto a concretude, gate verificável e posição na sequência, e o plano inteiro quanto a completude (`jev/review_full.py`, `jev/review_full.json`). Resumo no Apêndice B.
- **Limites:** o Jev dá julgamentos calibrados sobre o texto que recebe. Ele não substitui usuários reais. A validação com pessoas continua sendo o próximo passo quando o stand by acabar.

## Apêndice B — Resultado da revisão do Jev

Três rodadas de revisão, com correções entre elas.

| Pergunta sobre o plano inteiro | 1ª rodada | Final | Leitura |
|---|---|---|---|
| O resultado seria melhor que OpenClaw e Hermes para o público-alvo? (0–3) | 2,75 | 2,75 | "Claramente melhor nas dores que fazem as pessoas desistir" |
| A ordem das fases está correta? | 0,88 | 0,88 | Sim |
| A UI deixa os dois momentos "uau" visíveis? | 0,88 | 0,89 | Sim |
| A F0 e a F1 cabem para uma pessoa com agentes de código? | 0,50 | 0,52 | Incerto: o Jev não tem como saber, e a incerteza está concentrada no compilador (0,81 como maior ameaça) |
| Maior risco | compilador (1,00) | compilador (1,00) | Por isso a F0 começa com a prova do compilador e tem um gate que para tudo se falhar |
| Peça que mais falta | falar com usuários (0,84) | falar com usuários (0,82) | Virou a decisão pendente nº 5 (conversas privadas, sem divulgação) |

**Ações tomadas a partir da revisão:**
1. A F0 passou a começar pela **prova do compilador** (10 tarefas gravadas, gate de 8/10).
2. Foi criada a **especificação do que é compilável** (6.4.1).
3. A F1 foi **enxugada**: sem autoconserto automático (vai para a F2), conectores mínimos, UI mínima.
4. A tabela do MVP foi **dividida em F1 e F2**, porque o resumo matinal não tem ação irreversível.
5. O runtime das rotinas passou a ser o **goja**, a mudança com mais impacto segundo o Jev (0,66 contra 0,13 da segunda opção).
6. Foi criada a seção de **privacidade e dados** (6.7).
7. As **conversas privadas com usuários** entraram como decisão pendente.

**Sobre a incerteza no escopo:** uma probabilidade perto de 0,5 significa que o Jev vê argumentos parecidos para os dois lados, não que ele acha que não cabe. A resposta certa não é mais texto no plano, é **executar a prova do compilador da F0**, que resolve a dúvida com dados.

### Revisão do plano completo (roadmap F0–F12)

| Pergunta | Antes das correções | Depois |
|---|---|---|
| É um plano completo, do primeiro commit a um produto maduro? | 0,96 | 0,96 |
| As fases pós-v1.0 são tão detalhadas quanto as iniciais? (0–3) | 2,33 | 2,57 |
| A UI está coberta em todo o ciclo de vida? | 0,80 | 0,73 |
| Maior lacuna | falar com usuários (0,97) | falar com usuários (0,94) |

**Gates verificáveis por fase** (probabilidade de o gate poder ser checado sem julgamento subjetivo):

| Fase | Antes | Depois |
|---|---|---|
| F1 | 0,47 | 0,80 |
| F2 | 0,56 | 0,80 |
| F3 | 0,45 | 0,63 |
| F4 | 0,24 | 0,61 |
| F5 | 0,51 | 0,58 |
| F6 | 0,54 | 0,78 |
| F7 | 0,36 | 0,49 |
| F9 | 0,48 | 0,67 |
| F11 | 0,48 | 0,53 |

**O que foi feito:** todos os gates passaram a ter números e fonte de verificação (log de eventos, testes no CI, sessões gravadas, histórico do repositório); entrou o plano de pesquisa contínua e privada (2.1); entrou a seção de termos de uso e riscos legais (11).

**O que continua em aberto, com honestidade:**
- **Falar com usuários** segue como a maior lacuna. O plano de pesquisa existe, mas depende da decisão nº 5, e o Jev só vê o texto, não a execução.
- **Os gates da F7 e da F11** dependem de comunidade, e por isso nunca ficam totalmente sob controle do projeto.
- **A F11 fica tarde na sequência** (0,70 na avaliação de posição). O Guard poderia vir antes, como porta de entrada para usuários do OpenClaw e do Hermes, mas a priorização deu a ele valor baixo para o público inicial (1,08). Fica como decisão a revisitar depois da v1.0.


## Apêndice C — Decisões técnicas de F6 a F12 (Jev)

Tomadas com `tools/jev/decisions_later.py` (resultado em `decisions_later.json`), antes de cada fase:

| Decisão | Escolha | Probabilidade | Alternativa mais próxima |
|---|---|---|---|
| WhatsApp | API oficial (Cloud API) | 0,56 | ponte local (0,26), com risco de bloqueio do número |
| Assinatura da galeria | chaves Ed25519 dos autores | 0,74 | Sigstore (0,25) |
| Voz | whisper.cpp local | 0,51 | API do provedor (0,26) |
| PDF | biblioteca em Go puro | 0,84 | Typst (0,09) |
| Fotos | Tesseract local | 0,51 | Tesseract com fallback para o modelo (0,35) |
| Avisos no celular | Telegram como canal de push | 0,53 | ntfy (0,44) |
| SDK de conectores | processos MCP por stdio | 0,47 | pacotes Go compilados (0,43) |
| Rede de proteção | arquivo estático assinado num repositório | 0,95 | API própria (0,05) |

Duas mudanças em relação ao plano original, ambas pela decisão acima: a galeria usa chaves Ed25519 em vez de Sigstore, e o app de celular usa o Telegram (e o canal genérico) para avisos instantâneos em vez de push próprio.

O modelo local de julgamento foi medido antes de virar padrão: sozinho acerta 85%; em cascata com o Jev, enviando só os 22% de respostas incertas, acerta 92% (Jev sozinho: 94,5%). Por isso o backend local roda em cascata.
