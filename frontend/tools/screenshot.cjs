#!/usr/bin/env node
const { createServer } = require("http");
const { readFile, stat } = require("fs/promises");
const { extname, join, resolve } = require("path");

const distDir = resolve(__dirname, "../dist");
const outDir = resolve(__dirname, "../../docs/screenshots");

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

function fixtures() {
  const now = Date.now();
  return {
    "/v1/queues": {
      queues: [
        {
          name: "default",
          jobs: { total: 7, pending: 2, running: 1, completed: 3, failed: 1, dead: 1 },
        },
        {
          name: "emails",
          jobs: { total: 2, pending: 1, running: 0, completed: 1, failed: 0, dead: 0 },
        },
      ],
    },
    "/v1/queues/default/stats": {
      queue: "default",
      jobs: { total: 7, pending: 2, running: 1, completed: 3, failed: 1, dead: 1 },
    },
    "/v1/queues/emails/stats": {
      queue: "emails",
      jobs: { total: 2, pending: 1, running: 0, completed: 1, failed: 0, dead: 0 },
    },
    "/v1/jobs": {
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
  try {
    return await chromium.launch({
      headless: true,
      args: ["--no-sandbox", "--disable-dev-shm-usage"],
    });
  } catch {
    return await chromium.launch({
      headless: true,
      args: ["--no-sandbox", "--disable-dev-shm-usage", "--disable-setuid-sandbox"],
    });
  }
}

async function capture() {
  const port = 5181;
  const server = staticServer(port);
  const browser = await launchBrowser();
  const page = await browser.newPage({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1.5,
  });
  await page.addInitScript(() => {
    try {
      localStorage.setItem("dtq-theme", "dark");
    } catch {}
  });
  const base = "http://localhost:" + port;
  const shots = [
    { name: "console-dark.png", path: "/" },
    { name: "job-detail-dark.png", path: "/jobs/j_9xk2" },
    { name: "workbench-dark.png", path: "/workbench" },
    { name: "about-dark.png", path: "/about" },
  ];
  for (const s of shots) {
    await page.goto(base + s.path, { waitUntil: "networkidle" });
    await page.screenshot({ path: join(outDir, s.name), fullPage: true });
  }
  await browser.close();
  server.close();
  console.log("done");
}

capture().catch((e) => {
  console.error(e);
  process.exit(1);
});
