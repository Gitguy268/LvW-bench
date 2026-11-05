// node web/test/golden.test.mjs — checks web/js/score.js against internal/score/golden.json
import { readFileSync } from "node:fs";
import { scoreDecision, summarizeDecisions, checkAnswer } from "../js/score.js";

const g = JSON.parse(readFileSync(new URL("../../internal/score/golden.json", import.meta.url)));
const near = (a, b) => Math.abs(a - b) < 1e-6;
let fails = 0;
const scores = [];
g.decisions.forEach((d, i) => {
  const s = scoreDecision(d.question, d.pred, d.gold);
  scores.push(s);
  if (s.correct !== d.want.correct || !near(s.kl, d.want.kl) || !near(s.brier, d.want.brier) || !near(s.confidence, d.want.confidence)) {
    console.log("FAIL decision", i, s, d.want); fails++;
  }
});
const sum = summarizeDecisions(scores), w = g.summary.want;
for (const k of ["accuracy", "kl_from_gold", "brier", "ece"]) if (!near(sum[k], w[k])) { console.log("FAIL summary", k, sum[k], w[k]); fails++; }
g.gen.forEach((x, i) => {
  const [ok, ex] = checkAnswer(x.item, x.output);
  if (ok !== x.want.correct || ex !== x.want.extracted) { console.log("FAIL gen", i, x.item.answer_type, JSON.stringify(x.output), ok, ex, x.want); fails++; }
});
console.log(fails ? `${fails} failures` : `ok: ${g.decisions.length + g.gen.length + 1} checks`);
process.exit(fails ? 1 : 0);
