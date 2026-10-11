# Agent Memory System

Persistent, local-only memory so work continues across chat sessions.

## Location

```
.opencode/memory/
├── global.md          # Major memory: issues, milestones, current task, decisions
├── current.md         # Minor memory: this session's context + progress
└── archive/           # Rotating history of the last 3 interactions
    ├── interaction-1.md   # most recent
    ├── interaction-2.md
    └── interaction-3.md   # oldest
```

The whole `.opencode/memory/` directory is gitignored — it is local to this
machine and is never committed or pushed.

## When to read

- **At the start of every session**: read `global.md` then `current.md` to
  restore context. Say what you resumed ("Resuming: <task>").
- Before starting a new task, check `global.md` → Active Issues so you never
  create a duplicate issue or re-do finished work.

## When to update

- **`current.md`**: continuously — after each step, add what was done, what
  changed, what's next, and anything the user must confirm.
- **`global.md`**: at meaningful milestones — issue created, PR opened, merged,
  task completed, decision made. Keep it the single source of truth for tracked
  work.
- **At session end**: write a summary to `archive/interaction-N.md` (next
  number, 1 = most recent), rotate out the oldest so only **3** archive files
  remain, then trim `current.md` down to the essentials for the next session.

## Format rules

- `global.md` ≤ ~200 lines; `current.md` ≤ ~100 lines; archive files ≤ ~50
  lines each.
- When adding new entries, remove or compress old ones — do not let the files
  grow unbounded.
- Keep timestamps (`YYYY-MM-DD`) on entries that change over time.
- Track GitHub work per the workflow in `.opencode/instructions/workflow.md`:
  every issue assigned, labeled by type, linked to a branch and PR, assigned a
  milestone, and commented at each milestone.

## Golden rule

If a new session starts and `current.md` exists but is empty, that is
intentional — check `global.md` for the active task and `archive/` for what
happened before. If both are empty, ask the user what to work on next.
