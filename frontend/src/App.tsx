import JobDetail from "./JobDetail";
import JobsView from "./JobsView";
import { AppLink, matchJobPath, usePath } from "./router";

// App is the router shell: "/" shows the console, "/jobs/{id}" shows one job.
// The job id is used as a React key so navigating to a different job remounts
// the detail view with fresh state.
export default function App() {
  const path = usePath();
  const jobId = matchJobPath(path);

  return (
    <main className="app">
      <header>
        <h1>
          <AppLink to="/">distributed-task-queue</AppLink>
        </h1>
      </header>
      {jobId ? <JobDetail key={jobId} id={jobId} /> : <JobsView />}
    </main>
  );
}
