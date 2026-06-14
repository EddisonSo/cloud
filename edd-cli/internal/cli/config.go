package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Token      string `json:"token,omitempty"`
	BaseDomain string `json:"base_domain,omitempty"`
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".edd-config.json"
	}
	return filepath.Join(home, ".config", "ec", "config.json")
}

func loadConfig(path string) Config {
	var c Config
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func saveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// resolveToken: flag > EC_TOKEN env > config file. Returns token and base domain.
func resolveToken(flagToken, cfgPath string) (string, string) {
	cfg := loadConfig(cfgPath)
	base := cfg.BaseDomain
	if base == "" {
		base = "cloud.eddisonso.com"
	}
	if flagToken != "" {
		return flagToken, base
	}
	if env := os.Getenv("EC_TOKEN"); env != "" {
		return env, base
	}
	return cfg.Token, base
}
