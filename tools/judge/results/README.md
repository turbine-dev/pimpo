# Local judge: results

Question-conditioned yes/no judgments ("Is this email a bill?", "Does this event need travel?") on realistic items. 1708 examples labeled by Claude Sonnet (24 questions, balanced, with deliberately hard cases). Three questions were held out of training entirely: "unseen" is how the model does on questions it never saw.

Test set: 422 items, 216 of them on unseen questions, 96 marked hard.

| Judge | Accuracy | Brier | ECE | AUC | Unseen acc. | Hard acc. | Cost per 1000 |
|---|---|---|---|---|---|---|---|
| Qwen3-0.6B, base | 0.512 | 0.282 | 0.255 | 0.661 | 0.509 | 0.469 | free |
| Qwen3-0.6B, LoRA (600 iters, M1) | 0.851 | 0.120 | 0.091 | 0.921 | 0.852 | 0.667 | free |
| Jev | 0.945 | 0.036 | 0.052 | 0.991 | 0.972 | 0.802 | paid API |
| Claude Haiku (Claude Code) | 0.953 | 0.038 | 0.027 | 0.986 | 0.981 | 0.865 | ≈ $10 |

Fine-tuning takes the small model from chance to 85%, and it holds on questions it never trained on. It is still clearly behind Jev and Haiku, especially on hard cases.

## Cascade

The local model answers; only items where it is unsure (p within a band around 0.5) go to Jev.

| Band | Sent to Jev | Accuracy | Brier | ECE | AUC |
|---|---|---|---|---|---|
| none | 0% | 0.851 | 0.120 | 0.091 | 0.921 |
| ±0.1 | 3% | 0.865 | 0.113 | 0.084 | 0.926 |
| ±0.2 | 5% | 0.872 | 0.109 | 0.078 | 0.928 |
| ±0.3 | 10% | 0.889 | 0.099 | 0.074 | 0.934 |
| ±0.4 | 22% | 0.924 | 0.071 | 0.057 | 0.945 |
| all | 100% | 0.948 | 0.036 | 0.046 | 0.991 |

**Decision:** with the local backend selected, Vigia uses the cascade with a band of 0.4 (`judge.Cascade`): about a fifth of the paid calls for most of the accuracy. Without a Jev key, the unsure items go to the owner's model instead. With no network at all, the local answer stands.

## Reproduce

```
.venv/bin/python generate.py 3      # labels with Claude (≈ $10)
.venv/bin/python prepare.py
.venv/bin/mlx_lm.lora --model Qwen/Qwen3-0.6B --train --data mlx --adapter-path adapters --iters 600 --batch-size 8 --mask-prompt
.venv/bin/python evaluate.py local Qwen/Qwen3-0.6B adapters
.venv/bin/python evaluate.py jev          # needs TYPESAFE_API_KEY
.venv/bin/python evaluate.py claude haiku
.venv/bin/python cascade.py results/local-tuned-Qwen3-0.6B.preds.json results/jev.preds.json
```
