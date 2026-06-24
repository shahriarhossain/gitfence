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

## How it works

```
Agent runs `git push origin main`
        |
        v
 [gitfence parser]
        |
        |-- Read-only command?  -->  Pass through to native git (zero overhead)
        |
        |-- Mutating command?   -->  Block + return structured error
                                     {
                                       "error": "gitfence: blocked",
                                       "command": "push",
                                       "reason": "Mutating git commands are not permitted in this environment.",
                                       "suggestion": "Use read-only commands (status, diff, log) or request write access through your organization's governance policy."
                                     }
```

No network calls, no external dependencies, no configuration required
for the default mode. Install and it works.

## Installation

### From source (Go)

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

### Activate for the current environment

```bash
gitfence init
```

This creates a shell alias so `git` resolves to `gitfence` in the
current environment. The agent (or anyone in that session) now runs
all git commands through gitfence transparently.

### Activate for a Docker container / agent sandbox

```dockerfile
# Install gitfence as the git binary inside the container
COPY --from=shahriarhossain/gitfence:latest /usr/local/bin/gitfence /usr/local/bin/gitfence
RUN ln -sf /usr/local/bin/gitfence /usr/local/bin/git
```

### Verify installation

```bash
git status       # works normally — read-only, passes through
git push         # blocked — mutating command
```

Expected output for the blocked command:

```
gitfence: blocked 'push'

  Mutating git commands are not permitted in this environment.
  Allowed commands: status, diff, log, show, branch --list, ls-files,
                    config --get, remote -v, blame, shortlog, rev-parse

  To allow governed writes, connect gitfence to a policy gateway.
  Run: gitfence init --help
```

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

### Blocked (mutating) — returns structured error

| Command | What it does | Why it's blocked |
|---------|-------------|-----------------|
| `git push` | Push to remote | Can deploy unreviewed code, overwrite branches |
| `git push --force` | Force-push to remote | Rewrites remote history, destroys others' work |
| `git commit` | Create a commit | Captures a snapshot of changes in history |
| `git reset` | Reset current HEAD | `--hard` discards uncommitted work permanently |
| `git reset --hard` | Hard reset | Destroys working tree changes irreversibly |
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
| `git fetch` | Fetch from remote | Allowed by default, blocked only with refspecs that write |
| `git pull` | Fetch + merge | Merge component modifies local branch |
| `git submodule update` | Update submodules | Modifies working tree |
| `git config --set/--unset` | Write git configuration | Modifies repository config |
| `git remote add/remove/rename` | Modify remotes | Changes remote configuration |
| `git reflog expire/delete` | Modify reflog | Destroys recovery data |
| `git notes add/remove` | Modify notes | Changes commit metadata |
| `git worktree add/remove` | Modify worktrees | Creates/destroys working directories |

### Side-by-side: what changes for the agent

| What the agent wants to do | Without gitfence | With gitfence |
|---|---|---|
| Check repo status | `git status` | `git status` (unchanged) |
| View recent commits | `git log --oneline -10` | `git log --oneline -10` (unchanged) |
| See what changed | `git diff HEAD~1` | `git diff HEAD~1` (unchanged) |
| View branch list | `git branch -a` | `git branch -a` (unchanged) |
| Check file blame | `git blame src/main.py` | `git blame src/main.py` (unchanged) |
| Push to main | `git push origin main` | Blocked: "Mutating command not permitted" |
| Force-push | `git push --force` | Blocked: "Mutating command not permitted" |
| Hard reset | `git reset --hard HEAD~3` | Blocked: "Mutating command not permitted" |
| Delete a branch | `git branch -D feature/old` | Blocked: "Mutating command not permitted" |
| Rewrite history | `git rebase -i HEAD~5` | Blocked: "Mutating command not permitted" |
| Clean untracked files | `git clean -fd` | Blocked: "Mutating command not permitted" |

## Configuration

gitfence works with zero configuration. For advanced use cases, create
`~/.config/gitfence/config.toml`:

```toml
# Path to the real git binary (auto-detected if not set)
git_path = "/usr/bin/git"

# What to do when a mutating command is blocked
# "error" (default) — print structured error to stderr, exit 1
# "json"            — print JSON error to stdout (for agent tool parsing)
output_format = "error"

# Additional read-only commands to allow (beyond the default set)
# Use this for custom aliases or plumbing commands
extra_allow = ["git-lfs ls-files"]
```

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

To disable bypass, unset the variable:

```bash
unset GITFENCE_BYPASS
```

### How the bypass works internally

```go
package main

import (
    "fmt"
    "os"
    "os/exec"
)

func main() {
    if os.Getenv("GITFENCE_BYPASS") == "true" {
        fmt.Fprintln(os.Stderr, "gitfence: bypass active (GITFENCE_BYPASS=true) — all commands pass through to git")
        passThroughToRealGit(os.Args[1:])
        return
    }

    // Standard gitfence blocking logic...
}
```

### When to use bypass

| Scenario | Use bypass? |
|---|---|
| You're developing locally and need to push your own code | Yes |
| The governance gateway is not yet running | Yes |
| You're testing gitfence itself and need to verify blocking | No — leave bypass off |
| Agent sandbox / container / CI runner | Never — don't set the variable |

### Security note

The bypass is controlled by the **environment**, not by the agent. In a
properly configured agent sandbox or container, the agent should not
have permission to set environment variables that affect the parent
shell. The bypass is designed for human developers who control their own
terminal session — not for agents.

For defense-in-depth:
- Don't set `GITFENCE_BYPASS` in the agent's container image or
  environment config.
- If you use Docker, the variable is absent by default — gitfence
  enforces fully.
- Pair with server-side branch protection as a second layer.

## FAQ

**Can an agent bypass gitfence?**
If gitfence is installed as the `git` binary (via symlink or PATH
override), the agent cannot bypass it without access to the real git
binary path. The `GITFENCE_BYPASS` env var exists for human developers
(see Developer Mode above), but in a container environment where the
variable is not set and the real git binary is removed or renamed, there
is no bypass path. For defense-in-depth, pair gitfence with server-side
branch protection rules.

**Does gitfence add latency?**
For read-only commands: effectively zero — it parses the command
arguments (microseconds) and passes through to native git. For blocked
commands: the command never executes, so it's faster than letting it
fail on the server side.

**What about `git add` and `git commit`?**
Both are blocked by default. `git add` stages changes (modifies the
index) and `git commit` creates a snapshot in history. If your workflow
requires agents to commit locally (but not push), see the governed
mode documentation.

**What about git aliases?**
gitfence resolves aliases before evaluation. If `git co` is aliased to
`git checkout`, the checkout rules apply.

**What about git hooks?**
gitfence operates before git hooks. If a command is blocked by gitfence,
no git hooks execute — the command never reaches git.

## License

MIT
