// SPDX-License-Identifier: MIT
// Renders data/leaderboard.json (written by `lvwb webdata`).
const $ = (s) => document.querySelector(s);
const state = { track: "decide", mode: "general", sort: null, desc: true };
let board;

const EXPLAIN = {
  decide: "Typed decisions on five workflows (1,500 decisions). Accuracy is argmax agreement with the rule gold; KL, Brier and ECE score the probabilities, lower is better. Prior and Uniform are the floors.",
  answer: "Direct answers with thinking off: 1,143 items from GSM8K, MMLU, MMLU-Pro, TruthfulQA, IFEval and the generated set. Short answers get 32 tokens.",
  reason: "Thinking on (native <think> or step by step), 2,048-token budget, 240 items (40 per set). A think block that never closes counts as wrong.",
};

const f3 = (x) => (x === undefined || x === null ? "–" : x.toFixed(3));
const hf = (m) => (m.repo ? `<a href="https://huggingface.co/${m.repo}">${esc(m.label)}</a>` : esc(m.label));
function esc(s) { return String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]); }
const accCell = (x) => `<span class="acc">${f3(x)}<span class="b"><i style="width:${Math.round((x || 0) * 100)}%"></i></span></span>`;

function columns(track) {
  if (track === "decide") {
    const wf = board.configs.filter((c) => c.kind === "decide").map((c) => c.name);
    return [
      { k: "accuracy", t: "Accuracy ↑", v: (r) => r.score.accuracy, f: accCell },
      { k: "kl", t: "KL ↓", v: (r) => r.score.kl_from_gold, f: f3, asc: true },
      { k: "brier", t: "Brier ↓", v: (r) => r.score.brier, f: f3, asc: true },
      { k: "ece", t: "ECE ↓", v: (r) => r.score.ece, f: f3, asc: true },
      { k: "cov", t: "Coverage", v: (r) => r.score.coverage, f: (x) => (x === undefined ? "–" : Math.round(x * 100) + "%") },
      ...wf.map((w) => ({ k: w, t: w, v: (r) => (r.score.per_config[w] || {}).accuracy, f: f3 })),
      { k: "adapter", t: "Adapter", v: (r) => r.score.adapter || "", f: (x) => `<span class="chip">${esc(x)}</span>`, text: true },
    ];
  }
  const sets = board.configs.filter((c) => c.kind === "gen").map((c) => c.name);
  return [
    { k: "accuracy", t: "Accuracy ↑", v: (r) => r.score.accuracy, f: accCell },
    { k: "ci", t: "95% CI", v: (r) => r.score.ci_low, f: (_, r) => `${f3(r.score.ci_low)}–${f3(r.score.ci_high)}` },
    ...sets.map((c) => ({ k: c, t: c, v: (r) => (r.score.per_config[c] || {}).accuracy, f: f3 })),
    { k: "tok", t: "tok / answer", v: (r) => r.score.mean_output_tokens, f: (x) => (x ? Math.round(x) : "–") },
    { k: "tps", t: "tok/s", v: (r) => r.score.tokens_per_second, f: (x) => (x ? Math.round(x) : "–") },
    ...(state.track === "reason" ? [{ k: "trunc", t: "Out of budget", v: (r) => r.score.truncation_rate, f: (x) => (x === undefined ? "–" : Math.round(x * 100) + "%"), asc: true }] : []),
  ];
}

function render() {
  const cols = columns(state.track);
  let rows = (board.tracks[state.track] || []).filter((r) => r.mode === state.mode);
  if (state.sort) {
    const c = cols.find((c) => c.k === state.sort);
    rows = rows.slice().sort((a, b) => {
      const x = c.v(a), y = c.v(b);
      const d = c.text ? String(x).localeCompare(String(y)) : (x ?? -1) - (y ?? -1);
      return state.desc ? -d : d;
    });
  }
  $("#board thead").innerHTML = `<tr><th class="num">#</th><th>Model</th>${cols.map((c) => `<th class="num" data-k="${c.k}">${esc(c.t)}${state.sort === c.k ? (state.desc ? " ▾" : " ▴") : ""}</th>`).join("")}</tr>`;
  const body = rows.map((r, i) => `<tr><td class="num">${i + 1}</td><td>${hf(r.model)} ${r.mode === "specialist" ? '<span class="chip spec">specialist</span>' : ""}<br><span class="note mono">${esc(r.system.engine || r.model.backend)} · ${esc(r.date)}</span></td>${cols.map((c) => `<td class="num">${c.f(c.v(r), r)}</td>`).join("")}</tr>`);
  if (state.track === "decide" && state.mode === "general") {
    for (const [name, t] of Object.entries(board.reference)) {
      const r = { score: { ...t, per_config: t.per_config } };
      body.push(`<tr class="ref"><td class="num">–</td><td>${name === "Prior" ? "Prior (ignores the input)" : "Uniform (same probability on every option)"}</td>${cols.map((c) => `<td class="num">${c.k === "adapter" ? "reference" : c.k === "cov" ? "–" : c.f(c.v(r), r)}</td>`).join("")}</tr>`);
    }
  }
  if (!rows.length && !(state.track === "decide" && state.mode === "general")) body.push(`<tr><td colspan="${cols.length + 2}" class="note">No ${state.mode} results for this track yet.</td></tr>`);
  $("#board tbody").innerHTML = body.join("");
  $("#explain").textContent = EXPLAIN[state.track] + (state.mode === "specialist" ? " Specialists were trained on the same task as a workflow, so they are not comparable with the zero-shot table." : "");
  document.querySelectorAll("#board th[data-k]").forEach((th) => th.addEventListener("click", () => {
    const k = th.dataset.k;
    const c = cols.find((c) => c.k === k);
    state.desc = state.sort === k ? !state.desc : !c.asc;
    state.sort = k;
    render();
  }));
}

function bindTabs(sel, key, attr) {
  document.querySelectorAll(sel + " button").forEach((b) => b.addEventListener("click", () => {
    document.querySelectorAll(sel + " button").forEach((x) => x.classList.toggle("on", x === b));
    state[key] = b.dataset[attr];
    state.sort = null;
    render();
  }));
}

fetch("data/leaderboard.json").then((r) => r.json()).then((b) => {
  board = b;
  bindTabs("#tracks", "track", "t");
  bindTabs("#modes", "mode", "m");
  $("#meta").textContent = `Bench ${b.version}, profile ${b.profile}. Each row is the newest standard run of that model and format.`;
  render();
}).catch((e) => { $("#explain").textContent = "Could not load the leaderboard: " + e.message; });
