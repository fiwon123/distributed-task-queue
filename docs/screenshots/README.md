# Screenshots (dark mode)

These screenshots were generated using the scripted Playwright capture (mocked /v1 API) against `frontend/dist` with `data-theme="dark"`. To regenerate:

```bash
make screenshots
```

Note: in headless environments without a display server, Playwright may require xvfb; if generation fails locally, regenerate on a machine with a display or skip committing new images if they are unchanged in intent.

Files:
- console-dark.png — Jobs console listing queues and jobs (mixed states), enqueue form context
- job-detail-dark.png — Live job detail view for a running job
- workbench-dark.png — Workbench (API request runner built from OpenAPI)
- about-dark.png — About/Concepts & how it works page (educational theory)
