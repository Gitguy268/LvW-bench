# SPDX-License-Identifier: MIT
"""Writes internal/score/golden.json. Decision wants come from bench/score.py (the Python
port of typed-decisions' scorer); generation wants are written by hand below and act as the
spec that Go, JS and Python must all meet. Run: python3 testdata/make_golden.py"""
import json, sys
from pathlib import Path
root = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(root / "bench"))
import score  # noqa: E402

choice = {"type": "choice", "instructions": "Pick", "criteria": {"a": "first", "b": None, "c": "third"}}
sc = {"type": "score", "instructions": "Level", "criteria": ["low", "mid", "high", "max"]}
noul = {"type": "noul", "instructions": "Is it?", "criteria": {"true": "yes", "false": "no"}}
dec_inputs = [
    (choice, {"a": 0.7, "b": 0.2, "c": 0.1}, {"a": 0.6, "b": 0.3, "c": 0.1}),
    (choice, {"a": 0.1, "b": 0.1, "c": 0.8}, {"a": 0.9, "b": 0.05, "c": 0.05}),
    (choice, {}, {"a": 0.2, "b": 0.5, "c": 0.3}),                      # no answer: uniform
    (choice, {"a": 2.0, "b": 2.0}, {"a": 0.5, "b": 0.5, "c": 0.0}),     # unnormalised, tie
    (choice, {"b": 1.0}, {"a": 0.0, "b": 1.0, "c": 0.0}),
    (sc, {"0": 0.1, "1": 0.6, "2": 0.2, "3": 0.1}, {"0": 0.05, "1": 0.75, "2": 0.2, "3": 0.0}),
    (sc, {"3": 1.0}, {"0": 0.9, "1": 0.1, "2": 0.0, "3": 0.0}),          # confident and wrong
    (noul, {"yes": 0.8, "no": 0.2}, {"true": 0.7, "false": 0.3}),       # yes/no keys
    (noul, {"true": 0.4, "false": 0.6}, {"true": 0.97, "false": 0.03}),
    (noul, {"True": 0.55, "False": 0.45}, {"true": 0.5, "false": 0.5}),
]
decisions, scores = [], []
for q, p, g in dec_inputs:
    s = score.score_decision(q, p, g)
    scores.append(s)
    decisions.append({"question": q, "pred": p, "gold": g,
                      "want": {k: s[k] for k in ("correct", "kl", "brier", "confidence")}})
summary = score.summarize_decisions(scores)

def item(t, answer="", choices=None, meta=None, prompt="q"):
    return {"id": "x", "config": "t", "prompt": prompt, "choices": choices or [], "answer": answer,
            "answer_type": t, "meta": meta or {}, "source": "", "license": ""}

abc = ["Paris", "Rome", "Oslo", "Lima"]
gen = [
    (item("mc", "B", abc), "B", True, "B"),
    (item("mc", "B", abc), "The answer is (B).", True, "B"),
    (item("mc", "B", abc), "<think>maybe A? no.</think>\nAnswer: B", True, "B"),
    (item("mc", "B", abc), "<think>A or B, I am still thinking", False, ""),
    (item("mc", "C", abc), "It is Oslo.", True, "C"),
    (item("mc", "C", abc), "Paris or Oslo", False, ""),
    (item("mc", "A", abc), "**A**", True, "A"),
    (item("mc", "B", abc), "E", False, ""),
    (item("mc", "A", abc), "A capital city of France is Paris.", True, "A"),
    (item("mc", "B", abc), "A city in Italy", False, ""),
    (item("mc", "D", abc), "D) Lima", True, "D"),
    (item("number", "72"), "She has 72 apples in total.", True, "72"),
    (item("number", "1234"), "So 1,000 + 234.\nAnswer: 1,234", True, "1234"),
    (item("number", "18"), "Answer: $18.00", True, "18"),
    (item("number", "-5"), "The result is -5", True, "-5"),
    (item("number", "7"), "I think 8", False, "8"),
    (item("exact", "Thursday"), "Answer: thursday.", True, "thursday"),
    (item("exact", "anchor, maple, violet"), "anchor,maple ,violet", True, "anchor, maple, violet"),
    (item("exact", "2026-03-04"), "Let me count.\nAnswer: 2026-03-05", False, "2026-03-05"),
    (item("json", meta={"values": {"name": "Ada", "age": 36}}), "```json\n{\"name\": \"Ada\", \"age\": 36}\n```", True, ""),
    (item("json", meta={"values": {"name": "Ada", "age": 36}}), "Sure! {\"name\":\"ada\",\"age\":36.0,\"x\":1}", True, ""),
    (item("json", meta={"values": {"name": "Ada", "age": 36}}), "{\"name\": \"Ada\"}", False, ""),
    (item("mermaid", meta={"edges": [["Start", "Boil water"], ["Boil water", "Done"]]}),
     "```mermaid\nflowchart TD\n  A[Start] --> B[Boil water]\n  B --> C[Done]\n```", True, ""),
    (item("mermaid", meta={"edges": [["Start", "Done"]]}), "graph LR\nStart --> Done", True, ""),
    (item("mermaid", meta={"edges": [["Start", "Done"]]}), "A[Start] --> B[Done]", False, ""),
    (item("ifeval", meta={"instruction_id_list": ["punctuation:no_comma", "change_case:english_lowercase"], "kwargs": [{}, {}]}),
     "all lower case and no commas here", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["punctuation:no_comma"], "kwargs": [{}]}), "one, two", False, ""),
    (item("ifeval", meta={"instruction_id_list": ["length_constraints:number_words"], "kwargs": [{"relation": "at least", "num_words": 5}]}),
     "one two three four five", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["length_constraints:number_words"], "kwargs": [{"relation": "less than", "num_words": 3}]}),
     "one two three", False, ""),
    (item("ifeval", meta={"instruction_id_list": ["detectable_format:number_bullet_lists"], "kwargs": [{"num_bullets": 2}]}),
     "Intro\n* first\n- second", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["detectable_format:title", "detectable_content:postscript"], "kwargs": [{}, {"postscript_marker": "P.S."}]}),
     "<<My Title>>\nBody text.\nP.S. bye", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["detectable_format:number_highlighted_sections"], "kwargs": [{"num_highlights": 2}]}),
     "*one* and **two**", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["keywords:existence", "keywords:forbidden_words"], "kwargs": [{"keywords": ["Spot"]}, {"forbidden_words": ["cat"]}]}),
     "spot is a dog, catalogue is fine", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["keywords:frequency", "keywords:letter_frequency"],
                          "kwargs": [{"keyword": "dog", "frequency": 2, "relation": "at least"}, {"letter": "z", "let_frequency": 1, "let_relation": "less than"}]}),
     "Dog dog dog", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["length_constraints:number_paragraphs"], "kwargs": [{"num_paragraphs": 2}]}),
     "First part.\n***\nSecond part.", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["detectable_format:json_format", "startend:end_checker"], "kwargs": [{}, {"end_phrase": "}"}]}),
     "```json\n{\"a\": 1}\n```", False, ""),
    (item("ifeval", meta={"instruction_id_list": ["startend:quotation", "change_case:english_capital"], "kwargs": [{}, {}]}),
     "\"ALL CAPS HERE\"", True, ""),
    (item("ifeval", meta={"instruction_id_list": ["detectable_content:number_placeholders"], "kwargs": [{"num_placeholders": 2}]}),
     "Dear [name], at [address]", True, ""),
]
gen_out = []
for it, out, ok, ex in gen:
    got_ok, got_ex = score.check_answer(it, out)
    if (got_ok, got_ex) != (ok, ex):
        raise SystemExit(f"python scorer disagrees with the spec: {it['answer_type']} {out!r}: {got_ok} {got_ex!r}, want {ok} {ex!r}")
    gen_out.append({"item": it, "output": out, "want": {"correct": ok, "extracted": ex}})
golden = {"decisions": decisions, "summary": {"want": summary}, "gen": gen_out}
(root / "internal/score/golden.json").write_text(json.dumps(golden, indent=1) + "\n")
print("golden.json:", len(decisions), "decisions,", len(gen_out), "generation cases")
