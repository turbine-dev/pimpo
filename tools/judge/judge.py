"""Question-conditioned yes/no judge on a small local model with MLX. The
probability comes from the model's logits for "yes" and "no", not from
generated text."""
import json
import mlx.core as mx
from mlx_lm import load
from prepare import SYSTEM, prompt

class Judge:
    def __init__(self, model, adapter=None):
        self.model, self.tok = load(model, adapter_path=adapter)
        ids = lambda w: self.tok.encode(w, add_special_tokens=False)
        self.yes = {ids(w)[0] for w in ["yes", "Yes", " yes"]}
        self.no = {ids(w)[0] for w in ["no", "No", " no"]}

    def p(self, question, item):
        msgs = [{"role": "system", "content": SYSTEM}, {"role": "user", "content": prompt({"question": question, "item": item})}]
        try:
            text = self.tok.apply_chat_template(msgs, tokenize=False, add_generation_prompt=True, enable_thinking=False)
        except TypeError:
            text = self.tok.apply_chat_template(msgs, tokenize=False, add_generation_prompt=True)
        tokens = mx.array(self.tok.encode(text, add_special_tokens=False))[None]
        logits = self.model(tokens)[0, -1]
        probs = mx.softmax(logits.astype(mx.float32), axis=-1)
        y = sum(probs[i].item() for i in self.yes)
        n = sum(probs[i].item() for i in self.no)
        return y / (y + n) if y + n > 0 else 0.5
