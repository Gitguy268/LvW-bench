<div align="center">

# LvW-bench

A local benchmark for tiny AI models: System 1 decision models and System 2 generative models,
scored on your own machine.

[Leaderboard](https://gitguy268.github.io/LvW-bench/) ·
[Docs](https://gitguy268.github.io/LvW-bench/docs.html) ·
[Run in browser](https://gitguy268.github.io/LvW-bench/run.html) ·
[Dataset](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench) ·
[Install](#install) · [Usage](#usage) · [How it scores](#how-it-scores)

[![CI](https://github.com/Gitguy268/LvW-bench/actions/workflows/ci.yml/badge.svg)](https://github.com/Gitguy268/LvW-bench/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Gitguy268/LvW-bench)](https://github.com/Gitguy268/LvW-bench/releases)
[![Dataset](https://img.shields.io/badge/%F0%9F%A4%97-LvW--bench-yellow)](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

<img src="docs/leaderboard.png" alt="LvW-bench leaderboard page" width="900">

</div>

## What it is

LvW-bench measures small models on three tracks:

| Track | Kind of model | What it asks | Scored on |
|---|---|---|---|
| **decide** | System 1: decision models (d1, Jev-style heads), or any LLM | 5 workflows of typed questions (`choice`, `score`, `noul`), each answered as a probability distribution | accuracy, KL from gold, Brier, ECE |
| **answer** | System 2: generative models | GSM8K, MMLU, MMLU-Pro, TruthfulQA, IFEval and a seeded generated set, thinking off | accuracy (95% CI), tokens, speed |
| **reason** | System 2: generative models | the same sets with thinking on: native `<think>`, or step by step | accuracy, out-of-budget rate |

It takes the same inputs as hosted benches such as TPN: a Hugging Face link or id, a local
**`.gguf`**, a local **`.safetensors`** folder (adapters too), or any OpenAI-compatible server.
Everything runs locally:
- GGUF files run through `llama-server`. Decision models use its native `/v1/systemone`.
- Safetensors run through a small transformers helper that `uv` starts.
- Each result is tied to the repo commit, file, quant and engine build.

There are four ways to run it: the `lvwb` terminal command, a terminal UI, a desktop app for
macOS, Windows and Linux, and a [browser runner](https://gitguy268.github.io/LvW-bench/run.html)
(wllama or WebLLM on WebGPU). The task data and the leaderboard are published as a benchmark
dataset on Hugging Face, in the same layout as
[typed-decisions](https://huggingface.co/datasets/LocalLLaMA/typed-decisions).

## Install

### Install with Homebrew

```bash
brew trust gitguy268/tap                          # once: new Homebrew asks before using a tap
brew install gitguy268/tap/lvw-bench              # the lvwb command (macOS and Linux)
brew install --cask gitguy268/tap/lvw-bench-app   # the desktop app (macOS)
brew install llama.cpp uv                         # engines: GGUF and safetensors
```

### Other ways

| Platform | Terminal | App |
|---|---|---|
| macOS | Homebrew, or `lvwb-*-darwin-*.tar.gz` | Homebrew cask, or `LvW-bench-*-macos-universal.zip` |
| Linux | Homebrew, or `lvwb-*-linux-*.tar.gz` | `lvw-bench-app_*_amd64.deb` or `LvW-bench-*-linux-amd64.tar.gz` |
| Windows | `lvwb-*-windows-*.zip` | `LvW-bench-*-windows-amd64-setup.exe` |

All of these are on the [releases page](https://github.com/Gitguy268/LvW-bench/releases).
`SHA256SUMS.txt` lists the checksums.

The release builds are not signed. On macOS, the cask removes the quarantine flag; if you
download the zip yourself, right-click the app and choose Open the first time. On Windows,
SmartScreen asks once.

<details><summary>Build from source</summary>

```bash
git clone https://github.com/Gitguy268/LvW-bench && cd LvW-bench
go build -o lvwb ./cmd/lvwb            # Go 1.23+
./lvwb selftest

# desktop app (Wails v2; Linux needs libgtk-3-dev and libwebkit2gtk-4.1-dev)
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
cd app && wails build                  # add -tags webkit2_41 on Linux
```

</details>

## Usage

```bash
lvwb run Lucasgitguy-co/spot-snake-d1-2B-v0.1           # HF id: picks the GGUF (Q4_K_M), starts llama-server
lvwb run https://huggingface.co/unsloth/Qwen3-0.6B-GGUF  # HF link
lvwb run ~/models/model-Q4_K_M.gguf                      # local GGUF
lvwb run ~/models/my-model/                              # local safetensors folder
lvwb run http://127.0.0.1:8080/v1 --backend openai       # a server you already run
lvwb run qwen3-0.6b --profile quick                      # a preset (lvwb models)
lvwb tui                                                 # pick a preset in the terminal UI
lvwb leaderboard                                         # your saved results, ranked per track
```

| Flag | Meaning |
|---|---|
| `--track decide,answer,reason` | tracks to run (default: every track the model supports) |
| `--profile quick\|standard\|full` | smoke test, the leaderboard set, or everything |
| `--quant`, `--file`, `--format`, `--subfolder`, `--revision` | which weights to use inside a repo |
| `--trust-remote-code` | allow a repo's own model code (safetensors) |
| `--seed N` | a fresh generated set |
| `--mode specialist` | the model was trained on the same task |

Other commands: `lvwb models`, `tasks`, `results`, `compare a.json b.json`,
`export --md|--csv|--eval-results`, `rescore`, `submit`, `selftest`.

<details><summary>The desktop app</summary>

The app is a window over the same runner. Pick a preset or type a model, choose the tracks and
profile, and watch each track's progress. Saved results are in the Results tab. It looks for
`llama-server` and `uv` and says which one is missing.

<img src="docs/app.png" alt="LvW-bench desktop app" width="820">

</details>

## Leaderboard

Bench 0.1.0, `standard` profile, all runs local. The live version with sorting and per-workflow
columns is on the [leaderboard page](https://gitguy268.github.io/LvW-bench/).

<!-- leaderboard:start -->
<!-- leaderboard:end -->

## How it scores

<details><summary>System 1: decide</summary>

There are five original workflows: `spot_snake`, `it_support`, `delivery`, `code_review` and
`calendar`. Each case has a state and three typed questions. The gold distribution comes from
fixed rules, with no teacher model:
- A fact the state shows directly gets a sharp gold.
- A judgement call keeps a soft gold.

`spot_snake` uses the Spot's Biscuit Snake game's own text and the teacher from
[spot-snake-decisions](https://huggingface.co/datasets/Lucasgitguy-co/spot-snake-decisions-v0.1).

How models answer:
- **Decision models** answer the whole case through `/v1/systemone`.
- **Text models** answer each question through the probabilities of their first output token.
  When a server gives no logprobs, the bench samples 8 answers instead.

Scoring follows typed-decisions' scorer: argmax accuracy, KL(gold‖pred), Brier and 10-bin ECE.
Models trained on the same task are listed as **specialists**, apart from the zero-shot table.

</details>

<details><summary>System 2: answer and reason</summary>

The bench uses permissive subsets only: GSM8K, MMLU and MMLU-Pro (MIT), and TruthfulQA and
IFEval (Apache-2.0). ARC is CC-BY-SA, so it is left out. There is also a generated set of 225
items: arithmetic, word problems, letter counts, sorting, dates, syllogisms, list lookups, JSON
and Mermaid. Every generated answer is computed, so `--seed` gives fresh items.

Scoring:
- The scorer removes `<think>` blocks, then reads the `Answer:` line, the leading letter or the
  last number.
- A think block that never closes is out of budget and counts as wrong.
- IFEval uses strict prompt-level accuracy on 17 verifiable instruction types.

These are generated-answer scores, so they are **not comparable with lm-eval loglikelihood
numbers** (for example TPN's ARC `acc_norm`).

</details>

<details><summary>Reproducing a score</summary>

Every result has a `.predictions.jsonl` next to it. `lvwb rescore` (Go) or `bench/score.py`
(Python) recomputes the score without the model. The Go, Python and browser scorers share
`internal/score/golden.json`, and CI fails if any of them disagree.

</details>

## Repository

| Path | What is there |
|---|---|
| `cmd/lvwb` | CLI and TUI |
| `internal/` | tasks, scorers, model resolver, HF downloads, backends, runner |
| `helper/` | the Python safetensors server (embedded in `lvwb`, run with uv) |
| `app/` | Wails desktop app |
| `web/` | GitHub Pages: leaderboard, docs, browser runner |
| `dataset/` | builds the Hugging Face dataset and the Space |
| `bench/` | Python scorer shipped with the dataset |
| `results/` | the seed leaderboard results |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Results go to the dataset's
[discussions](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench/discussions) or as a PR
that adds files to `results/`.

## Licence

MIT, © Lucas Van De Weerd. The rows from GSM8K, MMLU and MMLU-Pro stay MIT, and the rows from
TruthfulQA and IFEval stay Apache-2.0. The scorer ports and every credit are in
[NOTICE](NOTICE). Models keep their own licences. LvW-bench is not affiliated with TPN,
TypeSafe, LocalLLaMA, Liquid AI or Hugging Face.
