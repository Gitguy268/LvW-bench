// SPDX-License-Identifier: MIT
// LvW-bench app UI. Talks to Go through window.go.main.App and runtime events.
const $ = (s) => document.querySelector(s);
const App = () => window.go.main.App;
const esc = (s) => String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
const f3 = (x) => (typeof x === "number" ? x.toFixed(3) : "–");

function log(s) { const l = $("#log"); l.textContent += s + "\n"; l.scrollTop = l.scrollHeight; }

function bar(id, frac, text) {
  let el = document.getElementById("bar-" + id);
  if (!el) {
    el = document.createElement("div");
    el.className = "card";
    el.id = "bar-" + id;
    el.innerHTML = `<div class="row" style="justify-content:space-between"><strong>${esc(id)}</strong><span class="note"></span></div><div class="progress" style="margin-top:8px"><i></i></div>`;
    $("#bars").appendChild(el);
  }
  el.querySelector("i").style.width = Math.round(Math.min(1, frac) * 100) + "%";
  el.querySelector(".note").textContent = text;
}

async function init() {
  const info = await App().Info();
  $("#ver").textContent = `v${info.version} · bench ${info.bench}`;
  $("#dir").textContent = info.results_dir;
  $("#eng").innerHTML =
    `<div><b>llama-server</b> ${info.llama_server ? `<span class="ok">found</span>` : `<span class="missing">missing</span>: brew install llama.cpp (needed for GGUF)`}</div>` +
    `<div><b>uv</b> ${info.uv ? `<span class="ok">found</span>` : `<span class="missing">missing</span>: brew install uv (needed for safetensors)`}</div>` +
    `<div class="note">${esc(info.os)}</div>`;
  $("#presets").innerHTML = info.presets.map((p) => `<div data-n="${esc(p.name)}"><strong>${esc(p.name)}</strong> <span class="chip">${esc(p.kind)}</span><small>${esc(p.comment)}</small></div>`).join("");
  document.querySelectorAll("#presets div").forEach((d) => d.addEventListener("click", () => {
    document.querySelectorAll("#presets div").forEach((x) => x.classList.toggle("on", x === d));
    $("#model").value = d.dataset.n;
    const p = info.presets.find((x) => x.name === d.dataset.n);
    document.querySelectorAll(".trk").forEach((c) => (c.checked = p.tracks.includes(c.value)));
    $("#mode").value = p.mode || "general";
  }));
}

window.runtime.EventsOn("lvwb:event", (e) => {
  if (e.kind === "status") log("· " + (e.track ? e.track + ": " : "") + e.msg);
  else if (e.kind === "download") bar("download", e.of_bytes ? e.bytes / e.of_bytes : 0, `${(e.bytes / 1e9).toFixed(2)} / ${(e.of_bytes / 1e9).toFixed(2)} GB`);
  else if (e.kind === "progress") bar(e.track, e.done / e.total, `${e.done}/${e.total} · ${((100 * e.correct) / Math.max(1, e.done)).toFixed(1)}% · ${e.config}`);
  else if (e.kind === "track_done") log(`✓ ${e.track}: ${e.msg}`);
});

$("#pickFile").addEventListener("click", () => $("#fileInput").click());
$("#fileInput").addEventListener("change", (ev) => {
  // Wails passes the real path for dropped / picked files on desktop builds.
  const f = ev.target.files[0];
  if (f && f.path) $("#model").value = f.path;
  else if (f) log("Type the full path of " + f.name + " into the model field.");
});

$("#go").addEventListener("click", async () => {
  const req = {
    model: $("#model").value, profile: $("#profile").value, mode: $("#mode").value,
    tracks: [...document.querySelectorAll(".trk:checked")].map((c) => c.value),
    quant: $("#quant").value.trim(), subfolder: $("#subfolder").value.trim(),
    trust_remote_code: $("#trust").checked, cpu_only: $("#cpu").checked,
  };
  if (!req.model.trim()) { log("Give a model first."); return; }
  if (/^https?:\/\//.test(req.model) && !/huggingface\.co|hf\.co/.test(req.model)) req.backend = "openai";
  $("#go").disabled = true; $("#stop").disabled = false;
  $("#bars").innerHTML = ""; $("#log").textContent = ""; $("#summary").innerHTML = "";
  $("#title").textContent = req.model;
  try {
    const r = await App().Run(req);
    const res = r.result;
    $("#title").textContent = res.model.label;
    $("#summary").innerHTML = `<div class="tablewrap"><table><thead><tr><th>Track</th><th class="num">Accuracy</th><th class="num">95% CI</th><th class="num">KL</th><th class="num">Brier</th><th class="num">ECE</th><th class="num">n</th><th>Per set</th></tr></thead><tbody>${
      Object.entries(res.tracks).map(([t, s]) => `<tr><td>${t}</td><td class="num">${f3(s.accuracy)}</td><td class="num">${f3(s.ci_low)}–${f3(s.ci_high)}</td><td class="num">${t === "decide" ? f3(s.kl_from_gold) : "–"}</td><td class="num">${t === "decide" ? f3(s.brier) : "–"}</td><td class="num">${t === "decide" ? f3(s.ece) : "–"}</td><td class="num">${s.n}</td><td class="note">${
        Object.entries(s.per_config).map(([c, v]) => `${c} ${f3(v.accuracy)}`).join(" · ")}</td></tr>`).join("")}</tbody></table></div><p class="note">Saved ${esc(r.path)}</p>`;
  } catch (e) {
    log("error: " + (e && e.message ? e.message : e));
  } finally {
    $("#go").disabled = false; $("#stop").disabled = true;
  }
});
$("#stop").addEventListener("click", () => App().Stop());
$("#openDir").addEventListener("click", () => App().OpenResults());

document.querySelectorAll("#tabs button").forEach((b) => b.addEventListener("click", async () => {
  document.querySelectorAll("#tabs button").forEach((x) => x.classList.toggle("on", x === b));
  $("#view-run").hidden = b.dataset.v !== "run";
  $("#view-results").hidden = b.dataset.v !== "results";
  if (b.dataset.v === "results") {
    const rs = (await App().Results()) || [];
    $("#results tbody").innerHTML = rs.map((r) => `<tr><td>${esc(r.created.slice(0, 16).replace("T", " "))}</td><td>${esc(r.model.label)}</td><td>${esc(r.profile)}</td><td class="num">${f3(r.tracks.decide?.accuracy)}</td><td class="num">${f3(r.tracks.decide?.kl_from_gold)}</td><td class="num">${f3(r.tracks.answer?.accuracy)}</td><td class="num">${f3(r.tracks.reason?.accuracy)}</td></tr>`).join("") ||
      `<tr><td colspan="7" class="note">No results yet.</td></tr>`;
  }
}));

init();
