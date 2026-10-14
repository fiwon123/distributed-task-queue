# Screenshots (dark mode)

These screenshots were generated using the scripted Playwright capture (mocked /v1 API) against `frontend/dist` with `data-theme="dark"`, captured at 960 CSS px @2× (deviceScaleFactor 2) with per-page height caps (About is the top-section crop). To regenerate:

```bash
make screenshots
```

## Requirements

The capture runs headless Chromium via Playwright in `frontend/`, so it needs
**fonts** and a **browser build**:

- **Fonts** — headless Chromium resolves text through fontconfig. With no fonts
  installed it renders glyphs as *nothing*: SVG icons and CSS borders still draw,
  so every PNG comes out looking like a black console. On Debian/Ubuntu:
  `apt-get install -y fontconfig fonts-liberation fonts-dejavu-core`.
- **Browser** — `npx playwright install chromium` (from `frontend/`). The script
  fails with that hint if the browser is missing.

The **dev sandbox image bakes both in** (`Dockerfile`, `PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright`),
so inside `make opencode` / `make dev-shell` the command just works. After a
Dockerfile change, rebuild the image: `make dev-build && make dev-restart`.

The script refuses to write a screenshot it cannot trust: it waits for
page-specific content and fonts, and **fails loudly** if a page shows an error
banner, still shows loading skeletons, or renders almost no text — so a blank
capture can never be committed silently again.

Files:
- console-dark.png — Jobs console listing queues and jobs (mixed states)
- job-detail-dark.png — Live job detail showing attempts, lease context, payload, last error
- workbench-dark.png — Workbench driving the API from the served OpenAPI spec
- about-dark.png — Concepts & how it works (theory, architecture mapping, concepts, state diagram)
