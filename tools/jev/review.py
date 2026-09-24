#!/usr/bin/env python3
"""Ask Jev to review PLANNING.md section by section and as a whole. Writes review.json."""
import json
import os
import re
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from prioritize import PRODUCT, call

HERE = Path(__file__).parent
DOC = (HERE.parent / "PLANNING.md").read_text()

SECTION_QS = {
    "specificity": {
        "type": "score",
        "instructions": "How concrete and actionable is `section` for a solo founder to start building from it?",
        "criteria": [
            "Vague: vision or slogans only",
            "Sketched: clear idea, missing behavior and data details",
            "Concrete: behavior, data and interfaces defined",
            "Ready to build: includes edge cases and acceptance criteria",
        ],
    },
    "open_decision": {
        "type": "noul",
        "instructions": "Does `section` leave an important decision unanswered that must be made before building what it describes (and that is not already listed as a pending decision)?",
    },
    "fits_mvp": {
        "type": "noul",
        "instructions": "Is `section` consistent with the `product`'s `mvp_goal` (it does not push essential MVP work later nor pull non-essential work into the MVP)?",
    },
}

DOC_QS = {
    "mvp_small_enough": {
        "type": "noul",
        "instructions": "Is the work in phases F0 and F1 of `plan` small enough for one founder helped by coding agents to reach the F1 gate?",
    },
    "phase_order_sound": {
        "type": "noul",
        "instructions": "Is the phase order in `plan` sound: riskiest assumption first, security before wide use, UI polish before recruiting non-technical users?",
    },
    "ui_supports_wow": {
        "type": "noul",
        "instructions": "Does the UI described in `plan` make the two 'wow' moments (the compile moment and the blocked-action moment) clearly visible to a first-time user?",
    },
    "better_than_competitors": {
        "type": "score",
        "instructions": "If `plan` is executed as written, how clearly would the result be better than OpenClaw and Hermes Agent for the `product`'s `target_user`?",
        "criteria": [
            "Not better: at best parity",
            "Somewhat better in a few areas",
            "Clearly better on the pains that make users quit",
            "Decisively better: a reason to switch for most of them",
        ],
    },
    "biggest_risk": {
        "type": "choice",
        "instructions": "What is the biggest risk to `plan` succeeding?",
        "criteria": {
            "compiler_hard": "Compiling tasks into reliable routines turns out too hard or too brittle",
            "few_repeat_tasks": "Users have too few repetitive tasks for routines to matter",
            "google_oauth": "Gmail/Google API access (OAuth verification, restricted scopes) blocks distribution",
            "approval_fatigue": "Approvals annoy users and they turn safety off",
            "adoption": "Users stay with OpenClaw/Hermes because of their ecosystems and communities",
            "solo_scope": "The scope is too large for one founder",
        },
    },
    "missing_piece": {
        "type": "choice",
        "instructions": "Which important piece is most missing from `plan`?",
        "criteria": {
            "user_research": "Talking to real users before building",
            "distribution": "How people will discover and install it",
            "data_privacy": "Privacy and data handling policy",
            "ops_support": "Support, bug reporting and maintenance process",
            "compiler_spec": "A precise spec of how the routine compiler decides what is compilable",
            "nothing_major": "Nothing major is missing",
        },
    },
}


def sections(doc):
    parts = re.split(r"^## ", doc, flags=re.M)[1:]
    return [{"title": p.partition("\n")[0].strip(), "body": p.partition("\n")[2].strip()} for p in parts]


def main():
    if not os.environ.get("TYPESAFE_API_KEY"):
        sys.exit("set TYPESAFE_API_KEY")
    secs = sections(DOC)
    with ThreadPoolExecutor(4) as ex:
        whole = ex.submit(call, {"product": PRODUCT, "plan": DOC}, DOC_QS)
        per = list(ex.map(lambda s: (s["title"], call({"product": PRODUCT, "section": s}, SECTION_QS)), secs))
        doc = whole.result()
    (HERE / "review.json").write_text(json.dumps({"document": doc, "sections": dict(per)}, indent=1, ensure_ascii=False))

    print(f"{'section':48} {'concr':>5} {'open':>5} {'mvp':>5}")
    for title, r in per:
        a = r["answers"]
        print(f"{title[:48]:48} {a['specificity']['score']:5.2f} {a['open_decision']['noul']:5.2f} {a['fits_mvp']['noul']:5.2f}")
    print()
    for k, a in doc["answers"].items():
        if a["type"] == "noul":
            print(f"{k:24} P(yes)={a['noul']:.2f}")
        elif a["type"] == "score":
            print(f"{k:24} score={a['score']:.2f} conf={a['confidence']:.2f}")
        else:
            probs = sorted(a["probabilities"].items(), key=lambda x: -x[1])
            print(f"{k:24} {a['choice']} (conf {a['confidence']:.2f}) " + ", ".join(f"{o}={p:.2f}" for o, p in probs[:3]))


if __name__ == "__main__":
    main()
