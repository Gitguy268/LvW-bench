# SPDX-License-Identifier: MIT
"""Pull the permissively licensed source subsets that LvW-bench embeds.

Run: uv run --python 3.12 --with datasets dataset/fetch_sources.py
Writes internal/tasks/data/<config>.jsonl.gz. Sampling is seeded, so a rerun gives the same items.
Only MIT / Apache-2.0 sources are used (ARC is CC-BY-SA-4.0 and HellaSwag has no licence on its card,
so both are left out).
"""
import gzip
import json
import random
import re
from collections import defaultdict
from pathlib import Path

from datasets import load_dataset

SEED = 20261105
OUT = Path(__file__).resolve().parents[1] / "internal" / "tasks" / "data"
LETTERS = "ABCDEFGHIJ"

# IFEval instructions LvW-bench can verify (same list in Go, JS and Python scorers).
IFEVAL_SUPPORTED = {
    "punctuation:no_comma",
    "change_case:english_lowercase",
    "change_case:english_capital",
    "length_constraints:number_words",
    "length_constraints:number_paragraphs",
    "detectable_format:number_bullet_lists",
    "detectable_format:json_format",
    "detectable_format:title",
    "detectable_format:number_highlighted_sections",
    "keywords:existence",
    "keywords:forbidden_words",
    "keywords:frequency",
    "keywords:letter_frequency",
    "startend:end_checker",
    "startend:quotation",
    "detectable_content:postscript",
    "detectable_content:number_placeholders",
}


def write(name, rows):
    OUT.mkdir(parents=True, exist_ok=True)
    with gzip.open(OUT / f"{name}.jsonl.gz", "wt", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False, sort_keys=True) + "\n")
    print(name, len(rows))


def round_robin(groups, per_group, rng):
    out = []
    for key in sorted(groups):
        items = groups[key][:]
        rng.shuffle(items)
        out.extend(items[:per_group])
    return out


def gsm8k():
    rng = random.Random(SEED)
    ds = list(load_dataset("openai/gsm8k", "main", split="test"))
    rng.shuffle(ds)
    rows = []
    for i, r in enumerate(ds[:200]):
        ans = r["answer"].split("####")[-1].strip().replace(",", "")
        rows.append({"id": f"gsm8k_{i:04d}", "config": "gsm8k", "prompt": r["question"], "choices": [],
                     "answer": ans, "answer_type": "number", "meta": {},
                     "source": "openai/gsm8k (test)", "license": "MIT"})
    write("gsm8k", rows)


def mmlu():
    rng = random.Random(SEED)
    groups = defaultdict(list)
    for r in load_dataset("cais/mmlu", "all", split="test"):
        groups[r["subject"]].append(r)
    rows = []
    for i, r in enumerate(round_robin(groups, 4, rng)):
        rows.append({"id": f"mmlu_{i:04d}", "config": "mmlu", "prompt": r["question"], "choices": list(r["choices"]),
                     "answer": LETTERS[r["answer"]], "answer_type": "mc", "meta": {"subject": r["subject"]},
                     "source": "cais/mmlu (test)", "license": "MIT"})
    write("mmlu", rows)


def mmlu_pro():
    rng = random.Random(SEED)
    groups = defaultdict(list)
    for r in load_dataset("TIGER-Lab/MMLU-Pro", split="test"):
        groups[r["category"]].append(r)
    rows = []
    for i, r in enumerate(round_robin(groups, 10, rng)):
        rows.append({"id": f"mmlu_pro_{i:04d}", "config": "mmlu_pro", "prompt": r["question"],
                     "choices": list(r["options"]), "answer": LETTERS[r["answer_index"]], "answer_type": "mc",
                     "meta": {"category": r["category"]}, "source": "TIGER-Lab/MMLU-Pro (test)", "license": "MIT"})
    write("mmlu_pro", rows)


def truthfulqa():
    rng = random.Random(SEED)
    ds = list(load_dataset("truthfulqa/truthful_qa", "multiple_choice", split="validation"))
    rng.shuffle(ds)
    rows = []
    for i, r in enumerate(ds[:150]):
        choices = list(r["mc1_targets"]["choices"])
        correct = choices[r["mc1_targets"]["labels"].index(1)]
        choices = choices[:10]
        if correct not in choices:
            choices[-1] = correct
        rng.shuffle(choices)  # the correct answer is always first upstream
        rows.append({"id": f"truthfulqa_{i:04d}", "config": "truthfulqa", "prompt": r["question"], "choices": choices,
                     "answer": LETTERS[choices.index(correct)], "answer_type": "mc", "meta": {},
                     "source": "truthfulqa/truthful_qa (multiple_choice, mc1, validation)", "license": "Apache-2.0"})
    write("truthfulqa", rows)


def ifeval():
    rng = random.Random(SEED)
    ds = [r for r in load_dataset("google/IFEval", split="train")
          if all(i in IFEVAL_SUPPORTED for i in r["instruction_id_list"])]
    rng.shuffle(ds)
    rows = []
    for i, r in enumerate(ds[:200]):
        kwargs = [{k: v for k, v in kw.items() if v is not None} for kw in r["kwargs"]]
        rows.append({"id": f"ifeval_{i:04d}", "config": "ifeval", "prompt": r["prompt"], "choices": [],
                     "answer": "", "answer_type": "ifeval",
                     "meta": {"instruction_id_list": r["instruction_id_list"], "kwargs": kwargs, "key": r["key"]},
                     "source": "google/IFEval (train)", "license": "Apache-2.0"})
    write("ifeval", rows)


if __name__ == "__main__":
    gsm8k()
    mmlu()
    mmlu_pro()
    truthfulqa()
    ifeval()
