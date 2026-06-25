package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/shahriarhossain/gitfence/internal/config"
	"github.com/shahriarhossain/gitfence/internal/parser"
)

type GitContext struct {
	HeadSHA       string   `json:"head_sha,omitempty"`
	CommitMessage string   `json:"commit_message,omitempty"`
	CommitAuthor  string   `json:"commit_author,omitempty"`
	DiffStat      string   `json:"diff_stat,omitempty"`
	FilesChanged  []string `json:"files_changed,omitempty"`
	RemoteURL     string   `json:"remote_url,omitempty"`
}

type EvaluateRequest struct {
	AgentID   string            `json:"agent_id"`
	Command   string            `json:"command"`
	Arguments map[string]string `json:"arguments"`
	Token     string            `json:"token"`
	Context   *GitContext       `json:"context,omitempty"`
}

type EvaluateResponse struct {
	Decision    string `json:"decision"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
	PolicyID    string `json:"policy_id"`
	ApprovalID  string `json:"approval_id"`
}

func Evaluate(cfg *config.Config, cmd parser.Command, rawArgs []string) (*EvaluateResponse, error) {
	arguments := extractArguments(cmd, rawArgs)
	ctx := CaptureContext(cfg)

	reqBody := EvaluateRequest{
		AgentID:   cfg.AgentID,
		Command:   cmd.Subcommand,
		Arguments: arguments,
		Token:     cfg.Token,
		Context:   ctx,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := cfg.GatewayURL + "/git/evaluate"
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("gateway unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gateway response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result EvaluateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse gateway response: %w", err)
	}

	return &result, nil
}

type ApprovalStatus struct {
	Status     string `json:"status"`
	ApprovalID string `json:"approval_id"`
	Message    string `json:"message"`
}

func CheckApproval(cfg *config.Config, approvalID string) (*ApprovalStatus, error) {
	url := cfg.GatewayURL + "/git/approval/" + approvalID
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("gateway unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result ApprovalStatus
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

// CaptureContext gathers repo state for the HITL approval context.
// Failures are silently ignored — context is best-effort.
func CaptureContext(cfg *config.Config) *GitContext {
	gitPath := findGitForContext()
	if gitPath == "" {
		return nil
	}

	ctx := &GitContext{}
	ctx.HeadSHA = gitOutput(gitPath, "rev-parse", "HEAD")
	ctx.CommitMessage = gitOutput(gitPath, "log", "-1", "--format=%s")
	ctx.CommitAuthor = gitOutput(gitPath, "log", "-1", "--format=%an <%ae>")
	ctx.DiffStat = gitOutput(gitPath, "diff", "--stat", "HEAD~1")
	ctx.RemoteURL = gitOutput(gitPath, "remote", "get-url", "origin")

	filesRaw := gitOutput(gitPath, "diff", "--name-only", "HEAD~1")
	if filesRaw != "" {
		ctx.FilesChanged = strings.Split(filesRaw, "\n")
	}

	if ctx.HeadSHA == "" {
		return nil
	}
	return ctx
}

// CaptureHeadSHA returns the current HEAD SHA, or empty string on failure.
func CaptureHeadSHA() string {
	gitPath := findGitForContext()
	if gitPath == "" {
		return ""
	}
	return gitOutput(gitPath, "rev-parse", "HEAD")
}

func gitOutput(gitPath string, args ...string) string {
	cmd := exec.Command(gitPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func findGitForContext() string {
	if envPath := config.GitPathFromEnv(); envPath != "" {
		return envPath
	}
	path, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	return path
}

func extractArguments(cmd parser.Command, rawArgs []string) map[string]string {
	args := make(map[string]string)
	args["command"] = cmd.Subcommand

	switch cmd.Subcommand {
	case "push":
		parsePushArgs(cmd.Args, args)
	case "commit":
		parseCommitArgs(cmd.Args, args)
	case "checkout", "switch":
		if len(cmd.Args) > 0 {
			args["branch"] = cmd.Args[len(cmd.Args)-1]
		}
	case "branch":
		if len(cmd.Args) > 0 {
			args["branch"] = cmd.Args[len(cmd.Args)-1]
		}
	case "merge", "rebase":
		if len(cmd.Args) > 0 {
			args["branch"] = cmd.Args[len(cmd.Args)-1]
		}
	default:
		if len(cmd.Args) > 0 {
			args["target"] = cmd.Args[len(cmd.Args)-1]
		}
	}

	return args
}

func parsePushArgs(cmdArgs []string, args map[string]string) {
	for _, a := range cmdArgs {
		if a == "--force" || a == "-f" || a == "--force-with-lease" {
			args["force"] = "true"
		}
	}

	positional := []string{}
	for _, a := range cmdArgs {
		if len(a) > 0 && a[0] != '-' {
			positional = append(positional, a)
		}
	}

	if len(positional) >= 1 {
		args["remote"] = positional[0]
	}
	if len(positional) >= 2 {
		args["branch"] = positional[1]
	}

	// Resolve implicit remote/branch when bare "git push" is used.
	// Without this, branch-level policy conditions won't match.
	if args["branch"] == "" {
		gitPath := findGitForContext()
		if gitPath != "" {
			if branch := gitOutput(gitPath, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
				args["branch"] = branch
			}
		}
	}
	if args["remote"] == "" {
		gitPath := findGitForContext()
		if gitPath != "" {
			branch := args["branch"]
			if branch != "" {
				if remote := gitOutput(gitPath, "config", "--get", "branch."+branch+".remote"); remote != "" {
					args["remote"] = remote
				}
			}
			if args["remote"] == "" {
				args["remote"] = "origin"
			}
		}
	}
}

func parseCommitArgs(cmdArgs []string, args map[string]string) {
	for i, a := range cmdArgs {
		if (a == "-m" || a == "--message") && i+1 < len(cmdArgs) {
			args["message"] = cmdArgs[i+1]
			break
		}
	}
}
