// SPDX-License-Identifier: MIT
// The LvW-bench tracks for the browser: the same prompts, adapters and budgets as
// internal/runner (Go). `engine` is any object with
//   chat(params)      -> OpenAI chat.completion response (wllama, WebLLM)
//   systemone(params) -> { answers } or throws when the model is not a decision model
import { options, scoreDecision, summarizeDecisions, checkAnswer, stripThinking, bootstrapCI } from "./score.js";

const LETTERS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";

function stateText(state) {
  if (typeof state === "string") return state;
  return JSON.stringify(state, null, 2);
}

function descriptions(q) {
  if (q.type === "score") return Object.fromEntries(q.criteria.map((d, i) => [String(i), d]));
  return Object.fromEntries(Object.entries(q.criteria || {}).map(([k, v]) => [k, v || ""]));
}

const dash = (s) => (s ? " — " + s : "");

export function decidePrompt(state, q) {
  const opts = options(q), desc = descriptions(q);
  let b = "Read the state and answer the question.\n\nState:\n" + stateText(state) + "\n\n";
  let labels;
  if (q.type === "noul") {
    b += `Statement: ${q.instructions}\n\nIs the statement true for this state?\n`;
    labels = ["A", "B"];
    b += `A) true${dash(desc.true)}\nB) false${dash(desc.false)}\n\nReply with A or B only.`;
  } else if (q.type === "score") {
    b += `Question: ${q.instructions}\n\nLevels:\n`;
    labels = opts.slice();
    for (const o of opts) b += `${o}) ${desc[o]}\n`;
    b += "\nReply with the level number only.";
  } else {
    b += `Question: ${q.instructions}\n\nOptions:\n`;
    labels = opts.map((_, i) => LETTERS[i]);
    opts.forEach((o, i) => (b += `${labels[i]}) ${o}${dash(desc[o])}\n`));
    b += "\nReply with the letter of the best option only.";
  }
  return [b, labels];
}

export function labelProbs(top, labels, opts) {
  const idx = Object.fromEntries(labels.map((l, i) => [l, i]));
  const p = opts.map(() => 0);
  let mass = 0;
  for (const [tok, v] of Object.entries(top)) {
    const t = tok.trim().replace(/^[()*.:]+|[()*.:]+$/g, "");
    if (t in idx) { p[idx[t]] += v; mass += v; }
  }
  return [Object.fromEntries(opts.map((o, i) => [o, p[i]])), mass > 0];
}

export function genPrompt(it, track) {
  let b = it.prompt;
  if (it.choices && it.choices.length) b += "\n\n" + it.choices.map((c, i) => `${LETTERS[i]}) ${c}`).join("\n") + "\n";
  const short = ["mc", "number", "exact"].includes(it.answer_type);
  if (!short) return b.trim();
  const what = { mc: "the letter of the correct option", number: "the final number", exact: "the answer" }[it.answer_type];
  b += track === "reason" ? `\nThink step by step, then give ${what} on the last line as "Answer: <answer>".` : `\nReply with ${what} only.`;
  return b.trim();
}

export function maxTokens(it, track, budget) {
  const short = ["mc", "number", "exact"].includes(it.answer_type);
  if (track === "answer") return short ? 32 : 768;
  return short ? budget : budget + 768;
}

function firstTop(resp) {
  const c = resp.choices && resp.choices[0] && resp.choices[0].logprobs && resp.choices[0].logprobs.content;
  if (!c || !c.length) return null;
  const top = {};
  for (const t of c[0].top_logprobs || []) top[t.token] = (top[t.token] || 0) + Math.exp(t.logprob);
  if (!Object.keys(top).length) top[c[0].token] = Math.exp(c[0].logprob);
  return top;
}

const spread = (xs, n) => (!n || n >= xs.length ? xs : Array.from({ length: n }, (_, i) => xs[Math.floor((i * xs.length) / n)]));

// runDecide: cases are typed-decisions style rows {id, workflow, state, questions, gold} (parsed).
export async function runDecide(engine, cases, { onProgress, signal } = {}) {
  let adapter = "systemone";
  try { await engine.systemone({ state: cases[0].state, questions: cases[0].questions }); } catch { adapter = "logprobs"; }
  const scores = [], per = {}, preds = [];
  let done = 0, correct = 0, answered = 0, asked = 0;
  const total = cases.reduce((a, c) => a + Object.keys(c.questions).length, 0);
  const t0 = performance.now(), lat = [];
  for (const c of cases) {
    if (signal && signal.aborted) throw new DOMException("stopped", "AbortError");
    const s0 = performance.now();
    let dists = {};
    if (adapter === "systemone") {
      const r = await engine.systemone({ state: c.state, questions: c.questions });
      for (const [q, a] of Object.entries(r.answers || {})) {
        dists[q] = a.probabilities || (a.noul !== undefined ? { true: a.noul, false: 1 - a.noul } : a.choice ? { [a.choice]: 1 } : {});
      }
    } else {
      for (const [qid, q] of Object.entries(c.questions)) {
        const [prompt, labels] = decidePrompt(c.state, q);
        const r = await engine.chat({ messages: [{ role: "user", content: prompt }], max_tokens: 1, temperature: 0,
          logprobs: true, top_logprobs: 20, chat_template_kwargs: { enable_thinking: false } });
        const top = firstTop(r);
        if (top) { const [p, ok] = labelProbs(top, labels, options(q)); if (ok) dists[qid] = p; }
      }
    }
    lat.push(performance.now() - s0);
    for (const [qid, q] of Object.entries(c.questions)) {
      asked++;
      if (dists[qid] && Object.keys(dists[qid]).length) answered++;
      const s = scoreDecision(q, dists[qid] || {}, c.gold[qid].probabilities);
      scores.push(s);
      (per[c.workflow] ||= []).push(s);
      preds.push({ track: "decide", config: c.workflow, id: c.id, question: qid, type: q.type, probabilities: s.probabilities, correct: s.correct });
      if (s.correct) correct++;
      done++;
    }
    onProgress && onProgress({ track: "decide", done, total, correct, config: c.workflow });
  }
  const sum = summarizeDecisions(scores);
  const [lo, hi] = bootstrapCI(scores.map((s) => (s.correct ? 1 : 0)));
  lat.sort((a, b) => a - b);
  return {
    track: { accuracy: sum.accuracy, ci_low: lo, ci_high: hi, kl_from_gold: sum.kl_from_gold, brier: sum.brier, ece: sum.ece, n: sum.n_decisions,
      coverage: answered / asked, adapter, p50_ms: lat[Math.floor(lat.length / 2)] || 0, wall_seconds: (performance.now() - t0) / 1000,
      per_config: Object.fromEntries(Object.entries(per).map(([k, v]) => { const s = summarizeDecisions(v); return [k, { n: s.n_decisions, accuracy: s.accuracy, kl_from_gold: s.kl_from_gold, brier: s.brier, ece: s.ece }]; })) },
    preds,
  };
}

export async function runGen(engine, items, track, { budget = 2048, onProgress, signal } = {}) {
  const xs = [], per = {}, preds = [];
  let outTok = 0, truncated = 0, done = 0, correct = 0;
  const t0 = performance.now(), lat = [];
  for (const it of items) {
    if (signal && signal.aborted) throw new DOMException("stopped", "AbortError");
    const s0 = performance.now();
    const r = await engine.chat({ messages: [{ role: "user", content: genPrompt(it, track) }], max_tokens: maxTokens(it, track, budget),
      temperature: 0, chat_template_kwargs: { enable_thinking: track === "reason" } });
    lat.push(performance.now() - s0);
    const ch = r.choices[0];
    const content = ch.message.content || "";
    const reasoning = ch.message.reasoning_content || "";
    const full = reasoning ? "<think>" + reasoning + "</think>" + content : content;
    const [, cut] = stripThinking(full);
    const isTrunc = cut || (ch.finish_reason === "length" && !content.trim());
    const [ok, extracted] = checkAnswer(it, full);
    outTok += (r.usage && r.usage.completion_tokens) || 0;
    if (isTrunc) truncated++;
    xs.push(ok ? 1 : 0);
    (per[it.config] ||= []).push(ok ? 1 : 0);
    preds.push({ track, config: it.config, id: it.id, output: reasoning ? "<think></think>" + content : content, extracted, correct: ok, truncated: isTrunc });
    if (ok) correct++;
    done++;
    onProgress && onProgress({ track, done, total: items.length, correct, config: it.config });
  }
  const mean = (v) => v.reduce((a, b) => a + b, 0) / Math.max(1, v.length);
  const [lo, hi] = bootstrapCI(xs);
  const wall = (performance.now() - t0) / 1000;
  lat.sort((a, b) => a - b);
  return {
    track: { accuracy: mean(xs), ci_low: lo, ci_high: hi, n: xs.length, mean_output_tokens: outTok / Math.max(1, items.length),
      tokens_per_second: outTok / wall, p50_ms: lat[Math.floor(lat.length / 2)] || 0, wall_seconds: wall, truncation_rate: truncated / items.length,
      per_config: Object.fromEntries(Object.entries(per).map(([k, v]) => [k, { n: v.length, accuracy: mean(v) }])) },
    preds,
  };
}

export const PROFILES = { quick: { decide: 10, gen: 20, reason: 10 }, standard: { decide: 0, gen: 0, reason: 40 } };

// select picks the items of a profile from the loaded task data.
export function select(data, profile) {
  const p = PROFILES[profile];
  const byWf = {}, byCfg = {};
  for (const c of data.decide) (byWf[c.workflow] ||= []).push(c);
  for (const it of data.gen) (byCfg[it.config] ||= []).push(it);
  return {
    decide: Object.values(byWf).flatMap((v) => spread(v, p.decide)),
    answer: Object.values(byCfg).flatMap((v) => spread(v, p.gen)),
    reason: Object.values(byCfg).flatMap((v) => spread(v, p.reason)),
  };
}
