package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/shahriarhossain/gitfence/internal/parser"
)

func main() {
	if os.Getenv("GITFENCE_BYPASS") == "true" {
		fmt.Fprintln(os.Stderr, "gitfence: bypass active (GITFENCE_BYPASS=true) — all commands pass through to git")
		passThroughToGit(os.Args[1:])
		return
	}

	args := os.Args[1:]
	if len(args) == 0 {
		passThroughToGit(args)
		return
	}

	cmd := parser.Parse(args)

	if cmd.IsMutating {
		fmt.Fprintf(os.Stderr, "gitfence: blocked '%s'\n\n", cmd.Subcommand)
		fmt.Fprintln(os.Stderr, "  Mutating git commands are not permitted in this environment.")
		fmt.Fprintln(os.Stderr, "  Allowed commands: status, diff, log, show, branch --list, ls-files,")
		fmt.Fprintln(os.Stderr, "                    config --get, remote -v, blame, shortlog, rev-parse")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  To allow governed writes, connect gitfence to a policy gateway.")
		fmt.Fprintln(os.Stderr, "  Run: gitfence init --help")
		os.Exit(1)
	}

	passThroughToGit(args)
}

func passThroughToGit(args []string) {
	gitPath, err := findGit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitfence: cannot find git binary: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(gitPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func findGit() (string, error) {
	if envPath := os.Getenv("GITFENCE_GIT_PATH"); envPath != "" {
		return envPath, nil
	}

	self, _ := os.Executable()

	path, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git not found in PATH")
	}

	selfReal, _ := realPath(self)
	pathReal, _ := realPath(path)
	if selfReal == pathReal {
		return "", fmt.Errorf("git resolves to gitfence itself — set GITFENCE_GIT_PATH to the real git binary")
	}

	return path, nil
}

func realPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	resolved, err := os.Readlink(p)
	if err != nil {
		return p, nil
	}
	_ = syscall
	return resolved, nil
}
