# GF-001 — Fix `go install` failure: unused syscall import and missing version tag

**Status:** Done

**Severity:** Blocker (prevents installation)

**Reported:** 2026-06-24

---

## Problem

Running `go install github.com/shahriarhossain/gitfence@latest` fails with:

```
main.go:7:2: "syscall" imported and not used
main.go:90:6: use of package syscall not in selector
```

Three root causes:

1. **Unused `syscall` import.** During the initial commit (`c88b3cc`), `main.go`
   imported `syscall` but never used it in a selector expression. The Go compiler
   rejects unused imports.

2. **No semver tag.** Without a tag, `go install @latest` resolves to a
   pseudo-version tied to the latest commit. The Go module proxy
   (`proxy.golang.org`) cached the broken commit `c88b3cc` as
   `v0.0.0-20260624220111-c88b3cc9a6f6`. Even after the fix was pushed
   (`97bcdee`), the proxy continued serving the cached broken version because
   there was no explicit version tag to supersede it.

3. **Non-ASCII filename.** The initial ticket file used an em dash (`—`) in the
   filename. Go's module zip format rejects non-ASCII characters in file paths,
   which broke `go install` even after the code was fixed. The file was renamed
   to use ASCII-only characters. Tag `v0.1.0` was already cached by the proxy
   with the broken filename, so `v0.1.1` was created as the first installable
   release.

## Fix

1. Removed the unused `syscall` import (commit `97bcdee`).
2. Renamed ticket file to ASCII-safe path (commit `8a1ad06`).
3. Created version tag `v0.1.1` on the fixed commit and pushed it to origin.

## Verification

After the fix, installation works:

```bash
go install github.com/shahriarhossain/gitfence@v0.1.1
gitfence status    # passes through to git — read-only, works
gitfence push      # blocked — mutating command
```

## What ships in v0.1.1

- Standalone git proxy binary (Fold 1 of TSF 252)
- Read-only allowlist: status, diff, log, show, branch --list, ls-files,
  config --get, remote -v, blame, shortlog, rev-parse, describe, ls-remote,
  cat-file, rev-list, name-rev, tag --list, stash list/show, reflog show,
  worktree list, notes list/show
- Mutating command blocking with structured stderr error
- Git global flag parsing (-C, -c, --git-dir, --work-tree, etc.)
- GITFENCE_BYPASS developer escape hatch
- Parser test suite
