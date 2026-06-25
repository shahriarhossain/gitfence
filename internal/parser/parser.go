package parser

import "strings"

type Command struct {
	Subcommand string
	Args       []string
	IsMutating bool
	Reason     string
}

var readOnlyCommands = map[string]bool{
	"status":    true,
	"diff":      true,
	"log":       true,
	"show":      true,
	"ls-files":  true,
	"blame":     true,
	"shortlog":  true,
	"rev-parse": true,
	"describe":  true,
	"ls-remote": true,
	"cat-file":  true,
	"rev-list":  true,
	"name-rev":  true,
	"reflog":    false, // only with "show"
	"worktree":  false, // only with "list"
	"notes":     false, // only with "list", "show"
}

var alwaysReadOnly = map[string]bool{
	"status":    true,
	"diff":      true,
	"log":       true,
	"show":      true,
	"ls-files":  true,
	"blame":     true,
	"shortlog":  true,
	"rev-parse": true,
	"describe":  true,
	"ls-remote": true,
	"cat-file":  true,
	"rev-list":  true,
	"name-rev":  true,
}

var alwaysMutating = map[string]bool{
	"push":       true,
	"commit":     true,
	"reset":      true,
	"checkout":   true,
	"switch":     true,
	"restore":    true,
	"rebase":     true,
	"merge":      true,
	"cherry-pick": true,
	"revert":     true,
	"rm":         true,
	"mv":         true,
	"clean":      true,
	"pull":       true,
	"add":        true,
	"init":       true,
	"clone":      true,
	"submodule":  true,
	"bisect":     true,
	"am":         true,
	"apply":      true,
	"format-patch": true,
}

func Parse(args []string) Command {
	if len(args) == 0 {
		return Command{Subcommand: "", IsMutating: false}
	}

	subcommand := ""
	remaining := args

	// Git global flags that consume the next argument as a value
	globalFlagsWithValue := map[string]bool{
		"-C": true, "-c": true, "--git-dir": true,
		"--work-tree": true, "--namespace": true,
		"--super-prefix": true, "--config-env": true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if globalFlagsWithValue[arg] && i+1 < len(args) {
				i++ // skip the value argument
			}
			continue
		}
		subcommand = arg
		remaining = args[i+1:]
		break
	}

	if subcommand == "" {
		return Command{Subcommand: "", IsMutating: false}
	}

	if alwaysReadOnly[subcommand] {
		return Command{
			Subcommand: subcommand,
			Args:       remaining,
			IsMutating: false,
		}
	}

	if alwaysMutating[subcommand] {
		return Command{
			Subcommand: subcommand,
			Args:       remaining,
			IsMutating: true,
			Reason:     "mutating subcommand",
		}
	}

	// Commands that are read-only only with specific flags
	switch subcommand {
	case "branch":
		if isBranchReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "branch create/delete/rename requires governed access"}

	case "tag":
		if isTagReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "tag create/delete requires governed access"}

	case "config":
		if isConfigReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "config write requires governed access"}

	case "remote":
		if isRemoteReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "remote modification requires governed access"}

	case "stash":
		if isStashReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "stash mutation requires governed access"}

	case "reflog":
		if isReflogReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "reflog mutation requires governed access"}

	case "worktree":
		if isWorktreeReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "worktree mutation requires governed access"}

	case "notes":
		if isNotesReadOnly(remaining) {
			return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
		}
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: true, Reason: "notes mutation requires governed access"}

	case "fetch":
		return Command{Subcommand: subcommand, Args: remaining, IsMutating: false}
	}

	// Unknown subcommand — block by default (allowlist approach)
	return Command{
		Subcommand: subcommand,
		Args:       remaining,
		IsMutating: true,
		Reason:     "unknown subcommand — blocked by default",
	}
}

func isBranchReadOnly(args []string) bool {
	if len(args) == 0 {
		return true // bare "git branch" lists branches
	}
	for _, arg := range args {
		switch arg {
		case "--list", "-l", "-a", "-r", "-v", "-vv",
			"--merged", "--no-merged", "--contains", "--no-contains",
			"--sort", "--format", "--points-at":
			continue
		case "-d", "-D", "-m", "-M", "--delete", "--move", "-c", "-C", "--copy":
			return false
		}
		if strings.HasPrefix(arg, "--sort=") || strings.HasPrefix(arg, "--format=") ||
			strings.HasPrefix(arg, "--points-at=") || strings.HasPrefix(arg, "--contains=") {
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			// A positional argument after branch could be a new branch name
			return false
		}
	}
	return true
}

func isTagReadOnly(args []string) bool {
	if len(args) == 0 {
		return true // bare "git tag" lists tags
	}
	for _, arg := range args {
		switch arg {
		case "--list", "-l", "-n", "--contains", "--no-contains",
			"--merged", "--no-merged", "--sort", "--format", "--points-at":
			continue
		case "-d", "-a", "-s", "-f", "--delete", "--annotate", "--sign":
			return false
		}
		if strings.HasPrefix(arg, "--sort=") || strings.HasPrefix(arg, "--format=") ||
			strings.HasPrefix(arg, "--contains=") || strings.HasPrefix(arg, "-n") {
			continue
		}
	}
	return true
}

func isConfigReadOnly(args []string) bool {
	hasWriteFlag := false
	hasReadFlag := false
	for _, arg := range args {
		switch arg {
		case "--get", "--get-all", "--get-regexp", "--list", "-l",
			"--get-color", "--get-colorbool", "--get-urlmatch":
			hasReadFlag = true
		case "--unset", "--unset-all", "--replace-all", "--add",
			"--rename-section", "--remove-section", "-e", "--edit":
			hasWriteFlag = true
		}
	}
	if hasWriteFlag {
		return false
	}
	if hasReadFlag {
		return true
	}
	// "git config key" without explicit flags is a read (used by shell prompts).
	// "git config key value" is a write, but we can't reliably distinguish
	// without counting positional args. Treat as read-only since blocking
	// config reads breaks shell integration (PS1 prompts, git aliases).
	return true
}

func isRemoteReadOnly(args []string) bool {
	if len(args) == 0 {
		return true // bare "git remote" lists remotes
	}
	switch args[0] {
	case "-v", "show", "get-url":
		return true
	case "add", "remove", "rm", "rename", "set-url", "set-head", "prune":
		return false
	}
	return true
}

func isStashReadOnly(args []string) bool {
	if len(args) == 0 {
		return false // bare "git stash" = "git stash push" which is mutating
	}
	switch args[0] {
	case "list", "show":
		return true
	}
	return false
}

func isReflogReadOnly(args []string) bool {
	if len(args) == 0 {
		return true // bare "git reflog" = "git reflog show"
	}
	switch args[0] {
	case "show":
		return true
	case "expire", "delete":
		return false
	}
	return true
}

func isWorktreeReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "list":
		return true
	}
	return false
}

func isNotesReadOnly(args []string) bool {
	if len(args) == 0 {
		return true // bare "git notes" = list
	}
	switch args[0] {
	case "list", "show":
		return true
	}
	return false
}
