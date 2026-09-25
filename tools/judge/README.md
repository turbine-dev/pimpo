# Local judgment model

Routines ask small yes/no questions ("is this email important?"). This folder trains a small model that answers them on your own machine, free and offline, and reads the probability straight from the model's logits.

## Steps

```bash
python3 -m venv .venv && .venv/bin/pip install mlx-lm
python3 generate.py 3                     # labeled examples from Claude (data/all.jsonl)
python3 prepare.py                        # train/valid/test; three question types held out entirely
.venv/bin/mlx_lm.lora --model Qwen/Qwen3-0.6B --train --data mlx --adapter-path adapters --iters 600 --batch-size 8 --mask-prompt
.venv/bin/python evaluate.py local Qwen/Qwen3-0.6B adapters
.venv/bin/python serve.py Qwen/Qwen3-0.6B adapters 11500
```

Then pick **Modelo local** in Zodim's settings. The results of the comparison with Jev and Claude are in [results/README.md](results/README.md).

Labels come from Claude, not from Jev, so the model is not trained on a vendor's outputs it may not be trained on.
