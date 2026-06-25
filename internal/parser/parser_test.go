package parser

import "testing"

func TestAlwaysReadOnly(t *testing.T) {
	cases := []struct {
		args []string
	}{
		{[]string{"status"}},
		{[]string{"status", "-s"}},
		{[]string{"diff"}},
		{[]string{"diff", "HEAD~1"}},
		{[]string{"log"}},
		{[]string{"log", "--oneline", "-10"}},
		{[]string{"show", "HEAD"}},
		{[]string{"ls-files"}},
		{[]string{"blame", "main.go"}},
		{[]string{"shortlog", "-sn"}},
		{[]string{"rev-parse", "HEAD"}},
		{[]string{"describe", "--tags"}},
		{[]string{"ls-remote", "origin"}},
		{[]string{"cat-file", "-t", "HEAD"}},
		{[]string{"rev-list", "--count", "HEAD"}},
	}
	for _, tc := range cases {
		cmd := Parse(tc.args)
		if cmd.IsMutating {
			t.Errorf("expected %v to be read-only, got mutating", tc.args)
		}
	}
}

func TestAlwaysMutating(t *testing.T) {
	cases := []struct {
		args []string
	}{
		{[]string{"push", "origin", "main"}},
		{[]string{"push", "--force"}},
		{[]string{"commit", "-m", "test"}},
		{[]string{"reset", "--hard", "HEAD~1"}},
		{[]string{"checkout", "main"}},
		{[]string{"rebase", "main"}},
		{[]string{"merge", "feature"}},
		{[]string{"cherry-pick", "abc123"}},
		{[]string{"revert", "abc123"}},
		{[]string{"rm", "file.txt"}},
		{[]string{"mv", "a.txt", "b.txt"}},
		{[]string{"clean", "-fd"}},
		{[]string{"pull", "origin", "main"}},
		{[]string{"add", "."}},
	}
	for _, tc := range cases {
		cmd := Parse(tc.args)
		if !cmd.IsMutating {
			t.Errorf("expected %v to be mutating, got read-only", tc.args)
		}
	}
}

func TestBranchReadOnly(t *testing.T) {
	readOnly := [][]string{
		{"branch"},
		{"branch", "--list"},
		{"branch", "-l"},
		{"branch", "-a"},
		{"branch", "-r"},
		{"branch", "-v"},
		{"branch", "--merged"},
	}
	for _, args := range readOnly {
		cmd := Parse(args)
		if cmd.IsMutating {
			t.Errorf("expected %v to be read-only", args)
		}
	}

	mutating := [][]string{
		{"branch", "-d", "old-branch"},
		{"branch", "-D", "old-branch"},
		{"branch", "-m", "old", "new"},
		{"branch", "new-branch"},
	}
	for _, args := range mutating {
		cmd := Parse(args)
		if !cmd.IsMutating {
			t.Errorf("expected %v to be mutating", args)
		}
	}
}

func TestConfigReadOnly(t *testing.T) {
	readOnly := [][]string{
		{"config", "--get", "user.name"},
		{"config", "--list"},
		{"config", "--get-all", "remote.origin.url"},
		{"config", "--get-regexp", "user.*"},
	}
	for _, args := range readOnly {
		cmd := Parse(args)
		if cmd.IsMutating {
			t.Errorf("expected %v to be read-only", args)
		}
	}

	mutating := [][]string{
		{"config", "--unset", "user.name"},
		{"config", "--edit"},
	}
	for _, args := range mutating {
		cmd := Parse(args)
		if !cmd.IsMutating {
			t.Errorf("expected %v to be mutating", args)
		}
	}
}

func TestStashReadOnly(t *testing.T) {
	cmd := Parse([]string{"stash", "list"})
	if cmd.IsMutating {
		t.Error("expected stash list to be read-only")
	}

	cmd = Parse([]string{"stash", "show"})
	if cmd.IsMutating {
		t.Error("expected stash show to be read-only")
	}

	cmd = Parse([]string{"stash"})
	if !cmd.IsMutating {
		t.Error("expected bare stash to be mutating")
	}

	cmd = Parse([]string{"stash", "pop"})
	if !cmd.IsMutating {
		t.Error("expected stash pop to be mutating")
	}
}

func TestUnknownSubcommandBlocked(t *testing.T) {
	cmd := Parse([]string{"some-unknown-command"})
	if !cmd.IsMutating {
		t.Error("expected unknown subcommand to be blocked by default")
	}
}

func TestEmptyArgs(t *testing.T) {
	cmd := Parse([]string{})
	if cmd.IsMutating {
		t.Error("expected empty args to be non-mutating")
	}
}

func TestGlobalFlagsBeforeSubcommand(t *testing.T) {
	cmd := Parse([]string{"-C", "/some/path", "status"})
	if cmd.IsMutating {
		t.Errorf("expected '-C /some/path status' to be read-only")
	}

	cmd = Parse([]string{"-C", "/some/path", "push", "origin", "main"})
	if !cmd.IsMutating {
		t.Errorf("expected '-C /some/path push origin main' to be mutating")
	}
}
