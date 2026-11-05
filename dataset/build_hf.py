# SPDX-License-Identifier: MIT
"""Build the Hugging Face dataset repo for LvW-bench in dist/hf/.

  uv run --python 3.12 --with pyarrow --with matplotlib dataset/build_hf.py [path/to/lvwb]

Writes parquet files per config and split, README.md (the dataset card with the leaderboard
from results/), eval.yaml, the bench/ scorer, licences and assets/leaderboard.png. It
refuses to build if a System One state appears in both train and test.
"""
import json
import shutil
import subprocess
import sys
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "dist" / "hf"
LVWB = sys.argv[1] if len(sys.argv) > 1 else str(ROOT / ".bin" / "lvwb")

DECIDE = ["spot_snake", "it_support", "delivery", "code_review", "calendar"]
GEN = ["gsm8k", "mmlu", "mmlu_pro", "truthfulqa", "ifeval", "generated"]


def run(*args):
    return subprocess.run([LVWB, *args], check=True, capture_output=True, text=True).stdout


def main():
    if OUT.exists():
        shutil.rmtree(OUT)
    raw = OUT.parent / "hf-jsonl"
    if raw.exists():
        shutil.rmtree(raw)
    run("dataset", str(raw))
    configs = []
    counts = {}
    for cfg_dir in sorted(raw.iterdir()):
        cfg = cfg_dir.name
        files = []
        for split_file in sorted(cfg_dir.glob("*.jsonl")):
            split = split_file.stem
            rows = [json.loads(l) for l in split_file.read_text().splitlines() if l]
            table = pa.Table.from_pylist(rows)
            dst = OUT / cfg / f"{split}-00000-of-00001.parquet"
            dst.parent.mkdir(parents=True, exist_ok=True)
            pq.write_table(table, dst, compression="zstd")
            files.append({"split": split, "path": f"{cfg}/{split}-*.parquet"})
            counts[(cfg, split)] = len(rows)
        configs.append({"config_name": cfg, "data_files": files})
    # train / test must not share a state
    for wf in DECIDE + ["decide_all"]:
        tr = {json.loads(l)["state"] for l in (raw / wf / "train.jsonl").read_text().splitlines() if l}
        te = {json.loads(l)["state"] for l in (raw / wf / "test.jsonl").read_text().splitlines() if l}
        if tr & te:
            raise SystemExit(f"{wf}: {len(tr & te)} states are in both train and test")
    order = DECIDE + ["decide_all"] + GEN + ["generative_all", "reason_subset"]
    configs.sort(key=lambda c: order.index(c["config_name"]))
    configs[order.index("decide_all")]["default"] = True

    board_md = run("leaderboard", "--md", str(ROOT / "results"))
    board = json.loads(run("leaderboard", "--json", str(ROOT / "results")))
    plot(board, OUT / "assets" / "leaderboard.png")

    yaml_configs = []
    for c in configs:
        lines = [f"- config_name: {c['config_name']}"]
        if c.get("default"):
            lines.append("  default: true")
        lines.append("  data_files:")
        for f in c["data_files"]:
            lines += [f"  - split: {f['split']}", f"    path: {f['path']}"]
        yaml_configs.append("\n".join(lines))
    counts_md = counts_table(counts)
    card = (ROOT / "dataset" / "card.md").read_text()
    card = (card.replace("{{CONFIGS}}", "\n".join(yaml_configs))
                .replace("{{LEADERBOARD}}", board_md.strip())
                .replace("{{COUNTS}}", counts_md)
                .replace("{{VERSION}}", board["version"]))
    (OUT / "README.md").write_text(card)
    shutil.copy(ROOT / "dataset" / "eval.yaml", OUT / "eval.yaml")
    (OUT / "bench").mkdir()
    shutil.copy(ROOT / "bench" / "score.py", OUT / "bench" / "score.py")
    shutil.copy(ROOT / "bench" / "submit_results.md", OUT / "bench" / "README.md")
    (OUT / "results").mkdir()
    for f in sorted((ROOT / "results").glob("*")):
        shutil.copy(f, OUT / "results" / f.name)
    for name in ["LICENSE", "NOTICE"]:
        shutil.copy(ROOT / name, OUT / name)
    shutil.copytree(ROOT / "licenses", OUT / "licenses")
    shutil.copy(ROOT / "dataset" / "gitattributes", OUT / ".gitattributes")
    shutil.rmtree(raw)
    print("built", OUT, sum(counts.values()), "rows in", len(configs), "configs")


def counts_table(counts):
    rows = ["| Config | Track | Train | Test | Source | Licence |", "|---|---|---|---|---|---|"]
    src = {
        "spot_snake": ("decide", "original, rule gold", "MIT"), "it_support": ("decide", "original, rule gold", "MIT"),
        "delivery": ("decide", "original, rule gold", "MIT"), "code_review": ("decide", "original, rule gold", "MIT"),
        "calendar": ("decide", "original, rule gold", "MIT"), "decide_all": ("decide", "all five workflows", "MIT"),
        "gsm8k": ("answer, reason", "openai/gsm8k test", "MIT"), "mmlu": ("answer, reason", "cais/mmlu test, 4 per subject", "MIT"),
        "mmlu_pro": ("answer, reason", "TIGER-Lab/MMLU-Pro test, 10 per category", "MIT"),
        "truthfulqa": ("answer, reason", "truthfulqa/truthful_qa mc1, options shuffled", "Apache-2.0"),
        "ifeval": ("answer, reason", "google/IFEval, verifiable subset", "Apache-2.0"),
        "generated": ("answer, reason", "original, seeded generator (seed 7)", "MIT"),
        "generative_all": ("answer", "the six sets above", "mixed, see rows"),
        "reason_subset": ("reason", "40 per set, the items the reason leaderboard runs", "mixed, see rows"),
    }
    for cfg in DECIDE + ["decide_all"] + GEN + ["generative_all", "reason_subset"]:
        t, s, l = src[cfg]
        rows.append(f"| `{cfg}` | {t} | {counts.get((cfg, 'train'), '–')} | {counts.get((cfg, 'test'), '–')} | {s} | {l} |")
    return "\n".join(rows)


def plot(board, path):
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    path.parent.mkdir(parents=True, exist_ok=True)
    fig, axes = plt.subplots(1, 2, figsize=(12, 5), dpi=120)
    ink, muted, accent, spec = "#1f2328", "#8b949e", "#2f6fb3", "#c9822b"
    ax = axes[0]
    for r in board["tracks"].get("decide", []):
        t = r["score"]
        c = spec if r["mode"] == "specialist" else accent
        ax.scatter(t["kl_from_gold"], t["accuracy"], color=c, s=40, zorder=3)
        ax.annotate(r["label"].split(" · ")[0], (t["kl_from_gold"], t["accuracy"]), fontsize=7, color=ink,
                    xytext=(4, 3), textcoords="offset points")
    for name, t in board["reference"].items():
        ax.scatter(t["kl_from_gold"], t["accuracy"], color=muted, marker="x", s=40, zorder=3)
        ax.annotate(name, (t["kl_from_gold"], t["accuracy"]), fontsize=7, color=muted, xytext=(4, 3), textcoords="offset points")
    ax.set_xlabel("KL from gold (lower is better)")
    ax.set_ylabel("Accuracy")
    ax.set_title("System 1 · decide", loc="left", fontsize=11)
    ax = axes[1]
    ans = {r["label"]: r["score"]["accuracy"] for r in board["tracks"].get("answer", [])}
    rea = {r["label"]: r["score"]["accuracy"] for r in board["tracks"].get("reason", [])}
    for label in ans:
        if label in rea:
            ax.scatter(ans[label], rea[label], color=accent, s=40, zorder=3)
            ax.annotate(label.split(" · ")[0], (ans[label], rea[label]), fontsize=7, color=ink, xytext=(4, 3), textcoords="offset points")
    lim = max([0.05] + list(ans.values()) + list(rea.values())) * 1.15
    ax.plot([0, lim], [0, lim], color=muted, lw=0.8, ls="--", zorder=1)
    ax.set_xlim(0, lim)
    ax.set_ylim(0, lim)
    ax.set_xlabel("answer accuracy (thinking off)")
    ax.set_ylabel("reason accuracy (thinking on)")
    ax.set_title("System 2 · answer vs reason", loc="left", fontsize=11)
    for a in axes:
        a.grid(alpha=0.25)
        for s in ("top", "right"):
            a.spines[s].set_visible(False)
    fig.tight_layout()
    fig.savefig(path)


if __name__ == "__main__":
    main()
