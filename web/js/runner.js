// SPDX-License-Identifier: MIT
// Browser runner page: loads a model with wllama or WebLLM and runs the LvW-bench tracks.
import { runDecide, runGen, select } from "./bench.js";

const WLLAMA = "https://cdn.jsdelivr.net/npm/@wllama/wllama@3.8.1/esm/index.js";
const WLLAMA_WASM = "https://cdn.jsdelivr.net/npm/@wllama/wllama@3.8.1/esm/wasm/wllama.wasm";
const WEBLLM = "https://cdn.jsdelivr.net/npm/@mlc-ai/web-llm@0.2.87/+esm";
const VERSION = "0.1.0";

const $ = (s) => document.querySelector(s);
let ctrl = null, lastResult = null, lastPreds = [];

const log = (s) => { const l = $("#log"); l.textContent += s + "\n"; l.scrollTop = l.scrollHeight; };

$("#engine").addEventListener("change", () => {
  const w = $("#engine").value === "webllm";
  $("#ggufBox").hidden = w;
  $("#webllmBox").hidden = !w;
});
const hasGPU = !!navigator.gpu;
$("#gpu").textContent = hasGPU ? "WebGPU available" : "No WebGPU: GGUF runs on the CPU, WebLLM will not start";

function parseRepo(s) {
  s = s.trim();
  const m = s.match(/^https?:\/\/(?:www\.)?(?:huggingface\.co|hf\.co)\/([^/]+\/[^/]+)(?:\/(?:blob|resolve)\/[^/]+\/(.+\.gguf))?/);
  if (m) return { repo: m[1], file: m[2] };
  const [repo, file] = s.replace(/^hf:\/\//, "").split(":");
  return { repo, file };
}

async function pickGGUF(repo) {
  const r = await fetch(`https://huggingface.co/api/models/${repo}/tree/main?recursive=true`);
  if (!r.ok) throw new Error(`${repo}: HTTP ${r.status}`);
  const files = (await r.json()).filter((f) => f.path.endsWith(".gguf") && !/mmproj/i.test(f.path));
  if (!files.length) throw new Error(`${repo} has no .gguf file (safetensors need lvwb)`);
  for (const q of ["Q4_K_M", "Q4_K_S", "Q5_K_M", "Q4_0", "Q8_0"]) {
    const hit = files.filter((f) => f.path.toUpperCase().includes(q)).sort((a, b) => a.path.length - b.path.length)[0];
    if (hit) return hit.path;
  }
  return files.sort((a, b) => a.size - b.size)[0].path;
}

async function wllamaEngine(budget) {
  const { Wllama } = await import(WLLAMA);
  const w = new Wllama({ default: WLLAMA_WASM });
  const params = { n_ctx: 2048 + budget + 768, n_gpu_layers: hasGPU ? 999 : 0, progressCallback: ({ loaded, total }) =>
    setBar("load", loaded / Math.max(1, total), `${(loaded / 1e6).toFixed(0)} / ${(total / 1e6).toFixed(0)} MB`) };
  let label, model;
  const files = $("#file").files;
  if (files.length) {
    await w.loadModel([...files], params);
    label = files[0].name.replace(/\.gguf$/, "");
    model = { label: label + " · GGUF", input: files[0].name, format: "gguf", backend: "wllama" };
  } else {
    const { repo, file } = parseRepo($("#model").value);
    const f = file || (await pickGGUF(repo));
    log(`loading ${repo} / ${f}`);
    await w.loadModelFromHF({ repo, file: f }, params);
    const q = (f.match(/(IQ\d_[A-Z]+|Q\d_K_[SML]|Q\d_K|Q\d_\d|BF16|F16)/i) || [""])[0].toUpperCase();
    model = { label: `${repo.split("/")[1]} · GGUF ${q}`.trim(), input: repo, format: "gguf", repo, file: f, quant: q, backend: "wllama" };
  }
  return {
    model, version: "wllama 3.8.1",
    chat: (p) => w.createChatCompletion({ ...p, stream: false }),
    systemone: (p) => w.createSystemOne(p),
  };
}

async function webllmEngine() {
  if (!hasGPU) throw new Error("WebLLM needs WebGPU");
  const webllm = await import(WEBLLM);
  const id = $("#mlc").value;
  const engine = await webllm.CreateMLCEngine(id, { initProgressCallback: (r) => setBar("load", r.progress || 0, r.text || "") });
  return {
    model: { label: id, input: id, format: "mlc", backend: "webllm" },
    version: "web-llm 0.2.87",
    chat: (p) => {
      const { chat_template_kwargs, top_logprobs, ...rest } = p;
      const body = { ...rest, stream: false };
      if (top_logprobs) body.top_logprobs = Math.min(5, top_logprobs);
      if (chat_template_kwargs) body.extra_body = { enable_thinking: !!chat_template_kwargs.enable_thinking };
      return engine.chat.completions.create(body);
    },
    systemone: () => { throw new Error("not a decision model"); },
  };
}

function setBar(id, frac, text) {
  let el = document.getElementById("bar-" + id);
  if (!el) {
    el = document.createElement("div");
    el.className = "card";
    el.id = "bar-" + id;
    el.innerHTML = `<div class="row" style="justify-content:space-between"><strong>${id}</strong><span class="note"></span></div><div class="progress" style="margin-top:8px"><i></i></div>`;
    $("#bars").appendChild(el);
  }
  el.querySelector("i").style.width = Math.round(frac * 100) + "%";
  el.querySelector(".note").textContent = text;
}

function download(name, text) {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([text], { type: "application/json" }));
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

$("#go").addEventListener("click", async () => {
  const tracks = [...document.querySelectorAll(".trk:checked")].map((x) => x.value);
  if (!tracks.length) return;
  const profile = $("#profile").value, budget = 2048;
  ctrl = new AbortController();
  $("#go").disabled = true; $("#stop").disabled = false;
  $("#out").hidden = false; $("#bars").innerHTML = ""; $("#log").textContent = ""; $("#summary").innerHTML = ""; $("#dl").hidden = true;
  try {
    log("loading task data");
    const [decide, gen] = await Promise.all([fetch("data/tasks/decide.json").then((r) => r.json()), fetch("data/tasks/gen.json").then((r) => r.json())]);
    const sel = select({ decide, gen }, profile);
    const eng = $("#engine").value === "webllm" ? await webllmEngine() : await wllamaEngine(budget);
    $("#title").textContent = eng.model.label;
    log(`model loaded (${eng.version})`);
    const res = { bench: "LvW-bench", bench_version: VERSION, created: new Date().toISOString(), model: eng.model, mode: "general",
      profile, seed: 0, tracks: {}, system: { os: navigator.platform, engine: eng.version, browser: navigator.userAgent, webgpu: String(hasGPU) } };
    const preds = [];
    const onProgress = (e) => setBar(e.track, e.done / e.total, `${e.done}/${e.total} · ${((100 * e.correct) / Math.max(1, e.done)).toFixed(1)}% · ${e.config}`);
    for (const t of tracks) {
      log(`running ${t}`);
      const out = t === "decide" ? await runDecide(eng, sel.decide, { onProgress, signal: ctrl.signal })
        : await runGen(eng, t === "answer" ? sel.answer : sel.reason, t, { budget, onProgress, signal: ctrl.signal });
      res.tracks[t] = out.track;
      preds.push(...out.preds);
      log(`${t}: accuracy ${out.track.accuracy.toFixed(3)}${out.track.adapter ? " (" + out.track.adapter + ")" : ""}`);
    }
    lastResult = res; lastPreds = preds;
    $("#summary").innerHTML = `<div class="tablewrap"><table><thead><tr><th>Track</th><th class="num">Accuracy</th><th class="num">95% CI</th><th class="num">KL</th><th class="num">n</th></tr></thead><tbody>${
      Object.entries(res.tracks).map(([t, s]) => `<tr><td>${t}</td><td class="num">${s.accuracy.toFixed(3)}</td><td class="num">${s.ci_low.toFixed(3)}–${s.ci_high.toFixed(3)}</td><td class="num">${s.kl_from_gold !== undefined ? s.kl_from_gold.toFixed(3) : "–"}</td><td class="num">${s.n}</td></tr>`).join("")}</tbody></table></div>`;
    $("#dl").hidden = false;
  } catch (e) {
    log(e.name === "AbortError" ? "stopped" : "error: " + (e.message || e));
  } finally {
    $("#go").disabled = false; $("#stop").disabled = true;
  }
});
$("#stop").addEventListener("click", () => ctrl && ctrl.abort());
$("#dlRes").addEventListener("click", () => download("lvw-bench-result.json", JSON.stringify(lastResult, null, 2)));
$("#dlPred").addEventListener("click", () => download("lvw-bench-result.predictions.jsonl", lastPreds.map((p) => JSON.stringify(p)).join("\n") + "\n"));
