package ctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ProjectRoot string
	WebWordlist string
}

const ConfigKeyProjectRoot = "project.root"
const ConfigKeyWebWordlist = "web.wordlist"

type ConfigEntry struct {
	Key   string
	Value string
}

func configPath() string {
	return filepath.Join(dataRoot(), "config.toml")
}

func LoadConfig() (*Config, error) {
	path := configPath()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	var config Config
	section := ""
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if section != "project" {
			if section != "web" {
				continue
			}
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		parsed, err := strconv.Unquote(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("failed to parse project root in %s: %w", path, err)
		}
		switch {
		case section == "project" && strings.TrimSpace(key) == "root":
			config.ProjectRoot = parsed
		case section == "web" && strings.TrimSpace(key) == "wordlist":
			config.WebWordlist = parsed
		}
	}

	return &config, nil
}

func SaveConfig(config *Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	content := "[project]\nroot = " + strconv.Quote(config.ProjectRoot) + "\n"
	if config.WebWordlist != "" {
		content += "\n[web]\nwordlist = " + strconv.Quote(config.WebWordlist) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config %s: %w", path, err)
	}
	return nil
}

func GetConfigValue(key string) (string, error) {
	config, err := LoadConfig()
	if err != nil {
		return "", err
	}
	switch key {
	case ConfigKeyProjectRoot:
		return config.ProjectRoot, nil
	case ConfigKeyWebWordlist:
		return config.WebWordlist, nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

func SetConfigValue(key, value string) (string, error) {
	config, err := LoadConfig()
	if err != nil {
		return "", err
	}
	switch key {
	case ConfigKeyProjectRoot:
		root, err := expandPath(value)
		if err != nil {
			return "", err
		}
		config.ProjectRoot = root
		if err := SaveConfig(config); err != nil {
			return "", err
		}
		return root, nil
	case ConfigKeyWebWordlist:
		wordlist, err := expandPath(value)
		if err != nil {
			return "", err
		}
		config.WebWordlist = wordlist
		if err := SaveConfig(config); err != nil {
			return "", err
		}
		return wordlist, nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

func ListConfigValues() ([]ConfigEntry, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return []ConfigEntry{
		{Key: ConfigKeyProjectRoot, Value: config.ProjectRoot},
		{Key: ConfigKeyWebWordlist, Value: config.WebWordlist},
	}, nil
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %s: %w", path, err)
	}
	return absolute, nil
}
