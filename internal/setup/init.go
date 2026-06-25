package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shahriarhossain/gitfence/internal/config"
)

func Init(args []string) {
	symlink := false
	targetDir := ""
	gatewayURL := ""
	agentID := ""
	token := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--symlink":
			symlink = true
		case "--target":
			if i+1 < len(args) {
				i++
				targetDir = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "gitfence init: --target requires a directory argument")
				os.Exit(1)
			}
		case "--gateway":
			if i+1 < len(args) {
				i++
				gatewayURL = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "gitfence init: --gateway requires a URL argument")
				os.Exit(1)
			}
		case "--agent-id":
			if i+1 < len(args) {
				i++
				agentID = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "gitfence init: --agent-id requires an argument")
				os.Exit(1)
			}
		case "--token":
			if i+1 < len(args) {
				i++
				token = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "gitfence init: --token requires an argument")
				os.Exit(1)
			}
		case "--help", "-h":
			printInitHelp()
			return
		default:
			if strings.HasPrefix(args[i], "--target=") {
				targetDir = strings.TrimPrefix(args[i], "--target=")
			} else if strings.HasPrefix(args[i], "--gateway=") {
				gatewayURL = strings.TrimPrefix(args[i], "--gateway=")
			} else if strings.HasPrefix(args[i], "--agent-id=") {
				agentID = strings.TrimPrefix(args[i], "--agent-id=")
			} else if strings.HasPrefix(args[i], "--token=") {
				token = strings.TrimPrefix(args[i], "--token=")
			} else {
				fmt.Fprintf(os.Stderr, "gitfence init: unknown flag '%s'\n", args[i])
				os.Exit(1)
			}
		}
	}

	if gatewayURL != "" {
		saveGatewayConfig(gatewayURL, agentID, token)
	}

	if symlink {
		initSymlink(targetDir)
	} else {
		initWrapper()
	}
}

func saveGatewayConfig(gatewayURL, agentID, token string) {
	if agentID == "" || token == "" {
		fmt.Fprintln(os.Stderr, "gitfence init: --gateway requires --agent-id and --token")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Example:")
		fmt.Fprintln(os.Stderr, "    gitfence init --gateway=http://localhost:8080 --agent-id=gittest-1 --token=abc123")
		os.Exit(1)
	}

	cfg := &config.Config{
		GatewayURL:  gatewayURL,
		AgentID:     agentID,
		Token:       token,
		OfflineMode: "fail-closed",
	}

	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("gitfence: gateway configured\n")
	fmt.Printf("  gateway:  %s\n", gatewayURL)
	fmt.Printf("  agent-id: %s\n", agentID)
	fmt.Printf("  config:   %s\n", config.ConfigPath())
	fmt.Println("")
}

func initSymlink(targetDir string) {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "gitfence init: --symlink is not supported on Windows")
		fmt.Fprintln(os.Stderr, "  Use 'gitfence init' without --symlink instead")
		os.Exit(1)
	}

	if targetDir == "" {
		targetDir = "/usr/local/bin"
	}

	gitfencePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: cannot determine own path: %v\n", err)
		os.Exit(1)
	}
	gitfencePath, _ = filepath.EvalSymlinks(gitfencePath)

	gitLink := filepath.Join(targetDir, "git")

	if info, err := os.Lstat(gitLink); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, _ := os.Readlink(gitLink)
			if existing == gitfencePath {
				fmt.Println("gitfence: already activated (symlink exists)")
				return
			}
		}
		realGitPath := findAndSaveRealGit(gitLink)
		os.Remove(gitLink)
		fmt.Fprintf(os.Stderr, "gitfence init: backed up existing git at %s\n", realGitPath)
	}

	if err := os.Symlink(gitfencePath, gitLink); err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: failed to create symlink: %v\n", err)
		fmt.Fprintln(os.Stderr, "  You may need to run this with sudo")
		os.Exit(1)
	}

	fmt.Printf("gitfence: activated (symlinked %s -> %s)\n", gitLink, gitfencePath)
	fmt.Println("")
	fmt.Println("  git commands now route through gitfence.")
	fmt.Println("  Read-only commands pass through. Mutating commands are blocked.")
	fmt.Println("")
	fmt.Println("  To undo: gitfence deactivate --symlink")
}

func initWrapper() {
	binDir := gitfenceBinDir()

	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: cannot create directory %s: %v\n", binDir, err)
		os.Exit(1)
	}

	gitfencePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: cannot determine own path: %v\n", err)
		os.Exit(1)
	}
	gitfencePath, _ = filepath.EvalSymlinks(gitfencePath)

	if runtime.GOOS == "windows" {
		writeWindowsWrapper(binDir, gitfencePath)
	} else {
		writeUnixWrapper(binDir, gitfencePath)
	}

	if isInPath(binDir) {
		fmt.Println("gitfence: activated")
	} else {
		fmt.Println("gitfence: wrapper created")
		fmt.Println("")
		printPathInstructions(binDir)
	}

	fmt.Println("")
	fmt.Println("  Once PATH is set, git commands route through gitfence.")
	fmt.Println("  Read-only commands pass through. Mutating commands are blocked.")
	fmt.Println("")
	fmt.Println("  To undo: gitfence deactivate")
}

func writeUnixWrapper(binDir, gitfencePath string) {
	realGit := findRealGitPath(binDir)
	wrapperPath := filepath.Join(binDir, "git")
	content := fmt.Sprintf("#!/bin/sh\n# Generated by gitfence init — do not edit\nGITFENCE_GIT_PATH=\"%s\" exec \"%s\" \"$@\"\n", realGit, gitfencePath)

	if err := os.WriteFile(wrapperPath, []byte(content), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: cannot write wrapper %s: %v\n", wrapperPath, err)
		os.Exit(1)
	}
	fmt.Printf("gitfence: created %s\n", wrapperPath)
}

func writeWindowsWrapper(binDir, gitfencePath string) {
	realGit := findRealGitPath(binDir)

	// .cmd for PowerShell and CMD
	cmdPath := filepath.Join(binDir, "git.cmd")
	cmdContent := fmt.Sprintf("@echo off\r\nset \"GITFENCE_GIT_PATH=%s\"\r\n\"%s\" %%*\r\n", realGit, gitfencePath)
	if err := os.WriteFile(cmdPath, []byte(cmdContent), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: cannot write wrapper %s: %v\n", cmdPath, err)
		os.Exit(1)
	}
	fmt.Printf("gitfence: created %s\n", cmdPath)

	// extensionless shell script for Git Bash / MINGW
	shPath := filepath.Join(binDir, "git")
	shContent := fmt.Sprintf("#!/bin/sh\n# Generated by gitfence init — do not edit\nGITFENCE_GIT_PATH=\"%s\" exec \"%s\" \"$@\"\n", realGit, gitfencePath)
	if err := os.WriteFile(shPath, []byte(shContent), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: cannot write wrapper %s: %v\n", shPath, err)
		os.Exit(1)
	}
	fmt.Printf("gitfence: created %s\n", shPath)
}

func findRealGitPath(excludeDir string) string {
	excludeAbs, _ := filepath.Abs(excludeDir)
	pathEnv := os.Getenv("PATH")
	sep := string(os.PathListSeparator)

	for _, dir := range strings.Split(pathEnv, sep) {
		dirAbs, _ := filepath.Abs(dir)
		if dirAbs == excludeAbs {
			continue
		}
		candidates := []string{"git"}
		if runtime.GOOS == "windows" {
			candidates = []string{"git.exe", "git.cmd", "git"}
		}
		for _, name := range candidates {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}

	path, err := exec.LookPath("git")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gitfence init: warning — could not find real git binary in PATH")
		fmt.Fprintln(os.Stderr, "  Set GITFENCE_GIT_PATH manually after activation")
		return ""
	}
	return path
}

func printPathInstructions(binDir string) {
	if runtime.GOOS == "windows" {
		fmt.Println("  Add to your PATH (run in PowerShell as Administrator):")
		fmt.Println("")
		fmt.Printf("    $oldPath = [Environment]::GetEnvironmentVariable('Path', 'User')\n")
		fmt.Printf("    [Environment]::SetEnvironmentVariable('Path', '%s;' + $oldPath, 'User')\n", binDir)
		fmt.Println("")
		fmt.Println("  Then restart your terminal.")
	} else {
		shell := detectShell()
		rcFile := shellRCFile(shell)
		fmt.Printf("  Add to your PATH (paste into %s):\n", rcFile)
		fmt.Println("")
		fmt.Printf("    export PATH=\"%s:$PATH\"\n", binDir)
		fmt.Println("")
		fmt.Printf("  Then run: source %s\n", rcFile)
	}
}

func printInitHelp() {
	fmt.Println("Usage: gitfence init [flags]")
	fmt.Println("")
	fmt.Println("Activate gitfence as the git binary in this environment.")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  --gateway=<url>    Policy gateway URL for command evaluation")
	fmt.Println("  --agent-id=<id>    Agent identifier registered in the policy gateway")
	fmt.Println("  --token=<token>    Authentication token for the gateway")
	fmt.Println("  --symlink          Create a symlink instead of a wrapper script")
	fmt.Println("                     (designed for Docker containers)")
	fmt.Println("  --target=<dir>     Directory for the symlink (default: /usr/local/bin)")
	fmt.Println("                     Only valid with --symlink")
	fmt.Println("  --help, -h         Show this help")
	fmt.Println("")
	fmt.Println("Without --gateway, gitfence blocks all mutating commands (free tier).")
	fmt.Println("With --gateway, mutating commands are forwarded to the policy gateway")
	fmt.Println("for policy evaluation (ALLOW / BLOCK / HITL).")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  gitfence init                           # standalone read-only mode")
	fmt.Println("  gitfence init --gateway=http://localhost:8080 \\")
	fmt.Println("                --agent-id=gittest-1 \\")
	fmt.Println("                --token=abc123             # gateway-connected mode")
	fmt.Println("  gitfence init --symlink                 # container mode")
}

func gitfenceBinDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitfence init: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".gitfence", "bin")
}

func isInPath(dir string) bool {
	pathEnv := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	for _, entry := range strings.Split(pathEnv, sep) {
		cleaned, _ := filepath.Abs(entry)
		target, _ := filepath.Abs(dir)
		if cleaned == target {
			return true
		}
	}
	return false
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "bash"
	}
	base := filepath.Base(shell)
	return base
}

func shellRCFile(shell string) string {
	home, _ := os.UserHomeDir()
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	default:
		return filepath.Join(home, ".bashrc")
	}
}

func findAndSaveRealGit(currentGitPath string) string {
	resolved, err := filepath.EvalSymlinks(currentGitPath)
	if err != nil {
		resolved = currentGitPath
	}

	backupPath := resolved + ".real"
	if _, err := os.Stat(backupPath); err == nil {
		return backupPath
	}

	path, err := exec.LookPath("git")
	if err != nil {
		return resolved
	}
	return path
}
