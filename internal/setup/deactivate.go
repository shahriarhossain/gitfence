package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Deactivate(args []string) {
	symlink := false

	for _, arg := range args {
		switch arg {
		case "--symlink":
			symlink = true
		case "--help", "-h":
			printDeactivateHelp()
			return
		default:
			fmt.Fprintf(os.Stderr, "gitfence deactivate: unknown flag '%s'\n", arg)
			os.Exit(1)
		}
	}

	if symlink {
		deactivateSymlink()
	} else {
		deactivateWrapper()
	}
}

func deactivateSymlink() {
	targets := []string{"/usr/local/bin/git", "/usr/bin/git"}

	gitfencePath, _ := os.Executable()
	gitfencePath, _ = filepath.EvalSymlinks(gitfencePath)

	removed := false
	for _, gitLink := range targets {
		info, err := os.Lstat(gitLink)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(gitLink)
		if err != nil {
			continue
		}
		targetReal, _ := filepath.EvalSymlinks(target)
		if targetReal == gitfencePath || target == gitfencePath {
			if err := os.Remove(gitLink); err != nil {
				fmt.Fprintf(os.Stderr, "gitfence deactivate: cannot remove %s: %v\n", gitLink, err)
				fmt.Fprintln(os.Stderr, "  You may need to run this with sudo")
				os.Exit(1)
			}
			fmt.Printf("gitfence: removed symlink %s\n", gitLink)
			removed = true
		}
	}

	if !removed {
		fmt.Println("gitfence: no symlinks found pointing to gitfence")
	} else {
		fmt.Println("gitfence: deactivated — git now resolves to the real binary")
	}
}

func deactivateWrapper() {
	binDir := gitfenceBinDir()

	var wrapperPaths []string
	if runtime.GOOS == "windows" {
		wrapperPaths = []string{
			filepath.Join(binDir, "git.cmd"),
			filepath.Join(binDir, "git"),
		}
	} else {
		wrapperPaths = []string{filepath.Join(binDir, "git")}
	}

	removed := false
	for _, wrapperPath := range wrapperPaths {
		if _, err := os.Stat(wrapperPath); os.IsNotExist(err) {
			continue
		}

		content, err := os.ReadFile(wrapperPath)
		if err == nil && !strings.Contains(string(content), "gitfence") {
			fmt.Fprintf(os.Stderr, "gitfence deactivate: %s does not appear to be a gitfence wrapper — refusing to delete\n", wrapperPath)
			os.Exit(1)
		}

		if err := os.Remove(wrapperPath); err != nil {
			fmt.Fprintf(os.Stderr, "gitfence deactivate: cannot remove %s: %v\n", wrapperPath, err)
			os.Exit(1)
		}

		fmt.Printf("gitfence: removed %s\n", wrapperPath)
		removed = true
	}

	if !removed {
		fmt.Println("gitfence: no wrapper found — gitfence is not activated")
		return
	}

	entries, _ := os.ReadDir(binDir)
	if len(entries) == 0 {
		os.Remove(binDir)
	}

	fmt.Println("gitfence: deactivated — git now resolves to the real binary")

	if isInPath(binDir) {
		fmt.Println("")
		fmt.Printf("  You can also remove %s from your PATH.\n", binDir)
	}
}

func printDeactivateHelp() {
	fmt.Println("Usage: gitfence deactivate [flags]")
	fmt.Println("")
	fmt.Println("Remove the gitfence wrapper and restore git to the real binary.")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  --symlink    Remove the symlink (if gitfence was set up with --symlink)")
	fmt.Println("  --help, -h   Show this help")
}
