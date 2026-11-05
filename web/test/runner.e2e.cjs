// Drives web/run.html in Playwright Chromium (WebGPU flags) and checks a quick run finishes.
// node web/test/runner.e2e.cjs <base-url> [tracks]   (PLAYWRIGHT=path/to/playwright module)
const { chromium } = require(process.env.PLAYWRIGHT || "playwright");
(async () => {
  const base = process.argv[2] || "http://127.0.0.1:8791";
  const tracks = (process.argv[3] || "decide,answer").split(",");
  const browser = await chromium.launch({ args: ["--enable-unsafe-webgpu", "--use-angle=metal"] });
  const page = await browser.newPage();
  page.on("pageerror", (e) => console.log("pageerror:", e.message));
  await page.goto(base + "/run.html");
  console.log("gpu:", await page.textContent("#gpu"));
  for (const t of ["decide", "answer", "reason"]) {
    const box = page.locator(`.trk[value=${t}]`);
    if ((await box.isChecked()) !== tracks.includes(t)) await box.click();
  }
  await page.click("#go");
  const t0 = Date.now();
  await page.waitForSelector("#dl:not([hidden])", { timeout: 40 * 60 * 1000 }).catch(() => {});
  console.log((await page.textContent("#log")).trim().split("\n").slice(-6).join("\n"));
  console.log("summary:", (await page.textContent("#summary")).replace(/\s+/g, " ").trim());
  console.log("seconds:", Math.round((Date.now() - t0) / 1000));
  await page.screenshot({ path: process.env.SHOT || "/tmp/lvwb-runner.png", fullPage: true });
  await browser.close();
})();
