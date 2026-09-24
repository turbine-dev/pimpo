#!/usr/bin/env python3
"""Ask Jev whether PLANNING.md is a complete plan (not only an MVP) and score every roadmap phase. Writes review_full.json."""
import json
import os
import re
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from prioritize import PRODUCT, call

HERE = Path(__file__).parent
DOC = (HERE.parent / "PLANNING.md").read_text()

PHASE_QS = {
    "specificity": {
        "type": "score",
        "instructions": "How concrete is `phase` as a plan to execute: are goals, deliverables, quality checks and the exit gate clear?",
        "criteria": [
            "Vague: a theme without deliverables",
            "Sketched: deliverables named but gate or quality unclear",
            "Concrete: deliverables, quality and a verifiable gate",
            "Ready to execute: also edge cases, dependencies and metrics",
        ],
    },
    "gate_verifiable": {"type": "noul", "instructions": "Is the gate of `phase` objectively verifiable (someone could check pass/fail without judgment calls)?"},
    "right_place": {"type": "noul", "instructions": "Given the rest of `roadmap_order`, is `phase` placed at a sensible point in the sequence?"},
}

DOC_QS = {
    "complete_plan": {
        "type": "noul",
        "instructions": "Is `plan` a complete plan from first commit to a mature, sustainable product (not just an MVP), covering product, UI, technology, quality, operations, community and long-term direction?",
    },
    "post_v1_detail": {
        "type": "score",
        "instructions": "How detailed are the phases after v1.0 (F6 to F12) in `plan` compared with the early phases?",
        "criteria": [
            "Much thinner: a list of themes",
            "Thinner: deliverables but little detail",
            "Similar: comparable detail",
            "Equally detailed including gates and metrics",
        ],
    },
    "missing_area": {
        "type": "choice",
        "instructions": "Which area is most missing or weakest in `plan`?",
        "criteria": {
            "legal_tos": "Legal and terms-of-service risks (WhatsApp, Gmail, Telegram policies, liability)",
            "data_backup": "User data backup, export and disaster recovery",
            "performance": "Performance and resource limits (Raspberry Pi, many routines)",
            "maintainer_load": "Maintainer workload and burnout for a solo founder",
            "user_research": "Talking to real users",
            "nothing_major": "Nothing major is missing",
        },
    },
    "ui_complete": {
        "type": "noul",
        "instructions": "Does `plan` cover the UI across the whole product life (web, Telegram, mobile, desktop, accessibility, languages), not only the MVP screens?",
    },
}


def main():
    if not os.environ.get("TYPESAFE_API_KEY"):
        sys.exit("set TYPESAFE_API_KEY")
    road = DOC[DOC.index("## 8. Roadmap completo"):DOC.index("## 9. Mapa completo")]
    order = re.findall(r"^### (F\d+ · [^\n]+)", road, flags=re.M)
    phases = [p for p in re.split(r"^### ", road, flags=re.M)[1:]]
    with ThreadPoolExecutor(4) as ex:
        whole = ex.submit(call, {"product": PRODUCT, "plan": DOC}, DOC_QS)
        per = list(ex.map(lambda p: (p.partition("\n")[0], call({"product": PRODUCT, "phase": p, "roadmap_order": order}, PHASE_QS)), phases))
        doc = whole.result()
    (HERE / "review_full.json").write_text(json.dumps({"document": doc, "phases": dict(per)}, indent=1, ensure_ascii=False))
    print(f"{'phase':55} {'concr':>5} {'gate':>5} {'place':>5}")
    for title, r in per:
        a = r["answers"]
        print(f"{title[:55]:55} {a['specificity']['score']:5.2f} {a['gate_verifiable']['noul']:5.2f} {a['right_place']['noul']:5.2f}")
    print()
    for k, a in doc["answers"].items():
        if a["type"] == "noul":
            print(f"{k:18} P(yes)={a['noul']:.2f}")
        elif a["type"] == "score":
            print(f"{k:18} score={a['score']:.2f}")
        else:
            probs = sorted(a["probabilities"].items(), key=lambda x: -x[1])
            print(f"{k:18} {a['choice']} " + ", ".join(f"{o}={p:.2f}" for o, p in probs[:3]))


if __name__ == "__main__":
    main()
