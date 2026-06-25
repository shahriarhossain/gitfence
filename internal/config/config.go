package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	GatewayURL      string
	AgentID         string
	Token           string
	OfflineMode     string // "fail-closed" or "fail-open"
	WaitMode        string // "auto" (default), "on", or "off"
	WaitPollSeconds int    // seconds between approval polls (default 10)
}

func (c *Config) HasGateway() bool {
	return c.GatewayURL != "" && c.AgentID != "" && c.Token != ""
}

func GitPathFromEnv() string {
	return os.Getenv("GITFENCE_GIT_PATH")
}

func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gitfence")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "gitfence.toml")
}

func Load() *Config {
	cfg := &Config{OfflineMode: "fail-closed", WaitMode: "auto", WaitPollSeconds: 10}

	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"")

		switch key {
		case "gateway_url":
			cfg.GatewayURL = val
		case "agent_id":
			cfg.AgentID = val
		case "token":
			cfg.Token = val
		case "offline_mode":
			cfg.OfflineMode = val
		case "wait_mode":
			cfg.WaitMode = val
		case "wait_poll_seconds":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.WaitPollSeconds = n
			}
		}
	}

	return cfg
}

func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	content := fmt.Sprintf(`gateway_url = "%s"
agent_id = "%s"
token = "%s"
offline_mode = "%s"
wait_mode = "%s"
wait_poll_seconds = %d
`, cfg.GatewayURL, cfg.AgentID, cfg.Token, cfg.OfflineMode, cfg.WaitMode, cfg.WaitPollSeconds)

	path := ConfigPath()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}
	return nil
}
