import JobDetail from "./JobDetail";
import JobsView from "./JobsView";
import Workbench from "./Workbench";
import { AppLink, matchJobPath, usePath } from "./router";

// App is the router shell: "/" shows the console, "/jobs/{id}" a single job, and
// "/workbench" the spec-driven request runner. The job id is used as a React key
// so navigating to a different job remounts the detail view with fresh state.
export default function App() {
  const path = usePath();
  const jobId = matchJobPath(path);
  const onWorkbench = path === "/workbench" || path === "/workbench/";

  return (
    <main className="app">
      <header>
        <h1>
          <AppLink to="/">distributed-task-queue</AppLink>
        </h1>
        <nav className="tabs">
          <AppLink to="/">Jobs</AppLink>
          <AppLink to="/workbench">Workbench</AppLink>
        </nav>
      </header>
      {jobId ? <JobDetail key={jobId} id={jobId} /> : onWorkbench ? <Workbench /> : <JobsView />}
    </main>
  );
}
