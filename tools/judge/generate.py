#!/usr/bin/env python3
"""Generate a labeled set of yes/no judgments about realistic items, with
Claude Code headless. Output: data/all.jsonl with {question, item, label}.

Each batch asks for one question and a balanced mix of items, including
hard cases, so the local model learns the question-conditioned task rather
than one fixed classifier."""
import json, random, subprocess, sys, concurrent.futures as cf
from pathlib import Path

QUESTIONS = [
  ("email", "Is this email important for the owner today?"),
  ("email", "Is this email a newsletter or promotion?"),
  ("email", "Does this email need a reply from the owner?"),
  ("email", "Is this email about a bill or payment due?"),
  ("email", "Is this email from a real person (not automated)?"),
  ("email", "Is this email urgent (needs action within 24 hours)?"),
  ("email", "Is this email a security alert about the owner's account?"),
  ("email", "Is this email related to work?"),
  ("email", "Is this email a shipping or delivery update?"),
  ("email", "Is this email a meeting invitation or scheduling request?"),
  ("email", "Este e-mail é importante para hoje?"),
  ("email", "Este e-mail é uma newsletter ou promoção?"),
  ("email", "Este e-mail precisa de resposta?"),
  ("email", "Este e-mail é sobre uma conta ou cobrança a pagar?"),
  ("event", "Is this calendar event a work meeting?"),
  ("event", "Does this event need preparation beforehand?"),
  ("event", "Este evento é pessoal (não de trabalho)?"),
  ("event", "Is this event something the owner can skip?"),
  ("json", "Does this weather forecast call for an umbrella?"),
  ("json", "Is this package tracking status a problem that needs attention?"),
  ("json", "Is this news headline relevant to someone working in software?"),
  ("text", "Is this message a complaint from a customer?"),
  ("text", "Esta mensagem de cliente pede um orçamento?"),
  ("text", "Is this support ticket about billing?"),
]

SHAPES = {
  "email": '{"id","from","from_name","subject","snippet","date","labels":[...],"replied":bool}',
  "event": '{"id","title","start","end","location","attendees":[...],"calendar"}',
  "json": 'a small realistic JSON object from the relevant API',
  "text": '{"from","text"}',
}

SCHEMA = json.dumps({"type": "object", "required": ["items"], "properties": {"items": {"type": "array", "items": {
  "type": "object", "required": ["item", "label", "hard"], "properties": {"item": {"type": "object"}, "label": {"type": "boolean"}, "hard": {"type": "boolean"}}}}}})

def batch(kind, question, n, seed):
  prompt = (f"Create {n} realistic, varied {kind} items (as JSON objects shaped like {SHAPES[kind]}) and label each for the question: "
            f"\"{question}\". About half must be yes and half no. Include {n//4} hard cases near the boundary (mark hard=true). "
            f"Vary senders, languages (mostly Portuguese and English), tone, length and context. Use fictional people and companies. Seed {seed}.")
  out = subprocess.run(["claude", "-p", prompt, "--output-format", "json", "--tools", "", "--no-session-persistence", "--model", "sonnet",
                        "--json-schema", SCHEMA, "--max-budget-usd", "0.5"], capture_output=True, text=True, timeout=600, cwd="/")
  try:
    data = json.loads(out.stdout.strip().splitlines()[-1])
    return [{"question": question, "kind": kind, "item": it["item"], "label": bool(it["label"]), "hard": bool(it.get("hard"))} for it in data["structured_output"]["items"]], data.get("total_cost_usd", 0)
  except Exception as e:
    print("batch failed:", question, e, out.stderr[-300:], file=sys.stderr)
    return [], 0

def main():
  per = int(sys.argv[1]) if len(sys.argv) > 1 else 3
  Path("data").mkdir(exist_ok=True)
  jobs = [(k, q, 24, s) for k, q in QUESTIONS for s in range(per)]
  rows, cost = [], 0.0
  with cf.ThreadPoolExecutor(6) as ex:
    for r, c in ex.map(lambda j: batch(*j), jobs):
      rows += r; cost += c
  random.Random(7).shuffle(rows)
  with open("data/all.jsonl", "w") as f:
    for r in rows:
      f.write(json.dumps(r, ensure_ascii=False) + "\n")
  print(f"{len(rows)} examples, ${cost:.2f}")

if __name__ == "__main__":
  main()
