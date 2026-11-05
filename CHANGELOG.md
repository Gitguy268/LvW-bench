# Changelog

## 0.1.0 (2026-11-05)

First release.

- Three tracks: `decide` (System 1, typed decisions scored on accuracy, KL, Brier and ECE),
  `answer` (System 2, thinking off) and `reason` (System 2, thinking on).
- Five original System 1 workflows with rule gold. Permissive subsets of GSM8K, MMLU, MMLU-Pro,
  TruthfulQA and IFEval, plus a seeded generated set of 225 items.
- Model inputs: HF links and ids, local `.gguf`, local `.safetensors` folders and adapters, and
  OpenAI-compatible servers and Ollama. GGUF runs on llama.cpp (native `/v1/systemone` for
  decision models). Safetensors run through a uv/transformers helper.
- `lvwb` CLI and TUI, a Wails desktop app, and a browser runner (wllama, WebLLM).
- Hugging Face dataset with `eval.yaml`, a leaderboard and seed results for the Spot Snake and
  Mermaid models plus small baselines.
