#!/usr/bin/env python3
"""Compare judges on data/test.jsonl: accuracy, Brier score, expected
calibration error and AUC, split by seen and unseen question types.
Usage: evaluate.py local MODEL [ADAPTER] | claude MODEL | jev   (writes results/<name>.json)"""
import json, subprocess, sys, os, urllib.request, concurrent.futures as cf
from pathlib import Path

def metrics(pairs):
    if not pairs:
        return {}
    n = len(pairs)
    acc = sum((p >= 0.5) == y for p, y in pairs) / n
    brier = sum((p - y) ** 2 for p, y in pairs) / n
    bins = [[] for _ in range(10)]
    for p, y in pairs:
        bins[min(int(p * 10), 9)].append((p, y))
    ece = sum(len(b) / n * abs(sum(p for p, _ in b) / len(b) - sum(y for _, y in b) / len(b)) for b in bins if b)
    pos = [p for p, y in pairs if y]; neg = [p for p, y in pairs if not y]
    auc = sum((a > b) + 0.5 * (a == b) for a in pos for b in neg) / (len(pos) * len(neg)) if pos and neg else None
    return {"n": n, "accuracy": round(acc, 3), "brier": round(brier, 3), "ece": round(ece, 3), "auc": round(auc, 3) if auc else None}

def claude_p(model, r):
    schema = '{"type":"object","required":["p"],"properties":{"p":{"type":"number","minimum":0,"maximum":1}}}'
    out = subprocess.run(["claude", "-p", f"Question: {r['question']}\nItem: {json.dumps(r['item'], ensure_ascii=False)}", "--output-format", "json", "--tools", "",
                          "--no-session-persistence", "--model", model, "--json-schema", schema, "--max-budget-usd", "0.05",
                          "--system-prompt", "You judge one item. Reply with p, the probability (0 to 1) that the answer to the question is yes. Be calibrated."],
                         capture_output=True, text=True, timeout=180, cwd="/")
    try:
        return float(json.loads(out.stdout.strip().splitlines()[-1])["structured_output"]["p"])
    except Exception:
        return 0.5

def jev_p(r):
    body = json.dumps({"model": "jev-latest", "state": {"item": r["item"]}, "questions": {"q": {"type": "noul", "instructions": r["question"] + " Answer about `item`."}}}).encode()
    req = urllib.request.Request("https://api.typesafe.ai/v1/systemone", data=body, headers={"Authorization": "Bearer " + os.environ["TYPESAFE_API_KEY"], "Content-Type": "application/json"})
    for _ in range(3):
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                return float(json.load(resp)["answers"]["q"]["noul"])
        except Exception:
            continue
    return 0.5

def main():
    kind = sys.argv[1]
    test = [json.loads(l) for l in open("data/test.jsonl")]
    limit = int(os.environ.get("LIMIT", "0"))
    if limit:
        test = test[:limit]
    if kind == "local":
        from judge import Judge
        j = Judge(sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else None)
        ps = [j.p(r["question"], r["item"]) for r in test]
        name = "local-" + ("tuned" if len(sys.argv) > 3 else "base") + "-" + sys.argv[2].split("/")[-1]
    elif kind == "claude":
        with cf.ThreadPoolExecutor(6) as ex:
            ps = list(ex.map(lambda r: claude_p(sys.argv[2], r), test))
        name = "claude-" + sys.argv[2]
    else:
        with cf.ThreadPoolExecutor(8) as ex:
            ps = list(ex.map(jev_p, test))
        name = "jev"
    pairs = [(p, int(r["label"])) for p, r in zip(ps, test)]
    res = {"judge": name, "all": metrics(pairs),
           "seen": metrics([(p, int(r["label"])) for p, r in zip(ps, test) if not r["unseen"]]),
           "unseen": metrics([(p, int(r["label"])) for p, r in zip(ps, test) if r["unseen"]]),
           "hard": metrics([(p, int(r["label"])) for p, r in zip(ps, test) if r.get("hard")])}
    Path("results").mkdir(exist_ok=True)
    json.dump(res, open(f"results/{name}.json", "w"), indent=1)
    print(json.dumps(res))

if __name__ == "__main__":
    main()
