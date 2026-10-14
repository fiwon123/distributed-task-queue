# Screenshots (dark mode)

These screenshots were generated using the scripted Playwright capture (mocked /v1 API) against `frontend/dist` with `data-theme="dark"`, captured at 960 CSS px @2× (deviceScaleFactor 2) with per-page height caps (About is the top-section crop). To regenerate:

```bash
make screenshots
```

Files:
- console-dark.png — Jobs console listing queues and jobs (mixed states)
- job-detail-dark.png — Live job detail showing attempts, lease context, payload, last error
- workbench-dark.png — Workbench driving the API from the served OpenAPI spec
- about-dark.png — Concepts & how it works (theory, architecture mapping, concepts, state diagram)
