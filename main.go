package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/shahriarhossain/gitfence/internal/config"
	"github.com/shahriarhossain/gitfence/internal/gateway"
	"github.com/shahriarhossain/gitfence/internal/parser"
	"github.com/shahriarhossain/gitfence/internal/setup"
)

var gitfenceSubcommands = map[string]bool{
	"init":       true,
	"deactivate": true,
	"version":    true,
	"check":      true,
}

// Commands that are gitfence-only (no git equivalent), so they work
// even when invoked as "git check <id>" via the wrapper.
var gitfenceExclusiveSubcommands = map[string]bool{
	"check":      true,
	"version":    true,
	"deactivate": true,
}

func main() {
	if os.Getenv("GITFENCE_BYPASS") == "true" {
		fmt.Fprintln(os.Stderr, "gitfence: bypass active (GITFENCE_BYPASS=true) — all commands pass through to git")
		passThroughToGit(os.Args[1:])
		return
	}

	args := os.Args[1:]

	if len(args) == 0 {
		// If invoked as "git" (via wrapper), pass through to real git.
		// If invoked as "gitfence", show usage.
		if isInvokedAsGit() {
			passThroughToGit(args)
		} else {
			printUsage()
		}
		return
	}

	if gitfenceSubcommands[args[0]] && (!isInvokedAsGit() || gitfenceExclusiveSubcommands[args[0]]) {
		runGitfenceCommand(args[0], args[1:])
		return
	}

	cmd := parser.Parse(args)

	if cmd.IsMutating {
		cfg := config.Load()
		if cfg.HasGateway() {
			handleGatewayEvaluation(cfg, cmd, args)
		} else {
			blockLocally(cmd)
		}
		return
	}

	passThroughToGit(args)
}

func isInvokedAsGit() bool {
	exe := filepath.Base(os.Args[0])
	exe = strings.TrimSuffix(exe, ".exe")
	exe = strings.TrimSuffix(exe, ".cmd")
	return exe == "git"
}

func runGitfenceCommand(subcmd string, args []string) {
	switch subcmd {
	case "init":
		setup.Init(args)
	case "deactivate":
		setup.Deactivate(args)
	case "version":
		fmt.Println("gitfence v0.2.0")
	case "check":
		checkApproval(args)
	}
}

func checkApproval(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gitfence check <approval-id>")
		os.Exit(1)
	}
	cfg := config.Load()
	if !cfg.HasGateway() {
		fmt.Fprintln(os.Stderr, "gitfence: no gateway configured. Run: gitfence init --gateway=...")
		os.Exit(1)
	}
	status, err := gateway.CheckApproval(cfg, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitfence: %v\n", err)
		os.Exit(1)
	}
	switch status.Status {
	case "APPROVED":
		fmt.Printf("gitfence: approval %s — APPROVED\n", args[0])
		fmt.Println("  You can now re-run your command.")
	case "PENDING":
		fmt.Printf("gitfence: approval %s — still waiting for human review\n", args[0])
	case "DENIED":
		fmt.Printf("gitfence: approval %s — DENIED\n", args[0])
		if status.Message != "" {
			fmt.Printf("  %s\n", status.Message)
		}
	case "TIMED_OUT":
		fmt.Printf("gitfence: approval %s — timed out (no reviewer responded)\n", args[0])
	default:
		fmt.Printf("gitfence: approval %s — %s\n", args[0], status.Status)
	}
}

func printUsage() {
	fmt.Println("gitfence — a drop-in git wrapper that blocks mutating commands")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  gitfence init              Activate gitfence as the git binary")
	fmt.Println("  gitfence init --symlink    Symlink gitfence as git (for containers)")
	fmt.Println("  gitfence deactivate        Remove gitfence wrapper and restore git")
	fmt.Println("  gitfence check <id>        Check approval status of a HITL request")
	fmt.Println("  gitfence version           Print version")
	fmt.Println("")
	fmt.Println("Once activated, use git normally — gitfence intercepts transparently.")
	fmt.Println("Read-only commands pass through. Mutating commands are blocked.")
}

func blockLocally(cmd parser.Command) {
	fmt.Fprintf(os.Stderr, "gitfence: blocked '%s'\n\n", cmd.Subcommand)
	fmt.Fprintln(os.Stderr, "  Mutating git commands are not permitted in this environment.")
	fmt.Fprintln(os.Stderr, "  Allowed commands: status, diff, log, show, branch --list, ls-files,")
	fmt.Fprintln(os.Stderr, "                    config --get, remote -v, blame, shortlog, rev-parse")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  To allow governed writes, connect gitfence to a policy gateway.")
	fmt.Fprintln(os.Stderr, "  Run: gitfence init --help")
	os.Exit(1)
}

func handleGatewayEvaluation(cfg *config.Config, cmd parser.Command, args []string) {
	headSHA := gateway.CaptureHeadSHA()

	resp, err := gateway.Evaluate(cfg, cmd, args)
	if err != nil {
		if cfg.OfflineMode == "fail-open" {
			fmt.Fprintf(os.Stderr, "gitfence: gateway unreachable, fail-open active — executing locally\n")
			passThroughToGit(args)
			return
		}
		fmt.Fprintf(os.Stderr, "gitfence: gateway error — %v\n\n", err)
		fmt.Fprintln(os.Stderr, "  Mutating commands are blocked when the gateway is unreachable (fail-closed mode).")
		fmt.Fprintln(os.Stderr, "  Check your gateway URL or set offline_mode = \"fail-open\" in config.")
		os.Exit(1)
	}

	switch resp.Decision {
	case "ALLOW":
		if resp.Message != "" {
			fmt.Fprintf(os.Stderr, "gitfence: %s\n", resp.Message)
		}
		passThroughToGit(args)

	case "BLOCK":
		fmt.Fprintf(os.Stderr, "gitfence: blocked '%s'\n\n", cmd.Subcommand)
		if resp.Message != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", resp.Message)
		}
		if resp.Remediation != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", resp.Remediation)
		}
		os.Exit(1)

	case "PENDING_APPROVAL":
		fmt.Fprintf(os.Stderr, "gitfence: '%s' requires human approval\n\n", cmd.Subcommand)
		if resp.Message != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", resp.Message)
		}
		if resp.ApprovalID != "" {
			fmt.Fprintf(os.Stderr, "\n  Approval ID: %s\n", resp.ApprovalID)
		}

		if shouldWait(cfg) && resp.ApprovalID != "" {
			fmt.Fprintf(os.Stderr, "  Waiting for approval... (Ctrl+C to exit)\n\n")
			waitForApproval(cfg, resp.ApprovalID, headSHA, args)
		} else {
			if resp.ApprovalID != "" {
				fmt.Fprintf(os.Stderr, "  Check status: gitfence check %s\n", resp.ApprovalID)
				fmt.Fprintln(os.Stderr, "  Once approved, re-run your command.")
			}
			os.Exit(2)
		}

	default:
		fmt.Fprintf(os.Stderr, "gitfence: unexpected gateway decision '%s'\n", resp.Decision)
		os.Exit(1)
	}
}

func shouldWait(cfg *config.Config) bool {
	switch cfg.WaitMode {
	case "on":
		return true
	case "off":
		return false
	default: // "auto"
		return isTerminal()
	}
}

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func waitForApproval(cfg *config.Config, approvalID string, headSHA string, args []string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	pollInterval := time.Duration(cfg.WaitPollSeconds) * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Fprintf(os.Stderr, "\n\ngitfence: interrupted\n")
			fmt.Fprintf(os.Stderr, "  Resume with: gitfence check %s\n", approvalID)
			fmt.Fprintf(os.Stderr, "  Once approved, re-run your command.\n")
			os.Exit(2)

		case <-ticker.C:
			status, err := gateway.CheckApproval(cfg, approvalID)
			if err != nil {
				// Transient error — keep polling
				continue
			}

			switch status.Status {
			case "APPROVED":
				executeAfterApproval(headSHA, args)
				return
			case "DENIED":
				fmt.Fprintf(os.Stderr, "  Denied")
				if status.Message != "" {
					fmt.Fprintf(os.Stderr, ": %s", status.Message)
				}
				fmt.Fprintln(os.Stderr)
				os.Exit(1)
			case "TIMED_OUT":
				fmt.Fprintln(os.Stderr, "  Timed out — no reviewer responded")
				os.Exit(1)
			}
			// PENDING — keep polling
		}
	}
}

func executeAfterApproval(originalSHA string, args []string) {
	if originalSHA != "" {
		currentSHA := gateway.CaptureHeadSHA()
		if currentSHA != "" && currentSHA != originalSHA {
			fmt.Fprintln(os.Stderr, "  Approved, but working tree changed since approval was requested")
			fmt.Fprintf(os.Stderr, "    Approved HEAD:  %s\n", truncSHA(originalSHA))
			fmt.Fprintf(os.Stderr, "    Current HEAD:   %s\n", truncSHA(currentSHA))
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "  Re-run the command to submit a new approval request.")
			os.Exit(1)
		}
	}
	fmt.Fprintln(os.Stderr, "  Approved — executing command")
	passThroughToGit(args)
}

func truncSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
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
	if envPath := config.GitPathFromEnv(); envPath != "" {
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
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p, nil
	}
	return resolved, nil
}
