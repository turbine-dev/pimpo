#!/usr/bin/env python3
"""Check the duplicate-fact question used by memory organizing (internal/app/memoryorganize.go)
on labeled pairs. Writes calibrate_memory.json."""
import json
from concurrent.futures import ThreadPoolExecutor
from prioritize import call

Q = ("Do `item.a` and `item.b` record the same fact, possibly in different words? "
     "Answer no if they differ in a date, day, place, name, number or other detail, or if one contradicts or updates the other. Answer about `item`.")
PAIRS = [
    ("Academia às terças", "Vou à academia às terças", 1),
    ("Reunião com a Ana dia 12", "Reunião com a Ana dia 19", 0),
    ("Minha irmã se chama Ana", "A irmã se chama Ana Paula", 0),
    ("Prefiro café sem açúcar", "Gosto de café sem açúcar", 1),
    ("Moro em São Paulo", "Moro em Lisboa", 0),
    ("Alergia a amendoim", "Tenho alergia a amendoim", 1),
    ("Não gosto de reuniões antes das 10h", "Evito reuniões antes das 10h", 1),
    ("O carro é um Civic 2019", "O carro é um Civic", 0),
    ("Meu aniversário é 3 de maio", "Faço aniversário em 3 de maio", 1),
    ("Trabalho na Beevo", "Trabalhava na Beevo", 0),
]

def one(p):
    r = call({"item": {"a": p[0], "b": p[1]}}, {"q": {"type": "noul", "instructions": Q}})
    return {"a": p[0], "b": p[1], "same": p[2], "p": r["answers"]["q"]["noul"]}

with ThreadPoolExecutor(10) as ex:
    rows = list(ex.map(one, PAIRS))
json.dump(rows, open("calibrate_memory.json", "w"), indent=1, ensure_ascii=False)
dups = [r["p"] for r in rows if r["same"]]
diffs = [r["p"] for r in rows if not r["same"]]
print(f"rewordings min {min(dups):.2f}, different max {max(diffs):.2f}")
