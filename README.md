# gitfence

A drop-in git wrapper that prevents AI agents from executing destructive
git commands. Install it once, and every `git` call in that environment
is filtered — read-only commands pass through instantly, mutating
commands are blocked before they touch your repository.

## Why gitfence exists

AI agents are writing code. They commit, push, rebase, and reset — the
same git operations any developer uses. The difference: an agent doesn't
understand consequences. It can hallucinate a reason to force-push to
main, delete a branch it considers "stale," or run `git reset --hard`
because it thinks that's the fastest way to a clean state. It doesn't
realize it just broke a CI pipeline, wiped uncommitted work, or
overwrote someone's branch.

The problem is that `git` was never designed with autonomous agents in
mind. Read and write operations live in the same binary, one flag apart.
`git log` is safe. `git push --force` rewrites history. There's no
separation between "inspect the repo" and "mutate the repo." Giving an
agent access to `git` for status checks implicitly gives it access to
every destructive operation git offers.

The current workarounds are too blunt:

- **Remove git entirely.** The agent can't push, but it also can't
  `git diff` or `git log` — losing the read-only operations that make
  it useful.
- **Read-only VMs.** Every write syscall is blocked at the OS level.
  The agent can't even write a file, let alone push.
- **Prompt instructions.** "Don't push to main" works until the model
  hallucinates a reason to ignore the instruction. Prompt-based
  guardrails are probabilistic, not deterministic.

gitfence is a deterministic alternative. It parses every git command
before execution and applies a strict allowlist. Read-only commands
pass through to native git. Mutating commands are blocked with a clear
error message that tells the agent what happened and what to do instead.

## Two modes

### Standalone mode (zero config)

Block all mutating commands. No network calls, no external dependencies.
Install and it works.

```
Agent runs `git push origin main`
        |
        v
 [gitfence parser]
        |
        |-- Read-only command?  -->  Pass through to native git (zero overhead)
        |
        |-- Mutating command?   -->  Block + return structured error
```

### Governed mode (with a policy gateway)

Connect gitfence to a policy gateway for granular control — allow
pushes to feature branches, block pushes to main, route force-pushes
to human approval. The gateway evaluates policy; gitfence executes
the decision.

```
Agent runs `git push origin main`
        |
        v
 [gitfence parser]
        |
        |-- Read-only?   -->  Pass through locally (no gateway call)
        |
        |-- Mutating?    -->  POST /git/evaluate to gateway
                |
                |-- ALLOW            -->  Execute via native git
                |-- BLOCK            -->  Return error to agent
                |-- PENDING_APPROVAL -->  Wait for human approval, then execute
```

gitfence is **gateway-agnostic**. It works with any HTTP service that
implements two endpoints. See [Gateway Contract](#gateway-contract)
for the full specification.

## Installation

### From source (Go 1.22+)

```bash
go install github.com/shahriarhossain/gitfence@latest
```

### Using Homebrew (macOS/Linux)

```bash
brew install shahriarhossain/tap/gitfence
```

### Manual download

Download the binary for your platform from the
[releases page](https://github.com/shahriarhossain/gitfence/releases),
place it in your PATH, and make it executable.

## Setup

### Standalone mode (block all mutations)

```bash
gitfence init
```

This creates a shell wrapper so `git` resolves to `gitfence`. The
agent runs all git commands through gitfence transparently. All
mutating commands are blocked.

### Governed mode (with a policy gateway)

```bash
gitfence init \
  --gateway=https://your-gateway.example.com \
  --agent-id=oncall-bot \
  --token=your-auth-token
```

Mutating commands are forwarded to the gateway for policy evaluation.
The gateway decides: ALLOW, BLOCK, or PENDING_APPROVAL.

### Docker / container setup

```dockerfile
COPY --from=shahriarhossain/gitfence:latest /usr/local/bin/gitfence /usr/local/bin/gitfence
RUN gitfence init --symlink
```

### Verify installation

```bash
which git                # should point to ~/.gitfence/bin/git
git status               # works normally — read-only, passes through
git push                 # blocked (standalone) or gateway-evaluated (governed)
```

## Gateway Contract

gitfence talks to **any** HTTP service that implements these two
endpoints. There is no vendor lock-in — build your own gateway, use
an existing one, or run without one entirely.

### Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/git/evaluate` | Evaluate a mutating command against policy |
| `GET` | `/git/approval/{id}` | Poll the status of a pending approval |

### `POST /git/evaluate`

Called before every mutating command. The gateway evaluates policy
and returns a decision.

**Request:**

```json
{
  "agent_id": "oncall-bot",
  "command": "push",
  "arguments": {
    "command": "push",
    "remote": "origin",
    "branch": "main",
    "force": "true"
  },
  "token": "auth-token",
  "context": {
    "head_sha": "abc123def456",
    "commit_message": "Fix: handle null case in parser",
    "commit_author": "oncall-bot <bot@company.com>",
    "diff_stat": "3 files changed, 42 insertions(+), 15 deletions(-)",
    "files_changed": ["src/parser.go", "internal/handler.go", "main.go"],
    "remote_url": "https://github.com/acme/repo.git"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agent_id` | string | yes | Identifier for the agent making the request |
| `command` | string | yes | Git subcommand (`push`, `commit`, `reset`, etc.) |
| `arguments` | object | yes | Parsed command arguments (see below) |
| `token` | string | yes | Authentication token for the gateway |
| `context` | object | no | Repository state at the time of the request |

**Arguments by command:**

| Command | Fields sent |
|---------|-------------|
| `push` | `command`, `remote`, `branch`, `force` |
| `commit` | `command`, `message` |
| `checkout` / `switch` | `command`, `branch` |
| `merge` / `rebase` | `command`, `branch` |
| `branch` (create/delete) | `command`, `branch` |
| Other mutating commands | `command`, `target` (last positional arg) |

When `git push` is run without explicit arguments, gitfence resolves
the current branch and tracking remote automatically.

**Context fields** (all optional, best-effort):

| Field | Source | Description |
|-------|--------|-------------|
| `head_sha` | `git rev-parse HEAD` | Current commit SHA |
| `commit_message` | `git log -1 --format=%s` | Latest commit message |
| `commit_author` | `git log -1 --format=%an <%ae>` | Latest commit author |
| `diff_stat` | `git diff --stat HEAD~1` | Diff statistics |
| `files_changed` | `git diff --name-only HEAD~1` | List of changed files |
| `remote_url` | `git remote get-url origin` | Remote repository URL |

**Response:**

```json
{
  "decision": "BLOCK",
  "message": "Push to 'main' blocked by branch protection policy.",
  "remediation": "Push to a feature branch (e.g. agent/fix-123) and open a pull request.",
  "policy_id": "branch-protection-001",
  "approval_id": ""
}
```

| Field | Type | Description |
|-------|------|-------------|
| `decision` | string | `ALLOW`, `BLOCK`, or `PENDING_APPROVAL` |
| `message` | string | Human-readable explanation of the decision |
| `remediation` | string | Suggested alternative action (for BLOCK) |
| `policy_id` | string | ID of the policy rule that matched |
| `approval_id` | string | Approval request ID (for PENDING_APPROVAL) |

**Decision behavior:**

| Decision | What gitfence does |
|----------|--------------------|
| `ALLOW` | Execute the command via native git |
| `BLOCK` | Print the message and remediation, exit code 1 |
| `PENDING_APPROVAL` | Enter wait mode (poll for approval), or exit with approval ID |

### `GET /git/approval/{id}`

Called by gitfence to poll the status of a pending approval request.

**Response:**

```json
{
  "status": "APPROVED",
  "approval_id": "ap_xxxxxxxxxxxx",
  "agent_id": "oncall-bot",
  "tool_id": "github",
  "method": "git_force_push",
  "message": "",
  "context": {
    "head_sha": "abc123def456"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `PENDING`, `APPROVED`, `DENIED`, or `TIMED_OUT` |
| `approval_id` | string | The approval request ID |
| `context` | object | The stored context from the original request |

**Status behavior:**

| Status | What gitfence does |
|--------|--------------------|
| `PENDING` | Keep polling |
| `APPROVED` | Verify HEAD SHA matches, then execute the command |
| `DENIED` | Print denial message, exit code 1 |
| `TIMED_OUT` | Print timeout message, exit code 1 |

### State integrity

When gitfence receives `APPROVED`, it checks whether the repository
state has changed since the approval was requested:

1. Compares the current `HEAD` SHA against the `head_sha` from the
   original request context.
2. If they match, the command executes.
3. If they differ, the command is blocked with a message explaining
   that the approval was for a different repo state.

This prevents a class of vulnerability where an approval for one set
of changes is used to push a different set of changes.

### Implementing your own gateway

A minimal gateway needs to:

1. Accept `POST /git/evaluate` and return a JSON response with a
   `decision` field (`ALLOW`, `BLOCK`, or `PENDING_APPROVAL`).
2. If using HITL approvals, implement `GET /git/approval/{id}` to
   return the status of pending requests.
3. Store the `context` from evaluate requests so it can be returned
   via the approval endpoint (needed for state integrity checks).

A gateway that always allows push to feature branches and blocks
push to main can be implemented in ~50 lines of code. No specific
framework or language is required — gitfence communicates over
plain HTTP/JSON.

## Wait Mode

When a command is routed to human approval (`PENDING_APPROVAL`),
gitfence stays alive and polls the gateway until the request is
resolved:

```
gitfence: 'push' requires human approval

  Force-push rewrites remote history and can destroy other
  contributors' work. Human approval required.

  Approval ID: ap_xxxxxxxxxxxx
  Waiting for approval... (Ctrl+C to exit)

  ... [polling every 10s] ...

  Approved — executing command
  [git output follows]
```

- **Ctrl+C** exits cleanly and prints the approval ID for later
  use with `gitfence check <id>`.
- **Auto mode** (default): wait mode is on in interactive terminals,
  off in CI/scripts. Configurable via `wait_mode` in config.
- **State integrity**: before executing, gitfence verifies `HEAD`
  hasn't changed since the approval request (see
  [State integrity](#state-integrity)).

## Supported commands

### Allowed (read-only) — pass through to native git

| Command | Flags | What it does |
|---------|-------|--------------|
| `git status` | all flags | Show working tree status |
| `git diff` | all flags | Show changes between commits, working tree, etc. |
| `git log` | all flags | Show commit history |
| `git show` | all flags | Show commit details, tags, etc. |
| `git branch` | `--list`, `-l`, `-a`, `-r`, `-v`, `--merged`, `--no-merged` | List branches (create/delete/rename blocked) |
| `git ls-files` | all flags | List tracked files |
| `git config` | `--get`, `--list`, `--get-all`, `--get-regexp` | Read git configuration (set/unset blocked) |
| `git remote` | `-v`, `show` | View remote info (add/remove/rename blocked) |
| `git blame` | all flags | Show line-by-line authorship |
| `git shortlog` | all flags | Summarize commit history |
| `git rev-parse` | all flags | Parse revision identifiers |
| `git describe` | all flags | Describe a commit using tags |
| `git ls-remote` | all flags | List references in a remote repo |
| `git cat-file` | all flags | Inspect git objects |
| `git rev-list` | all flags | List commit objects |
| `git name-rev` | all flags | Find symbolic names for revisions |
| `git tag` | `--list`, `-l` | List tags (create/delete blocked) |
| `git stash` | `list`, `show` | View stashed changes (push/pop/drop blocked) |
| `git reflog` | `show` | View reflog (expire/delete blocked) |
| `git worktree` | `list` | List worktrees (add/remove blocked) |
| `git notes` | `list`, `show` | View notes (add/remove blocked) |

### Blocked (mutating) — requires gateway or blocked outright

| Command | What it does | Why it's blocked |
|---------|-------------|-----------------|
| `git push` | Push to remote | Can deploy unreviewed code, overwrite branches |
| `git push --force` | Force-push to remote | Rewrites remote history, destroys others' work |
| `git commit` | Create a commit | Captures a snapshot of changes in history |
| `git reset` | Reset current HEAD | `--hard` discards uncommitted work permanently |
| `git checkout` | Switch branches / restore files | Can overwrite uncommitted changes |
| `git switch` | Switch branches | Can overwrite uncommitted changes |
| `git restore` | Restore working tree files | Discards uncommitted modifications |
| `git rebase` | Rebase commits | Rewrites commit history |
| `git merge` | Merge branches | Modifies branch history |
| `git cherry-pick` | Apply commits from other branches | Modifies current branch history |
| `git revert` | Create a revert commit | Modifies branch history |
| `git rm` | Remove files from tracking | Deletes files from working tree and index |
| `git mv` | Move/rename files | Modifies working tree |
| `git clean` | Remove untracked files | Deletes files permanently |
| `git stash push/pop/drop` | Modify stash | Can lose stashed work |
| `git tag -d / -a` | Create/delete tags | Modifies tag references |
| `git branch -d / -D / -m` | Delete/rename branches | Destroys branch references |
| `git pull` | Fetch + merge | Merge component modifies local branch |
| `git config --set/--unset` | Write git configuration | Modifies repository config |
| `git remote add/remove/rename` | Modify remotes | Changes remote configuration |

## Configuration

gitfence works with zero configuration in standalone mode. For
governed mode, create `~/.config/gitfence/gitfence.toml`:

```toml
# Policy gateway connection
gateway_url = "https://your-gateway.example.com"
agent_id = "oncall-bot"
token = "your-auth-token"

# What happens when the gateway is unreachable
# "fail-closed" (default) — block all mutating commands
# "fail-open"             — allow and log locally (risky)
offline_mode = "fail-closed"

# Wait mode for HITL approvals
# "auto" (default) — wait in interactive terminals, exit in CI
# "on"              — always wait
# "off"             — never wait (print approval ID and exit)
wait_mode = "auto"

# Seconds between approval status polls (default 10)
wait_poll_seconds = 10
```

| Key | Values | Default |
|-----|--------|---------|
| `gateway_url` | URL of the policy gateway | (none — standalone mode) |
| `agent_id` | Agent identifier | (none) |
| `token` | Gateway auth token | (none) |
| `offline_mode` | `fail-closed` / `fail-open` | `fail-closed` |
| `wait_mode` | `auto` / `on` / `off` | `auto` |
| `wait_poll_seconds` | Poll interval in seconds | `10` |

## Developer Mode (passthrough bypass)

If you're a developer working on your own code (not an agent), you may
need full git access while gitfence is installed. Set the
`GITFENCE_BYPASS` environment variable to make gitfence step aside:

```bash
export GITFENCE_BYPASS=true
git push origin main    # passes through to real git
```

When bypass is active, gitfence prints a warning on every command so you
don't forget it's on:

```
gitfence: bypass active (GITFENCE_BYPASS=true) — all commands pass through to git
```

| Scenario | Use bypass? |
|---|---|
| You're developing locally and need to push your own code | Yes |
| The governance gateway is not yet running | Yes |
| You're testing gitfence itself and need to verify blocking | No |
| Agent sandbox / container / CI runner | Never |

The bypass is controlled by the **environment**, not by the agent. In a
properly configured agent sandbox or container, the agent should not
have permission to set environment variables that affect the parent
shell. For defense-in-depth, pair gitfence with server-side branch
protection.

## FAQ

**Can an agent bypass gitfence?**
If gitfence is installed as the `git` binary (via symlink or PATH
override), the agent cannot bypass it without access to the real git
binary path. In a container where the real git binary is removed or
renamed, there is no bypass path.

**Does gitfence add latency?**
For read-only commands: effectively zero — argument parsing takes
microseconds. For governed mutating commands: one HTTP round-trip to
the gateway (typically <50ms on localhost, <200ms remote). For blocked
commands: the command never executes.

**Does gitfence depend on any specific gateway or platform?**
No. gitfence communicates over plain HTTP/JSON with two endpoints
(`POST /git/evaluate` and `GET /git/approval/{id}`). Any service
that implements the [Gateway Contract](#gateway-contract) works.
Without a gateway, gitfence operates in standalone mode and blocks
all mutations. If you're looking for a gateway that handles this
along with full AI governance (policy engine, audit ledger, HITL
approval queue, cost controls), check out
[Froda AI](https://www.froda.ai) — a runtime AI governance platform.

**What about `git add` and `git commit`?**
Both are classified as mutating. In standalone mode, they're blocked.
In governed mode, the gateway decides — most policies allow them since
they only affect the local repo.

**What about git aliases?**
gitfence resolves aliases before evaluation. If `git co` is aliased to
`git checkout`, the checkout rules apply.

**What about git hooks?**
gitfence operates before git hooks. If a command is blocked, no hooks
execute — the command never reaches git.

## License

MIT
