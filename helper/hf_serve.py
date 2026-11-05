# SPDX-License-Identifier: MIT
"""LvW-bench helper: serve a safetensors model (local folder or HF repo) over a small
OpenAI-compatible API, plus /v1/systemone for decision heads.

  POST /v1/chat/completions   generation, temperature 0 = greedy, top_logprobs of the first token
  POST /v1/systemone          only for decision plugins (the spot-snake MiniMind-JEV ranker)
  GET  /v1/models, /health

lvwb starts this with `uv run --python 3.12`; it binds to 127.0.0.1 only.
"""
import argparse
import json
import math
import re
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import torch


def device():
    if torch.cuda.is_available():
        return "cuda"
    if torch.backends.mps.is_available():
        return "mps"
    return "cpu"


def resolve_dir(model, revision, subfolder):
    """Local folder as is; a repo id is downloaded with huggingface_hub (shared HF cache)."""
    p = Path(model).expanduser()
    if p.exists():
        return p / subfolder if subfolder else p
    from huggingface_hub import snapshot_download
    allow = [f"{subfolder}/*"] if subfolder else None
    local = Path(snapshot_download(model, revision=revision or None, allow_patterns=allow))
    return local / subfolder if subfolder else local


class CausalLM:
    """Any transformers causal LM, optionally a PEFT adapter on a base model."""

    def __init__(self, path, args):
        from transformers import AutoModelForCausalLM, AutoTokenizer
        dev = device()
        cfg_path = path / "config.json"
        dtype = args.dtype
        if not dtype and cfg_path.exists():
            cfg = json.loads(cfg_path.read_text())
            dtype = cfg.get("torch_dtype") or cfg.get("dtype") or "float32"
        dtype = dtype or "float32"
        if dev == "cpu" and dtype == "float16":
            dtype = "float32"  # fp16 matmuls are slow or missing on CPU
        torch_dtype = getattr(torch, dtype)
        trust = args.trust_remote_code
        adapter_cfg = path / "adapter_config.json"
        if adapter_cfg.exists() and not cfg_path.exists():
            from peft import PeftModel
            base = args.base or json.loads(adapter_cfg.read_text()).get("base_model_name_or_path")
            if not base:
                raise SystemExit("adapter has no base model: pass --base")
            self.tok = AutoTokenizer.from_pretrained(base, trust_remote_code=trust)
            model = AutoModelForCausalLM.from_pretrained(base, dtype=torch_dtype, trust_remote_code=trust)
            model = PeftModel.from_pretrained(model, str(path))
        else:
            self.tok = AutoTokenizer.from_pretrained(str(path), trust_remote_code=trust)
            model = AutoModelForCausalLM.from_pretrained(str(path), dtype=torch_dtype, trust_remote_code=trust)
        self.model = model.to(dev).eval()
        self.dev = dev
        self.name = path.name
        self.lock = threading.Lock()

    def prompt_ids(self, messages, thinking):
        tmpl = getattr(self.tok, "chat_template", None)
        if tmpl:
            kw = {} if thinking is None else {"enable_thinking": thinking}
            try:
                text = self.tok.apply_chat_template(messages, tokenize=False, add_generation_prompt=True, **kw)
            except TypeError:
                text = self.tok.apply_chat_template(messages, tokenize=False, add_generation_prompt=True)
        else:
            text = "".join(f"{m['role'].capitalize()}: {m['content']}\n" for m in messages) + "Assistant:"
        enc = self.tok(text, return_tensors="pt", add_special_tokens=not tmpl)
        return enc["input_ids"].to(self.dev)

    def chat(self, body):
        thinking = (body.get("chat_template_kwargs") or {}).get("enable_thinking")
        ids = self.prompt_ids(body["messages"], thinking)
        max_new = int(body.get("max_tokens") or 256)
        temp = float(body.get("temperature") or 0)
        top_n = int(body.get("top_logprobs") or 0) if body.get("logprobs") else 0
        gen = dict(max_new_tokens=max_new, return_dict_in_generate=True, output_scores=top_n > 0,
                   pad_token_id=self.tok.pad_token_id if self.tok.pad_token_id is not None else self.tok.eos_token_id)
        if temp > 0:
            gen.update(do_sample=True, temperature=temp)
        else:
            gen.update(do_sample=False)
        with self.lock, torch.no_grad():
            out = self.model.generate(ids, **gen)
        new = out.sequences[0, ids.shape[1]:]
        text = self.tok.decode(new, skip_special_tokens=True)
        eos = self.tok.eos_token_id
        eos_ids = set(eos if isinstance(eos, list) else [eos])
        finish = "stop" if len(new) < max_new or (len(new) and int(new[-1]) in eos_ids) else "length"
        choice = {"index": 0, "message": {"role": "assistant", "content": text}, "finish_reason": finish}
        if top_n > 0 and out.scores:
            logp = torch.log_softmax(out.scores[0][0].float(), dim=-1)
            vals, idx = logp.topk(top_n)
            first = int(new[0]) if len(new) else int(idx[0])
            choice["logprobs"] = {"content": [{
                "token": self.tok.decode([first]), "logprob": float(logp[first]),
                "top_logprobs": [{"token": self.tok.decode([int(i)]), "logprob": float(v)} for v, i in zip(vals, idx)],
            }]}
        return {"object": "chat.completion", "model": self.name, "choices": [choice],
                "usage": {"prompt_tokens": int(ids.shape[1]), "completion_tokens": int(len(new))}}


class SpotSnakeRanker:
    """Lucasgitguy-co/spot-snake-minimind-jev-v0.1: MiniMind-JEV head over a frozen BGE encoder.
    It ranks the safe moves of the spot_snake workflow and answers nothing else."""

    TRANSLATIONS = [("Which way should Spot step next to reach the ", "Spot下一步往哪里走才能到达"),
                    (" without getting boxed in?", "且不被困住？"), ("Spot is ", "Spot长"),
                    (" squares long on a ", "格，场地"), (" by ", "乘"), (" lawn, heading ", "格，朝向"),
                    ("The ", ""), (" is ", "在"), ("golden biscuit", "金色饼干"), ("biscuit", "饼干"),
                    ("eats the ", "吃到"), ("goes toward the ", "靠近"), ("goes away from the ", "远离"),
                    (", leaves ", "，留下"), (" open squares", "个可达空格"), (" and ", "和"), ("here", "这里")]

    def __init__(self, path):
        from safetensors.torch import load_file
        from sentence_transformers import SentenceTransformer
        from torch import nn
        cfg = json.loads((path / "config.json").read_text())

        class Head(nn.Module):
            def __init__(self, dim):
                super().__init__()
                self.net = nn.Sequential(nn.Linear(dim, 256), nn.GELU(), nn.Linear(256, 256), nn.GELU(), nn.Linear(256, 1))

            def forward(self, x):
                return self.net(x).squeeze(-1)

        self.dev = device()
        self.head = Head(cfg["dim"]).to(self.dev).eval()
        self.head.load_state_dict(load_file(str(path / "head.safetensors")))
        self.enc = SentenceTransformer(str(path / "encoder"), device=self.dev)
        self.temperature = float(cfg.get("temperature", 1.0))
        self.name = path.name
        self.lock = threading.Lock()

    def zh(self, state, ask, option):
        s = state + "\n" + ask + "\n候选动作：" + option
        for a, b in self.TRANSLATIONS:
            s = s.replace(a, b)
        for a, b in [("up", "上"), ("down", "下"), ("left", "左"), ("right", "右")]:
            s = re.sub(r"\b" + a + r"\b", b, s)
        return s

    @staticmethod
    def state_text(state):
        if isinstance(state, str):
            return state
        spot, lawn = state.get("spot", {}), state.get("lawn", "")
        return (f"Spot is {spot.get('length')} squares long on a {lawn} lawn, heading {spot.get('heading')}. "
                f"The biscuit is {state.get('biscuit')}.")

    def systemone(self, body):
        state = self.state_text(body.get("state"))
        answers = {}
        for qid, q in (body.get("questions") or {}).items():
            crit = q.get("criteria") or {}
            if q.get("type") != "choice" or not isinstance(crit, dict) or not set(crit) <= {"up", "down", "left", "right"}:
                continue  # not a move question: this head cannot answer it
            moves = list(crit)
            texts = [self.zh(state, q.get("instructions", ""), f"{m}: {crit[m] or ''}") for m in moves]
            with self.lock, torch.no_grad():
                emb = self.enc.encode(texts, normalize_embeddings=True, convert_to_tensor=True).float()
                z = self.head(emb.to(self.dev)).float().cpu() / self.temperature
            p = torch.softmax(z, dim=0).tolist()
            best = max(range(len(moves)), key=lambda i: p[i])
            answers[qid] = {"type": "choice", "choice": moves[best], "probabilities": dict(zip(moves, p)),
                            "confidence": p[best]}
        return {"model": self.name, "answers": answers, "usage": {"input_tokens": 0, "output_tokens": 0}}


def make_handler(engine, modalities):
    class H(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def send(self, code, obj):
            b = json.dumps(obj).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(b)))
            self.end_headers()
            self.wfile.write(b)

        def do_GET(self):
            if self.path in ("/health", "/v1/health"):
                import transformers
                return self.send(200, {"status": "ok", "torch": torch.__version__, "transformers": transformers.__version__,
                                       "device": getattr(engine, "dev", "cpu")})
            if self.path in ("/v1/models", "/models"):
                return self.send(200, {"object": "list", "data": [{"id": engine.name, "object": "model",
                                       "architecture": {"input_modalities": ["text"], "output_modalities": modalities}}]})
            self.send(404, {"error": "not found"})

        def do_POST(self):
            try:
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
                if self.path.endswith("/chat/completions") and hasattr(engine, "chat"):
                    return self.send(200, engine.chat(body))
                if self.path.endswith("/systemone") and hasattr(engine, "systemone"):
                    return self.send(200, engine.systemone(body))
                self.send(404, {"error": f"{self.path} is not served by this model"})
            except Exception as e:  # report, keep serving
                self.send(500, {"error": f"{type(e).__name__}: {e}"})

    return H


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", required=True)
    ap.add_argument("--revision", default="")
    ap.add_argument("--subfolder", default="")
    ap.add_argument("--base", default="")
    ap.add_argument("--dtype", default="")
    ap.add_argument("--plugin", default="auto")
    ap.add_argument("--trust-remote-code", action="store_true")
    ap.add_argument("--port", type=int, required=True)
    args = ap.parse_args()
    t0 = time.time()
    path = resolve_dir(args.model, args.revision, args.subfolder)
    plugin = args.plugin
    if plugin == "auto":
        plugin = "spot-snake-ranker" if (path / "head.safetensors").exists() and (path / "encoder").exists() else "causal-lm"
    if plugin == "spot-snake-ranker":
        engine, mods = SpotSnakeRanker(path), ["decisions"]
    else:
        engine, mods = CausalLM(path, args), ["text"]
    print(f"[helper] {plugin} {path} on {getattr(engine, 'dev', '?')} ready in {time.time() - t0:.1f}s", flush=True)
    ThreadingHTTPServer(("127.0.0.1", args.port), make_handler(engine, mods)).serve_forever()


if __name__ == "__main__":
    main()
