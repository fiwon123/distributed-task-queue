#!/usr/bin/env node
// Captures the four README screenshots from frontend/dist against a stub API.
//
// Three things this guards against (issue #75):
//   1. No fonts on the host → headless Chromium renders text as nothing
//      (blank/black PNGs). On Linux we preflight `fc-list` and refuse to run
//      fontless instead of silently committing unreadable screenshots.
//   2. Screenshotting before the page is actually ready → we wait for
//      page-specific content, fonts, and the absence of skeletons/error
//      banners before every capture.
//   3. Silent blank captures → each PNG's text is asserted non-empty; the run
//      fails loudly rather than writing a black image.
//
// The fixture server also serves /openapi.json (from docs/openapi.json) so the
// Workbench renders the operation list instead of a spec-parse error banner.
const { createServer } = require("http");
const { execFileSync } = require("child_process");
const { readFile, stat } = require("fs/promises");
const { extname, join, resolve } = require("path");

const distDir = resolve(__dirname, "../dist");
const outDir = resolve(__dirname, "../../docs/screenshots");
const openapiPath = resolve(__dirname, "../../docs/openapi.json");

const MIN_TEXT_CHARS = 150; // every page shows far more than this when healthy

const MIME = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".css": "text/css",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".ico": "image/x-icon",
  ".woff2": "font/woff2",
  ".json": "application/json",
};

// On Linux, Chromium resolves text through fontconfig: with no fonts the
// glyphs render as nothing while SVG icons and CSS borders still draw — i.e.
// exactly the "black screenshot" failure (issue #75). Fail before launching.
function assertFontsAvailable() {
  if (process.platform !== "linux") return;
  const hint =
    " headless Chromium will render NO text (the black-screenshot bug). " +
    "Debian/Ubuntu: `apt-get install -y fontconfig fonts-liberation`. " +
    "In the dev sandbox this is baked into the image — rebuild it with " +
    "`make dev-build && make dev-restart`.";
  let out = "";
  try {
    out = execFileSync("fc-list", { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
  } catch {
    throw new Error("fc-list not found: fontconfig is missing on this Linux host." + hint);
  }
  if (!out.trim()) throw new Error("fc-list found no installed fonts." + hint);
}

function fixtures() {
  const now = Date.now();
  return {
    "/v1/queues": {
      queues: [
        { name: "default", total: 7 },
        { name: "emails", total: 2 },
      ],
    },
    "/v1/queues/default/stats": {
      queue: "default",
      counts: { pending: 2, running: 1, completed: 3, failed: 1, dead: 1 },
      total: 7,
    },
    "/v1/queues/emails/stats": {
      queue: "emails",
      counts: { pending: 1, running: 0, completed: 1, failed: 0, dead: 0 },
      total: 2,
    },
    "/v1/jobs": {
      count: 5,
      next: null,
      jobs: [
        {
          id: "j_9xk2",
          queue: "default",
          type: "send_email",
          status: "running",
          priority: 10,
          attempts: 1,
          max_attempts: 5,
          run_at: new Date(now - 30000).toISOString(),
          locked_until: new Date(now + 120000).toISOString(),
          created_at: new Date(now - 300000).toISOString(),
          last_error: "",
          payload: { to: "alice@example.com", subject: "Welcome" },
        },
        {
          id: "j_7ab4",
          queue: "default",
          type: "report",
          status: "pending",
          priority: 0,
          attempts: 0,
          max_attempts: 3,
          run_at: new Date(now - 5000).toISOString(),
          locked_until: null,
          created_at: new Date(now - 120000).toISOString(),
          last_error: "",
          payload: { range: "7d" },
        },
        {
          id: "j_3t9m",
          queue: "default",
          type: "report",
          status: "failed",
          priority: 0,
          attempts: 2,
          max_attempts: 3,
          run_at: new Date(now + 60000).toISOString(),
          locked_until: null,
          created_at: new Date(now - 600000).toISOString(),
          completed_at: null,
          last_error: "timeout while calling upstream",
          payload: { range: "30d" },
        },
        {
          id: "j_2r8f",
          queue: "default",
          type: "cleanup",
          status: "dead",
          priority: -1,
          attempts: 3,
          max_attempts: 3,
          run_at: new Date(now - 100000).toISOString(),
          locked_until: null,
          created_at: new Date(now - 1800000).toISOString(),
          last_error: "permanent failure",
          payload: { days: 30 },
        },
        {
          id: "j_5h1p",
          queue: "emails",
          type: "send_email",
          status: "completed",
          priority: 5,
          attempts: 1,
          max_attempts: 3,
          run_at: new Date(now - 200000).toISOString(),
          locked_until: null,
          created_at: new Date(now - 240000).toISOString(),
          completed_at: new Date(now - 180000).toISOString(),
          last_error: "",
          payload: { to: "bob@example.com" },
        },
      ],
    },
    "/v1/jobs/j_9xk2": {
      id: "j_9xk2",
      queue: "default",
      type: "send_email",
      status: "running",
      priority: 10,
      attempts: 1,
      max_attempts: 5,
      run_at: new Date(now - 30000).toISOString(),
      locked_until: new Date(now + 120000).toISOString(),
      created_at: new Date(now - 300000).toISOString(),
      last_error: "",
      payload: { to: "alice@example.com", subject: "Welcome" },
    },
    "/v1/workers": { workers: [] },
  };
}

function staticServer(port) {
  const files = fixtures();
  return createServer(async (req, res) => {
    const url = req.url.split("?")[0];
    if (url.startsWith("/v1/")) {
      const data = files[url] || { error: "not stubbed", path: url, reqUrl: req.url };
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(data));
      return;
    }
    // The Workbench loads the real spec; serving index.html here (the SPA
    // fallback below) made it show a spec-parse error banner in captures.
    if (url === "/openapi.json") {
      try {
        const buf = await readFile(openapiPath);
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(buf);
        return;
      } catch {
        res.writeHead(500, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: "openapi.json fixture unreadable" }));
        return;
      }
    }
    let pathname = url === "/" ? "/index.html" : url;
    try {
      let fp = join(distDir, pathname);
      const s = await stat(fp);
      if (s.isDirectory()) fp = join(fp, "index.html");
      const buf = await readFile(fp);
      res.writeHead(200, { "Content-Type": MIME[extname(fp)] || "application/octet-stream" });
      res.end(buf);
    } catch {
      const buf = await readFile(join(distDir, "index.html"));
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(buf);
    }
  }).listen(port);
}

async function launchBrowser() {
  const { chromium } = require("playwright");
  const argSets = [
    ["--no-sandbox", "--disable-dev-shm-usage"],
    ["--no-sandbox", "--disable-dev-shm-usage", "--disable-setuid-sandbox"],
  ];
  let lastErr;
  for (const args of argSets) {
    try {
      return await chromium.launch({ headless: true, args });
    } catch (err) {
      lastErr = err;
      if (/Executable doesn't exist|browser.*not found|please run/i.test(err.message)) {
        throw new Error(
          "Playwright's Chromium is not installed. Run `npx playwright install chromium` " +
            "from frontend/ (in the dev sandbox the browser is baked into the image — " +
            "rebuild it with `make dev-build && make dev-restart`). " +
            `Original error: ${err.message}`,
          { cause: err },
        );
      }
    }
  }
  throw lastErr;
}

async function capture() {
  assertFontsAvailable();
  const port = 5181;
  const server = staticServer(port);
  const browser = await launchBrowser();
  const page = await browser.newPage({
    viewport: { width: 960, height: 820 },
    deviceScaleFactor: 2,
  });
  await page.addInitScript(() => {
    try {
      localStorage.setItem("dtq-theme", "dark");
    } catch {}
  });
  // Surface page-side failures: without this a missing selector is an opaque
  // 30s timeout even though the cause is a console error or a failed request.
  const pageErrors = [];
  page.on("pageerror", (e) => pageErrors.push(`pageerror: ${e.message}`));
  page.on("console", (m) => {
    if (m.type() === "error") pageErrors.push(`console.error: ${m.text()}`);
  });
  page.on("requestfailed", (r) =>
    pageErrors.push(`requestfailed: ${r.url()} (${r.failure()?.errorText ?? "?"})`),
  );
  const base = "http://localhost:" + port;
  const shots = [
    {
      name: "console-dark.png",
      path: "/",
      height: 1100,
      // queue stats + the jobs table with at least one row
      ready: (p) => p.waitForSelector('section[aria-label="jobs"] table tbody tr'),
    },
    {
      name: "job-detail-dark.png",
      path: "/jobs/j_9xk2",
      height: 880,
      // aria-busy flips to false once the job record has loaded
      ready: (p) =>
        p.waitForSelector('section[aria-label="job detail"][aria-busy="false"] dd.mono'),
    },
    {
      name: "workbench-dark.png",
      path: "/workbench",
      height: 800,
      // operations parsed from /openapi.json (not the spec-error state)
      ready: (p) => p.waitForSelector('.workbench[aria-busy="false"] .operation'),
    },
    {
      name: "about-dark.png",
      path: "/about",
      height: 900,
      ready: (p) => p.waitForSelector('section[aria-label="about"] .concept-card'),
    },
  ];
  for (const s of shots) {
    await page.goto(base + s.path, { waitUntil: "load" });
    await page.setViewportSize({ width: 960, height: s.height });
    try {
      await s.ready(page);
    } catch (err) {
      const snippet = (await page.evaluate(() => document.body.innerText).catch(() => "")).trim();
      throw new Error(
        `${s.name}: page never became ready — ${err.message}\n` +
          `page text: ${JSON.stringify(snippet.slice(0, 400))}\n` +
          `page errors: ${pageErrors.length ? pageErrors.join(" | ") : "(none)"}`,
        { cause: err },
      );
    }
    // no loading placeholders may be on screen at capture time
    await page.waitForSelector(".skeleton", { state: "detached" });
    const bannerCount = await page.locator(".banner.error:visible").count();
    if (bannerCount > 0) {
      throw new Error(
        `${s.name}: ${bannerCount} visible error banner(s) — fixture or page is broken`,
      );
    }
    await page.evaluate(async () => {
      await document.fonts.ready;
    });
    const text = (await page.evaluate(() => document.body.innerText)).trim();
    if (text.length < MIN_TEXT_CHARS) {
      throw new Error(
        `${s.name}: page shows only ${text.length} chars of text (expected >= ${MIN_TEXT_CHARS}) ` +
          `— refusing to write a blank screenshot. ` +
          (process.platform === "linux"
            ? "Check fonts: `fc-list | head` should list system fonts."
            : "The page failed to render its content."),
      );
    }
    await page.waitForTimeout(150); // let the final paint settle
    await page.screenshot({ path: join(outDir, s.name), fullPage: false, animations: "disabled" });
    console.log(`captured ${s.name} (${text.length} chars of text)`);
  }
  await browser.close();
  server.close();
  console.log("done");
}

capture().catch((e) => {
  console.error(e);
  process.exit(1);
});
