#!/usr/bin/env python3
"""Simulate a cascade: the local judge answers, and only items it is unsure
about (p inside a band around 0.5) go to a stronger judge.
Usage: cascade.py LOCAL_PREDS STRONG_PREDS"""
import json, sys
from evaluate import metrics

test = [json.loads(l) for l in open("data/test.jsonl")]
local, strong = json.load(open(sys.argv[1])), json.load(open(sys.argv[2]))
print("band        escalated  accuracy  brier  ece    auc")
for w in (0, 0.1, 0.2, 0.3, 0.4, 0.5):
    ps = [s if abs(l - 0.5) < w else l for l, s in zip(local, strong)]
    esc = sum(abs(l - 0.5) < w for l in local) / len(local)
    m = metrics([(p, int(r["label"])) for p, r in zip(ps, test)])
    print(f"±{w:<10} {esc:>8.0%}  {m['accuracy']:>8}  {m['brier']:>5}  {m['ece']:>5}  {m['auc']}")
