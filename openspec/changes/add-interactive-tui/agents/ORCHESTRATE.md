# Orchestrator: implement add-interactive-tui with parallel agents

You are the orchestrator. Spawn and sequence sub-agents; don't implement tasks yourself, except trivial fixes needed to unblock a merge. Work autonomously: don't ask the user questions, never push.

- `REPO`: the repository root (the directory containing this `openspec/` folder; resolve the absolute path with `git rev-parse --show-toplevel`).
- `AGENTS`: `$REPO/openspec/changes/add-interactive-tui/agents`
- Worktree parent: `$(dirname $REPO)`

## How to spawn agents
Use the Agent tool for every agent:
- `subagent_type: "general-purpose"`, `model: "sonnet"`
- **Don't** use `isolation: "worktree"`. It may branch from `origin/main`, which lacks our unpushed commits. Create worktrees yourself with `git worktree add` as below.
- Prompt template (fill in the bracketed parts):

  ```
  You are agent <ID> for the gctx project. Read and follow <ABSOLUTE PATH TO AGENT FILE>.
  Shared rules: <AGENTS>/shared-context.md.
  Your working directory is <ABSOLUTE WORKTREE OR REPO PATH>, branch <BRANCH>. Run every command there, using absolute paths.
  Work autonomously until every task in your file is done or provably blocked, then write and commit your report and return it as your final message.
  ```

Agents run in the background, and you are notified when each finishes. Never predict or invent an agent's result.

## Step 0: preflight
1. `git -C $REPO status --porcelain` must be empty, and the current branch must be `feat/interactive-tui`. If there are uncommitted changes, commit them as `chore: wip before orchestration`.
2. `go -C $REPO test -short ./...` must pass. If it doesn't, stop and report.

## Step 1: Stage A, plus the docs agent in parallel
1. Create the docs worktree from the current tip:
   `git -C $REPO worktree add -b tui/docs <parent>/gctx-wt-docs feat/interactive-tui`
2. In a single message, spawn both:
   - `stage-a`: agent file `$AGENTS/stage-a-foundation.md`, working dir `$REPO`, branch `feat/interactive-tui`.
   - `b4-docs`: agent file `$AGENTS/b4-docs.md`, working dir `<parent>/gctx-wt-docs`, branch `tui/docs`.
3. Wait for `stage-a` to finish. Then verify:
   - `$AGENTS/reports/stage-a.md` exists.
   - `go -C $REPO build ./... && go -C $REPO test -short ./...` passes.
   - The contract files exist: `pkg/tui/{service,model,run,keys,styles,main_keys,view,modals,exec,tokenize}.go`.

   If verification fails, use SendMessage to `stage-a` with the failure output and wait again. Allow at most 2 retries, then stop and report.

## Step 2: parallel B agents
1. Create one worktree per agent from the new tip:
   ```
   for b in views mutations exec; do git -C $REPO worktree add -b tui/$b <parent>/gctx-wt-$b feat/interactive-tui; done
   ```
2. In a single message, spawn all three:
   - `b1-views`: `$AGENTS/b1-views.md`, dir `<parent>/gctx-wt-views`, branch `tui/views`
   - `b2-mutations`: `$AGENTS/b2-mutations.md`, dir `<parent>/gctx-wt-mutations`, branch `tui/mutations`
   - `b3-exec`: `$AGENTS/b3-exec.md`, dir `<parent>/gctx-wt-exec`, branch `tui/exec`
3. As each agent finishes (including `b4-docs` from step 1), check that its worktree builds and its tests pass: `go -C <worktree> build ./... && go -C <worktree> test -short ./pkg/tui/... ./cmd/...`.
   - Also check that it edited only the files it owns: `git -C <worktree> diff --name-only feat/interactive-tui...HEAD` compared against the ownership list in its agent file.
   - If either check fails, SendMessage the agent with details and wait (at most 2 retries).

## Step 3: Stage C (integration)
When all four B agents are done and verified, spawn `stage-c`:
- Agent file `$AGENTS/stage-c-integration.md`, working dir `$REPO`, branch `feat/interactive-tui`.
- Append to its prompt the branch names (`tui/views`, `tui/mutations`, `tui/exec`, `tui/docs`) and worktree paths, plus the full text of the four B reports.

Wait for it, then verify:
- `go -C $REPO test ./...` passes.
- `openspec validate add-interactive-tui --strict` passes.
- `git -C $REPO worktree list` shows only the main checkout.

If anything fails, SendMessage `stage-c` (at most 2 retries).

## Step 4: final report to the user
Summarize:
- commits added (`git -C $REPO log --oneline main..feat/interactive-tui`)
- the tasks.md completion count
- tasks left unchecked, with reasons
- deviations reported by agents
- final test results
- next steps for the user:
  - push and check CI on 3 OSes
  - run the manual matrix in test-plan.md (including Windows E2E-12)
  - open a PR from `feat/interactive-tui`
  - after release, run `openspec archive add-interactive-tui`
