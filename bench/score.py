# SPDX-License-Identifier: MIT
"""LvW-bench scorer in Python: rescore an lvwb predictions file, or import the functions.

The rules match internal/score (Go) and web/js/score.js; internal/score/golden.json keeps
all three identical. The decision scorer follows LocalLLaMA/typed-decisions
bench/score_with_jobs.py (Apache-2.0).

  python bench/score.py RESULT.predictions.jsonl [--data DIR]

DIR holds the dataset JSONL/parquet (default: download Lucasgitguy-co/LvW-bench with `datasets`).
"""
import argparse
import json
import math
import random
import re
import sys
from collections import defaultdict

LETTERS = "ABCDEFGHIJ"


# --- System One -----------------------------------------------------------------------------

def options(q):
    if q["type"] == "noul":
        return ["true", "false"]
    if q["type"] == "score":
        return [str(i) for i in range(len(q["criteria"]))]
    return list(q["criteria"])


def _noul(typ, m):
    if typ != "noul":
        return m
    out = {}
    for k, v in m.items():
        k = {"yes": "true", "Yes": "true", "True": "true", "no": "false", "No": "false", "False": "false"}.get(k, k)
        out[k] = out.get(k, 0.0) + v
    return out


def _argmax(v):
    best = 0
    for i, x in enumerate(v):
        if x > v[best]:
            best = i
    return best


def score_decision(q, pred, gold):
    opts = options(q)
    pred, gold = _noul(q["type"], pred or {}), _noul(q["type"], gold)
    g = [float(gold.get(o, 0.0)) for o in opts]
    p = [max(float(pred.get(o, 0.0)), 0.0) for o in opts]
    s = sum(p)
    p = [x / s for x in p] if s > 0 else [1.0 / len(opts)] * len(opts)
    kl = sum(gi * math.log(gi / max(pi, 1e-9)) for gi, pi in zip(g, p) if gi > 0)
    brier = sum((pi - gi) ** 2 for pi, gi in zip(p, g))
    return {"correct": _argmax(p) == _argmax(g), "kl": kl, "brier": brier, "confidence": p[_argmax(p)],
            "probabilities": dict(zip(opts, p))}


def summarize_decisions(scores):
    n = len(scores)
    if not n:
        return {"n_decisions": 0, "accuracy": 0, "kl_from_gold": 0, "brier": 0, "ece": 0}
    bins = 10
    hits, total, conf = [0.0] * bins, [0.0] * bins, [0.0] * bins
    for s in scores:
        b = min(int(s["confidence"] * bins), bins - 1)
        total[b] += 1
        conf[b] += s["confidence"]
        hits[b] += s["correct"]
    ece = sum(total[b] / n * abs(hits[b] / total[b] - conf[b] / total[b]) for b in range(bins) if total[b])
    return {"n_decisions": n, "accuracy": sum(s["correct"] for s in scores) / n,
            "kl_from_gold": sum(s["kl"] for s in scores) / n, "brier": sum(s["brier"] for s in scores) / n, "ece": ece}


# --- System Two -----------------------------------------------------------------------------

RE_THINK = re.compile(r"<think>.*?</think>", re.S)
RE_ANSWER_MC = re.compile(r"answer\s*(?:is)?\s*[:：]?\s*\**\(?([A-J])\)?\**(?:[^A-Za-z]|$)", re.I)
RE_LEAD_MC = re.compile(r"^\**\(?([A-J])\)?\**(?:[.):]|\s*$)")
RE_ANSWER_NUM = re.compile(r"answer\s*(?:is)?\s*[:：]?\s*\**\s*\$?\s*(-?[\d,]*\.?\d+)", re.I)
RE_NUMBER = re.compile(r"-?[\d,]*\.?\d+")
RE_ANSWER_LINE = re.compile(r"^\s*\**answer\**\s*[:：]\s*(.+?)\s*$", re.I | re.M)
RE_FENCED = re.compile(r"```[a-zA-Z]*\s*\n(.*?)```", re.S)
RE_COMMA = re.compile(r"\s*,\s*")


def strip_thinking(s):
    i = s.rfind("</think>")
    if i >= 0:
        return s[i + len("</think>"):].strip(), False
    if "<think>" in s:
        return "", True
    return RE_THINK.sub("", s).strip(), False


def norm_number(s):
    s = s.replace(",", "")
    try:
        f = float(s)
    except ValueError:
        return s
    if f == int(f) and abs(f) < 1e15:
        return str(int(f))
    return repr(f)


def norm_text(s):
    s = s.strip().lower().strip(" .!*\"'`")
    s = " ".join(s.split())
    return RE_COMMA.sub(", ", s)


def _extract_mc(text, n, choices):
    def valid(l):
        return l != "" and LETTERS.index(l) < n
    m = RE_ANSWER_MC.findall(text)
    if m and valid(m[-1]):
        return m[-1]
    t = text.strip()
    m = RE_LEAD_MC.match(t)
    if m and valid(m.group(1)):
        return m.group(1)
    low, found = t.lower(), ""
    for i, c in enumerate(choices):
        c = c.strip().lower()
        if c and c in low:
            if found:
                return ""
            found = LETTERS[i]
    return found


def _extract_number(text):
    m = RE_ANSWER_NUM.findall(text)
    if m:
        return norm_number(m[-1])
    m = RE_NUMBER.findall(text)
    return norm_number(m[-1]) if m else ""


def extract(item, output):
    text, _ = strip_thinking(output)
    t = item["answer_type"]
    if t == "mc":
        return _extract_mc(text, len(item["choices"]), item["choices"])
    if t == "number":
        return _extract_number(text)
    if t == "exact":
        m = RE_ANSWER_LINE.findall(text)
        if m:
            return norm_text(m[-1])
        return norm_text(text.strip().split("\n")[-1])
    return text


def _first_json(text):
    cands = [m for m in RE_FENCED.findall(text)] + [text]
    for c in cands:
        start = c.find("{")
        while start >= 0:
            depth = 0
            for i in range(start, len(c)):
                if c[i] == "{":
                    depth += 1
                elif c[i] == "}":
                    depth -= 1
                if depth == 0:
                    try:
                        v = json.loads(c[start:i + 1])
                        if isinstance(v, dict):
                            return v
                    except ValueError:
                        pass
                    break
            nxt = c.find("{", start + 1)
            if nxt < 0:
                break
            start = nxt
    return None


def _json_equal(a, b):
    if json.dumps(a, sort_keys=True) == json.dumps(b, sort_keys=True):
        return True
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool) and not isinstance(b, bool):
        return float(a) == float(b)
    return isinstance(a, str) and isinstance(b, str) and norm_text(a) == norm_text(b)


RE_M_HEAD = re.compile(r"^(?:flowchart|graph)\s+(?:TD|TB|LR|RL|BT)\s*;?$")
RE_M_EDGE = re.compile(r"([A-Za-z0-9_]+)\s*(?:\[[^\]]*\]|\([^)]*\)|\{[^}]*\})?\s*(?:-->|---|-\.->|==>)\s*(?:\|[^|]*\|\s*)?([A-Za-z0-9_]+)")
RE_M_NODE = re.compile(r"([A-Za-z0-9_]+)\s*(?:\[([^\]]*)\]|\(([^)]*)\)|\{([^}]*)\})")


def _check_mermaid(item, text):
    m = RE_FENCED.search(text)
    block = m.group(1) if m else text
    lines = [l.strip() for l in block.split("\n") if l.strip() and not l.strip().startswith("%%")]
    head = next((i for i, l in enumerate(lines) if RE_M_HEAD.match(l)), -1)
    if head < 0:
        return False
    label, edges = {}, set()
    for l in lines[head + 1:]:
        for mm in RE_M_NODE.finditer(l):
            for g in mm.groups()[1:]:
                if g:
                    label[mm.group(1)] = norm_text(g.strip("\"'"))
        for mm in RE_M_EDGE.finditer(l):
            edges.add((mm.group(1), mm.group(2)))
    name = lambda i: label.get(i) or norm_text(i)
    have = {(name(a), name(b)) for a, b in edges}
    want = item["meta"].get("edges") or []
    return bool(want) and all((norm_text(a), norm_text(b)) in have for a, b in want)


def check_answer(item, output):
    text, _ = strip_thinking(output)
    t = item["answer_type"]
    if t in ("mc", "number", "exact"):
        e = extract(item, output)
        if t == "mc":
            return e == item["answer"], e
        if t == "number":
            try:
                return abs(float(e) - float(norm_number(item["answer"]))) < 1e-6, e
            except ValueError:
                return False, e
        accept = [item["answer"]] + list(item["meta"].get("accept") or [])
        return any(e == norm_text(a) for a in accept), e
    if t == "ifeval":
        return check_ifeval(item, text), ""
    if t == "json":
        obj = _first_json(text)
        want = item["meta"].get("values") or {}
        return obj is not None and all(k in obj and _json_equal(obj[k], v) for k, v in want.items()), ""
    if t == "mermaid":
        return _check_mermaid(item, text), ""
    return False, ""


# --- IFEval subset (google-research instruction_following_eval, Apache-2.0) ------------------

def _rel(rel, actual, want):
    return actual < want if rel == "less than" else actual >= want


def _cased(s, upper):
    cased = False
    for ch in s:
        if ch.isalpha() and ch.upper() != ch.lower():
            cased = True
            if (upper and ch.islower()) or (not upper and ch.isupper()):
                return False
    return cased


def check_instruction(iid, kw, v):
    A = re.ASCII
    if iid == "punctuation:no_comma":
        return "," not in v
    if iid == "change_case:english_lowercase":
        return _cased(v, False)
    if iid == "change_case:english_capital":
        return _cased(v, True)
    if iid == "length_constraints:number_words":
        return _rel(kw.get("relation"), len(re.findall(r"\w+", v, A)), int(kw.get("num_words", 0)))
    if iid == "length_constraints:number_paragraphs":
        paras = re.split(r"\s?\*\*\*\s?", v)
        n = len(paras)
        for i, p in enumerate(paras):
            if not p.strip():
                if i == 0 or i == len(paras) - 1:
                    n -= 1
                else:
                    return False
        return n == int(kw.get("num_paragraphs", 0))
    if iid == "detectable_format:number_bullet_lists":
        n = len(re.findall(r"^\s*\*[^\*].*$", v, re.M)) + len(re.findall(r"^\s*-.*$", v, re.M))
        return n == int(kw.get("num_bullets", 0))
    if iid == "detectable_format:json_format":
        s = v.strip()
        for p in ("```json", "```Json", "```JSON", "```"):
            if s.startswith(p):
                s = s[len(p):]
                break
        s = s.strip()
        if s.endswith("```"):
            s = s[:-3]
        try:
            json.loads(s.strip())
            return True
        except ValueError:
            return False
    if iid == "detectable_format:title":
        return any(t.strip("<>").strip() for t in re.findall(r"<<[^\n]+>>", v))
    if iid == "detectable_format:number_highlighted_sections":
        n = sum(1 for h in re.findall(r"\*[^\n\*]*\*", v) if h.strip("*").strip())
        n += sum(1 for h in re.findall(r"\*\*[^\n\*]*\*\*", v) if h[2:-2].strip())
        return n >= int(kw.get("num_highlights", 0))
    if iid == "keywords:existence":
        return all(re.search(re.escape(k), v, re.I) for k in kw.get("keywords", []))
    if iid == "keywords:forbidden_words":
        return not any(re.search(r"\b" + re.escape(k) + r"\b", v, re.I | A) for k in kw.get("forbidden_words", []))
    if iid == "keywords:frequency":
        return _rel(kw.get("relation"), len(re.findall(re.escape(kw.get("keyword", "")), v, re.I)), int(kw.get("frequency", 0)))
    if iid == "keywords:letter_frequency":
        l = (kw.get("letter") or "").lower()
        return bool(l) and _rel(kw.get("let_relation"), v.lower().count(l), int(kw.get("let_frequency", 0)))
    if iid == "startend:end_checker":
        return v.strip().strip('"').lower().endswith((kw.get("end_phrase") or "").strip().lower())
    if iid == "startend:quotation":
        s = v.strip()
        return len(s) > 1 and s[0] == '"' and s[-1] == '"'
    if iid == "detectable_content:postscript":
        m = kw.get("postscript_marker", "")
        if m == "P.P.S":
            pat = r"\s*p\.\s?p\.\s?s.*$"
        elif m == "P.S.":
            pat = r"\s*p\.\s?s\..*$"
        else:
            pat = r"\s*" + re.escape(m.lower()) + r".*$"
        return bool(re.findall(pat, v.lower(), re.M))
    if iid == "detectable_content:number_placeholders":
        return len(re.findall(r"\[.*?\]", v)) >= int(kw.get("num_placeholders", 0))
    return False


def check_ifeval(item, text):
    ids = item["meta"].get("instruction_id_list") or []
    kws = item["meta"].get("kwargs") or []
    return bool(ids) and all(check_instruction(i, kws[n] if n < len(kws) else {}, text) for n, i in enumerate(ids))


def bootstrap_ci(xs, draws=1000):
    """95% percentile interval of the mean. Python's RNG differs from Go's, so the bounds can
    differ in the third decimal from lvwb's; the accuracy itself is identical."""
    if not xs:
        return 0.0, 0.0
    r = random.Random(1)
    n = len(xs)
    means = sorted(sum(xs[r.randrange(n)] for _ in range(n)) / n for _ in range(draws))
    return means[25], means[974]


# --- CLI -------------------------------------------------------------------------------------

def _load_rows(data, config, split):
    if data:
        from pathlib import Path
        p = Path(data) / config / f"{split}.jsonl"
        if p.exists():
            return [json.loads(l) for l in p.read_text().splitlines() if l]
        import pyarrow.parquet as pq
        return pq.read_table(str(Path(data) / config / f"{split}-00000-of-00001.parquet")).to_pylist()
    from datasets import load_dataset
    return list(load_dataset("Lucasgitguy-co/LvW-bench", config, split=split))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("predictions")
    ap.add_argument("--data", default="")
    args = ap.parse_args()
    preds = [json.loads(l) for l in open(args.predictions) if l.strip()]
    cache = {}

    def rows(config, split):
        if (config, split) not in cache:
            cache[(config, split)] = {r["id"]: r for r in _load_rows(args.data, config, split)}
        return cache[(config, split)]

    dec, gen = defaultdict(list), defaultdict(list)
    for p in preds:
        if p["track"] == "decide":
            row = rows(p["config"], "test")[p["id"]]
            q = json.loads(row["questions"])[p["question"]]
            gold = json.loads(row["gold"])[p["question"]]["probabilities"]
            dec["all"].append(score_decision(q, p.get("probabilities") or {}, gold))
        else:
            row = dict(rows(p["config"], "test")[p["id"]])
            row["meta"] = json.loads(row["meta"]) if isinstance(row["meta"], str) else row["meta"]
            ok, _ = check_answer(row, p.get("output") or "")
            gen[p["track"]].append(1.0 if ok else 0.0)
    out = {}
    if dec:
        out["decide"] = summarize_decisions(dec["all"])
    for t, xs in gen.items():
        lo, hi = bootstrap_ci(xs)
        out[t] = {"n": len(xs), "accuracy": sum(xs) / len(xs), "ci_low": lo, "ci_high": hi}
    json.dump(out, sys.stdout, indent=2)
    print()


if __name__ == "__main__":
    main()
