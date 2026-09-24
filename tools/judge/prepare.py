#!/usr/bin/env python3
"""Split data/all.jsonl into train/valid/test for mlx-lm, holding out whole
question types so the test measures generalization to new judgments."""
import json, random
from collections import defaultdict
from pathlib import Path

SYSTEM = "You judge one item against one yes/no question for a personal assistant. Answer with exactly one word: yes or no."
UNSEEN = {"Is this email a shipping or delivery update?", "Este evento é pessoal (não de trabalho)?", "Is this support ticket about billing?"}

def prompt(r):
    return f"Question: {r['question']}\nItem: {json.dumps(r['item'], ensure_ascii=False)}"

def chat(r):
    return {"messages": [{"role": "system", "content": SYSTEM}, {"role": "user", "content": prompt(r)}, {"role": "assistant", "content": "yes" if r["label"] else "no"}]}

def main():
    rows = [json.loads(l) for l in open("data/all.jsonl")]
    rng = random.Random(11)
    by_q = defaultdict(list)
    for r in rows:
        by_q[r["question"]].append(r)
    train, valid, test = [], [], []
    for q, items in by_q.items():
        rng.shuffle(items)
        if q in UNSEEN:
            for r in items:
                test.append({**r, "unseen": True})
            continue
        n = len(items)
        test += [{**r, "unseen": False} for r in items[: n // 7]]
        valid += items[n // 7 : n // 7 + n // 10]
        train += items[n // 7 + n // 10 :]
    Path("mlx").mkdir(exist_ok=True)
    for name, split in [("train", train), ("valid", valid)]:
        with open(f"mlx/{name}.jsonl", "w") as f:
            for r in split:
                f.write(json.dumps(chat(r), ensure_ascii=False) + "\n")
    with open("data/test.jsonl", "w") as f:
        for r in test:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    yes = sum(r["label"] for r in train)
    print(f"train {len(train)} ({yes} yes) valid {len(valid)} test {len(test)} ({sum(r['unseen'] for r in test)} on unseen questions)")

if __name__ == "__main__":
    main()
