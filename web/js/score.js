// SPDX-License-Identifier: MIT
// LvW-bench scorer for the browser. Same rules as internal/score (Go) and bench/score.py;
// internal/score/golden.json keeps the three identical (web/test/golden.test.mjs).

const LETTERS = "ABCDEFGHIJ";

// --- System One -----------------------------------------------------------------------------

export function options(q) {
  if (q.type === "noul") return ["true", "false"];
  if (q.type === "score") return q.criteria.map((_, i) => String(i));
  return Object.keys(q.criteria || {});
}

function noul(type, m) {
  if (type !== "noul") return m || {};
  const map = { yes: "true", Yes: "true", True: "true", no: "false", No: "false", False: "false" };
  const out = {};
  for (const [k, v] of Object.entries(m || {})) {
    const key = map[k] || k;
    out[key] = (out[key] || 0) + v;
  }
  return out;
}

function argmax(v) {
  let best = 0;
  for (let i = 0; i < v.length; i++) if (v[i] > v[best]) best = i;
  return best;
}

export function scoreDecision(q, pred, gold) {
  const opts = options(q);
  pred = noul(q.type, pred);
  gold = noul(q.type, gold);
  const g = opts.map((o) => gold[o] || 0);
  let p = opts.map((o) => Math.max(pred[o] || 0, 0));
  const s = p.reduce((a, b) => a + b, 0);
  p = s > 0 ? p.map((x) => x / s) : p.map(() => 1 / opts.length);
  let kl = 0, brier = 0;
  for (let i = 0; i < opts.length; i++) {
    if (g[i] > 0) kl += g[i] * Math.log(g[i] / Math.max(p[i], 1e-9));
    brier += (p[i] - g[i]) ** 2;
  }
  const probabilities = {};
  opts.forEach((o, i) => (probabilities[o] = p[i]));
  return { correct: argmax(p) === argmax(g), kl, brier, confidence: p[argmax(p)], probabilities };
}

export function summarizeDecisions(scores) {
  const n = scores.length;
  if (!n) return { n_decisions: 0, accuracy: 0, kl_from_gold: 0, brier: 0, ece: 0 };
  const bins = 10, hits = Array(bins).fill(0), total = Array(bins).fill(0), conf = Array(bins).fill(0);
  let acc = 0, kl = 0, br = 0;
  for (const s of scores) {
    if (s.correct) acc++;
    kl += s.kl;
    br += s.brier;
    const b = Math.min(Math.floor(s.confidence * bins), bins - 1);
    total[b]++;
    conf[b] += s.confidence;
    if (s.correct) hits[b]++;
  }
  let ece = 0;
  for (let b = 0; b < bins; b++) if (total[b]) ece += (total[b] / n) * Math.abs(hits[b] / total[b] - conf[b] / total[b]);
  return { n_decisions: n, accuracy: acc / n, kl_from_gold: kl / n, brier: br / n, ece };
}

// --- System Two -----------------------------------------------------------------------------

const RE_THINK = /<think>[\s\S]*?<\/think>/g;
const RE_ANSWER_MC = /answer\s*(?:is)?\s*[:：]?\s*\**\(?([A-J])\)?\**(?:[^A-Za-z]|$)/gi;
const RE_LEAD_MC = /^\**\(?([A-J])\)?\**(?:[.):]|\s*$)/;
const RE_ANSWER_NUM = /answer\s*(?:is)?\s*[:：]?\s*\**\s*\$?\s*(-?[\d,]*\.?\d+)/gi;
const RE_NUMBER = /-?[\d,]*\.?\d+/g;
const RE_ANSWER_LINE = /^\s*\**answer\**\s*[:：]\s*(.+?)\s*$/gim;
const RE_FENCED = /```[a-zA-Z]*\s*\n([\s\S]*?)```/g;

export function stripThinking(s) {
  const i = s.lastIndexOf("</think>");
  if (i >= 0) return [s.slice(i + 8).trim(), false];
  if (s.includes("<think>")) return ["", true];
  return [s.replace(RE_THINK, "").trim(), false];
}

function lastGroup(re, text) {
  const all = [...text.matchAll(re)];
  return all.length ? all[all.length - 1][1] : null;
}

export function normNumber(s) {
  s = s.replace(/,/g, "");
  const f = Number(s);
  if (s === "" || Number.isNaN(f)) return s;
  return String(f);
}

export function normText(s) {
  s = s.trim().toLowerCase().replace(/^[ .!*"'`]+|[ .!*"'`]+$/g, "");
  s = s.split(/\s+/).filter(Boolean).join(" ");
  return s.replace(/\s*,\s*/g, ", ");
}

function extractMC(text, n, choices) {
  const valid = (l) => l && LETTERS.indexOf(l) < n;
  const a = lastGroup(RE_ANSWER_MC, text);
  if (valid(a)) return a;
  const t = text.trim();
  const m = t.match(RE_LEAD_MC);
  if (m && valid(m[1])) return m[1];
  const low = t.toLowerCase();
  let found = "";
  for (let i = 0; i < choices.length; i++) {
    const c = choices[i].trim().toLowerCase();
    if (c && low.includes(c)) {
      if (found) return "";
      found = LETTERS[i];
    }
  }
  return found;
}

function extractNumber(text) {
  const a = lastGroup(RE_ANSWER_NUM, text);
  if (a !== null) return normNumber(a);
  const all = text.match(RE_NUMBER);
  return all ? normNumber(all[all.length - 1]) : "";
}

export function extract(item, output) {
  const [text] = stripThinking(output);
  switch (item.answer_type) {
    case "mc": return extractMC(text, item.choices.length, item.choices);
    case "number": return extractNumber(text);
    case "exact": {
      const a = lastGroup(RE_ANSWER_LINE, text);
      if (a !== null) return normText(a);
      const lines = text.trim().split("\n");
      return normText(lines[lines.length - 1]);
    }
  }
  return text;
}

function firstJSON(text) {
  const cands = [...text.matchAll(RE_FENCED)].map((m) => m[1]).concat([text]);
  for (const c of cands) {
    let start = c.indexOf("{");
    while (start >= 0) {
      let depth = 0;
      for (let i = start; i < c.length; i++) {
        if (c[i] === "{") depth++;
        else if (c[i] === "}") depth--;
        if (depth === 0) {
          try {
            const v = JSON.parse(c.slice(start, i + 1));
            if (v && typeof v === "object" && !Array.isArray(v)) return v;
          } catch {}
          break;
        }
      }
      const next = c.indexOf("{", start + 1);
      if (next < 0) break;
      start = next;
    }
  }
  return null;
}

function canon(v) {
  if (Array.isArray(v)) return "[" + v.map(canon).join(",") + "]";
  if (v && typeof v === "object") return "{" + Object.keys(v).sort().map((k) => JSON.stringify(k) + ":" + canon(v[k])).join(",") + "}";
  return JSON.stringify(v);
}

function jsonEqual(a, b) {
  if (canon(a) === canon(b)) return true;
  return typeof a === "string" && typeof b === "string" && normText(a) === normText(b);
}

const RE_M_HEAD = /^(?:flowchart|graph)\s+(?:TD|TB|LR|RL|BT)\s*;?$/;
const RE_M_EDGE = /([A-Za-z0-9_]+)\s*(?:\[[^\]]*\]|\([^)]*\)|\{[^}]*\})?\s*(?:-->|---|-\.->|==>)\s*(?:\|[^|]*\|\s*)?([A-Za-z0-9_]+)/g;
const RE_M_NODE = /([A-Za-z0-9_]+)\s*(?:\[([^\]]*)\]|\(([^)]*)\)|\{([^}]*)\})/g;

function checkMermaid(item, text) {
  const f = [...text.matchAll(RE_FENCED)];
  const block = f.length ? f[0][1] : text;
  const lines = block.split("\n").map((l) => l.trim()).filter((l) => l && !l.startsWith("%%"));
  const head = lines.findIndex((l) => RE_M_HEAD.test(l));
  if (head < 0) return false;
  const label = {}, edges = [];
  for (const l of lines.slice(head + 1)) {
    for (const m of l.matchAll(RE_M_NODE)) for (const g of m.slice(2)) if (g) label[m[1]] = normText(g.replace(/^["']+|["']+$/g, ""));
    for (const m of l.matchAll(RE_M_EDGE)) edges.push([m[1], m[2]]);
  }
  const name = (id) => label[id] || normText(id);
  const have = new Set(edges.map(([a, b]) => name(a) + "\u0000" + name(b)));
  const want = (item.meta && item.meta.edges) || [];
  return want.length > 0 && want.every(([a, b]) => have.has(normText(a) + "\u0000" + normText(b)));
}

export function checkAnswer(item, output) {
  const [text] = stripThinking(output);
  const meta = item.meta || {};
  switch (item.answer_type) {
    case "mc": { const e = extract(item, output); return [e === item.answer, e]; }
    case "number": {
      const e = extract(item, output);
      const a = Number(e), b = Number(normNumber(item.answer));
      return [e !== "" && !Number.isNaN(a) && Math.abs(a - b) < 1e-6, e];
    }
    case "exact": {
      const e = extract(item, output);
      return [[item.answer, ...(meta.accept || [])].some((x) => e === normText(x)), e];
    }
    case "ifeval": return [checkIFEval(item, text), ""];
    case "json": {
      const obj = firstJSON(text);
      return [obj !== null && Object.entries(meta.values || {}).every(([k, v]) => k in obj && jsonEqual(obj[k], v)), ""];
    }
    case "mermaid": return [checkMermaid(item, text), ""];
  }
  return [false, ""];
}

// --- IFEval subset (google-research instruction_following_eval, Apache-2.0) ------------------

const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const rel = (r, actual, want) => (r === "less than" ? actual < want : actual >= want);

function cased(s, upper) {
  let any = false;
  for (const ch of s) {
    if (ch.toUpperCase() !== ch.toLowerCase()) {
      any = true;
      if (upper ? ch !== ch.toUpperCase() : ch !== ch.toLowerCase()) return false;
    }
  }
  return any;
}

function count(re, s) { return (s.match(re) || []).length; }

export function checkInstruction(id, kw, v) {
  kw = kw || {};
  switch (id) {
    case "punctuation:no_comma": return !v.includes(",");
    case "change_case:english_lowercase": return cased(v, false);
    case "change_case:english_capital": return cased(v, true);
    case "length_constraints:number_words": return rel(kw.relation, count(/\w+/g, v), kw.num_words | 0);
    case "length_constraints:number_paragraphs": {
      const paras = v.split(/\s?\*\*\*\s?/);
      let n = paras.length;
      for (let i = 0; i < paras.length; i++) {
        if (!paras[i].trim()) {
          if (i === 0 || i === paras.length - 1) n--;
          else return false;
        }
      }
      return n === (kw.num_paragraphs | 0);
    }
    case "detectable_format:number_bullet_lists":
      return count(/^\s*\*[^*].*$/gm, v) + count(/^\s*-.*$/gm, v) === (kw.num_bullets | 0);
    case "detectable_format:json_format": {
      let s = v.trim();
      for (const p of ["```json", "```Json", "```JSON", "```"]) if (s.startsWith(p)) { s = s.slice(p.length); break; }
      s = s.trim();
      if (s.endsWith("```")) s = s.slice(0, -3);
      try { JSON.parse(s.trim()); return true; } catch { return false; }
    }
    case "detectable_format:title": return (v.match(/<<[^\n]+>>/g) || []).some((t) => t.replace(/^<+|>+$/g, "").trim());
    case "detectable_format:number_highlighted_sections": {
      let n = (v.match(/\*[^\n*]*\*/g) || []).filter((h) => h.replace(/^\*+|\*+$/g, "").trim()).length;
      n += (v.match(/\*\*[^\n*]*\*\*/g) || []).filter((h) => h.slice(2, -2).trim()).length;
      return n >= (kw.num_highlights | 0);
    }
    case "keywords:existence": return (kw.keywords || []).every((k) => new RegExp(esc(k), "i").test(v));
    case "keywords:forbidden_words": return !(kw.forbidden_words || []).some((k) => new RegExp("\\b" + esc(k) + "\\b", "i").test(v));
    case "keywords:frequency": return rel(kw.relation, count(new RegExp(esc(kw.keyword || ""), "gi"), v), kw.frequency | 0);
    case "keywords:letter_frequency": {
      const l = (kw.letter || "").toLowerCase();
      return !!l && rel(kw.let_relation, v.toLowerCase().split(l).length - 1, kw.let_frequency | 0);
    }
    case "startend:end_checker":
      return v.trim().replace(/^"+|"+$/g, "").toLowerCase().endsWith((kw.end_phrase || "").trim().toLowerCase());
    case "startend:quotation": { const s = v.trim(); return s.length > 1 && s[0] === '"' && s[s.length - 1] === '"'; }
    case "detectable_content:postscript": {
      const m = kw.postscript_marker || "";
      const pat = m === "P.P.S" ? /\s*p\.\s?p\.\s?s.*$/m : m === "P.S." ? /\s*p\.\s?s\..*$/m : new RegExp("\\s*" + esc(m.toLowerCase()) + ".*$", "m");
      return pat.test(v.toLowerCase());
    }
    case "detectable_content:number_placeholders": return count(/\[.*?\]/g, v) >= (kw.num_placeholders | 0);
  }
  return false;
}

export function checkIFEval(item, text) {
  const ids = (item.meta && item.meta.instruction_id_list) || [];
  const kws = (item.meta && item.meta.kwargs) || [];
  return ids.length > 0 && ids.every((id, i) => checkInstruction(id, kws[i], text));
}

export function bootstrapCI(xs, draws = 1000) {
  if (!xs.length) return [0, 0];
  let seed = 1; // small LCG: the browser does not need Go's RNG, bounds may differ in the 3rd decimal
  const rnd = () => ((seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648);
  const means = [];
  for (let b = 0; b < draws; b++) {
    let s = 0;
    for (let i = 0; i < xs.length; i++) s += xs[Math.floor(rnd() * xs.length)];
    means.push(s / xs.length);
  }
  means.sort((a, b) => a - b);
  return [means[25], means[974]];
}
