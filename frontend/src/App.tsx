import About from "./About";
import { GitHubIcon } from "./icons";
import JobDetail from "./JobDetail";
import JobsView from "./JobsView";
import { DOCS_PATH, OPENAPI_PATH, REPO_URL } from "./links";
import Logo from "./Logo";
import ThemeToggle from "./ThemeToggle";
import Workbench from "./Workbench";
import { AppLink, matchJobPath, usePath } from "./router";

// App is the router shell: "/" shows the console, "/jobs/{id}" a single job, and
// "/workbench" the spec-driven request runner, "/about" the how-to page. The job
// id is used as a React key so navigating to a different job remounts the detail
// view with fresh state.
export default function App() {
  const path = usePath();
  const jobId = matchJobPath(path);
  const onWorkbench = path === "/workbench" || path === "/workbench/";
  const onAbout = path === "/about" || path === "/about/";
  const onJobs = !onWorkbench && !onAbout; // "/" and /jobs/{id} both live in Jobs

  return (
    <div className="app">
      <header className="site-header">
        <h1 className="brand">
          <AppLink to="/">
            <Logo size={30} />
            <span className="brand-name">
              distributed<span className="brand-accent">·</span>task
              <span className="brand-accent">·</span>queue
            </span>
          </AppLink>
        </h1>

        <nav className="site-nav" aria-label="primary">
          <AppLink to="/" className="nav-link" current={onJobs}>
            Jobs
          </AppLink>
          <AppLink to="/workbench" className="nav-link" current={onWorkbench}>
            Workbench
          </AppLink>
          <AppLink to="/about" className="nav-link" current={onAbout}>
            About
          </AppLink>
        </nav>

        <div className="header-actions">
          <a
            className="icon-button"
            href={REPO_URL}
            target="_blank"
            rel="noreferrer"
            aria-label="View source on GitHub"
            title="View source on GitHub"
          >
            <GitHubIcon />
          </a>
          <ThemeToggle />
        </div>
      </header>

      <main className="site-main">
        {jobId ? (
          <JobDetail key={jobId} id={jobId} />
        ) : onWorkbench ? (
          <Workbench />
        ) : onAbout ? (
          <About />
        ) : (
          <JobsView />
        )}
      </main>

      <footer className="site-footer">
        <span className="tagline">Postgres is the queue · at-least-once delivery</span>
        <nav className="footer-links" aria-label="footer">
          <a href={REPO_URL} target="_blank" rel="noreferrer">
            <GitHubIcon /> GitHub
          </a>
          <a href={DOCS_PATH}>API docs</a>
          <a href={OPENAPI_PATH}>OpenAPI</a>
        </nav>
      </footer>
    </div>
  );
}
