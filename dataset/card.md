---
license: mit
language:
- en
pretty_name: LvW-bench
size_categories:
- 1K<n<10K
task_categories:
- text-classification
- question-answering
- text-generation
tags:
- benchmark
- small-models
- tiny-models
- system-one
- system-two
- structured-decisions
- calibration
- reasoning
- local-inference
- gguf
- safetensors
configs:
{{CONFIGS}}
---

# LvW-bench

A benchmark for tiny models that you run on your own machine. It scores both kinds of small
model in one place:

- **System 1, `decide`:** typed decisions. The model gets a state and typed questions
  (`choice`, `score`, `noul`) and answers each with a probability distribution. Scores are
  accuracy, KL from gold, Brier and ECE, the same as
  [LocalLLaMA/typed-decisions](https://huggingface.co/datasets/LocalLLaMA/typed-decisions).
  Native decision models (d1, Jev-style heads) answer through `/v1/systemone`. A text model
  answers through the probabilities of its first output token.
- **System 2, `answer` and `reason`:** generated answers on permissive subsets of GSM8K, MMLU,
  MMLU-Pro, TruthfulQA and IFEval, plus a seeded generated set. Each model runs twice, once
  with thinking off and a small budget (`answer`) and once with thinking on (`reason`).
  Thinking is native `<think>` when the chat template has it, and step by step otherwise.

The runner is `lvwb`, from the [GitHub repo](https://github.com/Gitguy268/LvW-bench). It runs
on macOS, Linux and Windows, from the terminal (CLI and TUI), from a desktop app, or in the
browser. It accepts a Hugging Face link or id, a local `.gguf` file, a local `.safetensors`
folder, or any OpenAI-compatible server.

![Accuracy against KL for System 1, and answer against reason accuracy for System 2](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench/resolve/main/assets/leaderboard.png)

## Leaderboard

Bench version {{VERSION}}, `standard` profile:

- `decide` uses the whole `decide_all` test split (500 cases, 1,500 decisions).
- `answer` uses the whole `generative_all` test split (1,143 items).
- `reason` uses `reason_subset` (240 items, 40 per set, 2,048-token budget).

All runs were local, on llama.cpp for GGUF and transformers for safetensors. Every row
records the exact repo commit, file, quant and engine build. The result and prediction files
are in [`results/`](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench/tree/main/results).

{{LEADERBOARD}}

To add a model, run it with `lvwb run <model>` and open a
[discussion](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench/discussions). Attach the
result JSON and its `.predictions.jsonl`, and say whether the model is general or a
specialist. `lvwb submit <result.json>` prints the steps. `bench/score.py` rescores the
predictions file to the same numbers without the model.

## Benchmarking a model

```bash
brew trust gitguy268/tap && brew install gitguy268/tap/lvw-bench   # or a release binary
lvwb run Lucasgitguy-co/spot-snake-d1-2B-v0.1           # HF id: picks the GGUF, starts llama-server
lvwb run https://huggingface.co/unsloth/Qwen3-0.6B-GGUF # HF link
lvwb run ~/models/my-model-Q4_K_M.gguf                  # local GGUF
lvwb run ~/models/my-model/                             # local safetensors folder (uv + transformers)
lvwb run http://127.0.0.1:8080/v1 --backend openai      # a server that is already running
lvwb tui                                                # pick a preset in the terminal UI
```

- `--track decide|answer|reason`
- `--profile quick|standard|full`
- `--quant Q8_0`, `--subfolder ep3`, `--revision refs/pr/1`
- `--seed N` draws a fresh generated set
- `--mode specialist` if the model was trained on `train`

GGUF needs `llama-server` (`brew install llama.cpp`). Native `/v1/systemone` decisions need
llama.cpp 0.9.0 or later. Safetensors need [uv](https://docs.astral.sh/uv/). The first run
installs PyTorch in a cached environment.

<details><summary>Rescoring with Python only</summary>

```bash
pip install datasets
python bench/score.py my-model__standard__20261105-120000.predictions.jsonl
```

`bench/score.py` holds the same rules as the Go scorer, and a golden test file keeps the two
identical. The System 1 part follows typed-decisions' `bench/score_with_jobs.py`.

</details>

## Reading a score

- **System 1 gold is computed by rules, not by a teacher model.** Each workflow builds a
  case from sampled facts and derives the gold distribution from fixed rules. Facts that can
  be read off the state, such as whether two meetings overlap, have a sharp gold. Judgement
  calls, such as which action fits a damaged parcel, keep a soft gold. A score measures
  agreement with those rules. The Prior row, label frequencies from `train` that ignore the
  input, is the floor to beat.
- **System 2 scores generated answers.** The scorer extracts the final answer (the
  `Answer:` line, the leading letter or the last number) after removing `<think>` blocks. A
  think block that never closes means the budget ran out, and that answer counts as wrong.
  **These numbers are not comparable with lm-eval loglikelihood scores** (for example TPN's
  ARC or MMLU `acc_norm`), which compare option probabilities and never let the model write.
- **General and specialist rows are not comparable.** The Spot Snake models were trained on
  Spot's moves with the same teacher and text as `spot_snake`, so they are listed as
  specialists. `spot_snake` has more "pocket" positions than real games, where one move closes
  Spot in. The Spot Snake d1 models mostly keep going straight there, so they score lower than
  their in-game teacher agreement.
- **Coverage** is the share of questions a model answered. A head that only ranks moves
  answers the other questions with nothing, and those are scored as uniform.

## The data

{{COUNTS}}

### System 1 rows

`state` and `questions` together are the body of a `POST /v1/systemone` request. `gold`
holds the gold distribution per question. The flat columns `<question>__label`,
`__probabilities`, `__confidence`, `__score` and `__probability_true` hold the same values.
`factors` describe how the case was built and are not model input.

| Workflow | Questions |
|---|---|
| `spot_snake` | `move` (choice), `eats_now` (noul), `boxed_in_risk` (score 0–3) |
| `it_support` | `category` (choice), `priority` (score), `needs_onsite` (noul) |
| `delivery` | `action` (choice), `notify_customer` (noul), `severity` (score) |
| `code_review` | `decision` (choice), `risk` (score), `needs_tests` (noul) |
| `calendar` | `clash` (noul), `resolution` (choice), `urgency` (score) |

```python
from datasets import load_dataset
import json
row = load_dataset("Lucasgitguy-co/LvW-bench", "decide_all", split="test")[0]
state, questions, gold = (json.loads(row[k]) for k in ("state", "questions", "gold"))
```

### System 2 rows

Each row has `prompt`, `choices` (multiple choice only), `answer` and `answer_type`.
`answer_type` is one of `mc`, `number`, `exact`, `ifeval`, `json` or `mermaid`. The checker
for each type is in `bench/score.py`. `meta` holds what the checker needs: IFEval
instruction ids and kwargs, required JSON values, or required Mermaid edges.

## How it was built

- **Sources:** permissive subsets are sampled with a fixed seed. ARC (CC-BY-SA) and HellaSwag
  (no licence) are left out. TruthfulQA options are shuffled, because upstream always lists
  the correct one first. IFEval keeps only prompts whose instructions can be verified (17
  instruction types).
- **System 1:** `train` comes from different seeds than `test`, and the build refuses to
  package if a state appears in both.
- **Generated set:** 9 families of 25 items each: arithmetic, word problems, letter counts,
  sorting, weekdays and dates, syllogisms, list lookups, JSON output and Mermaid flowcharts.
  Every answer is computed, so `--seed` makes a fresh set no model can have memorised.

## Limits

- The models are small and the sets are small. Read the 95% confidence intervals before
  ranking two close rows.
- System 1 gold follows simple rules written for this benchmark. A model that reasons
  better than the rules can score lower.
- Running a text model on System 1 through first-token probabilities depends on the chat
  template. Backends without logprobs fall back to sampling 8 answers per question, marked
  `sampled`.
- Text only in v0.1. Vision-language models run in text mode.

## Licence

The original data, code and results are MIT. The rows from GSM8K, MMLU and MMLU-Pro stay
MIT, and the rows from TruthfulQA and IFEval stay Apache-2.0. Each row's `license` column
says which applies. See `NOTICE` and `licenses/`. Models keep their own licences.

```bibtex
@misc{lvwbench2026,
  title  = {LvW-bench: a local benchmark for tiny System 1 and System 2 models},
  author = {Van De Weerd, Lucas},
  year   = {2026},
  url    = {https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench}
}
```
