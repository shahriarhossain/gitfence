package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	GatewayURL  string
	AgentID     string
	Token       string
	OfflineMode string // "fail-closed" or "fail-open"
}

func (c *Config) HasGateway() bool {
	return c.GatewayURL != "" && c.AgentID != "" && c.Token != ""
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
	cfg := &Config{OfflineMode: "fail-closed"}

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
`, cfg.GatewayURL, cfg.AgentID, cfg.Token, cfg.OfflineMode)

	path := ConfigPath()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}
	return nil
}
