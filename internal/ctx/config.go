package ctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ProjectRoot          string
	DirectoryMaxRequests int
	FileMaxRequests      int
	PasswordMaxRequests  int
	TLSVerify            bool
}

const ConfigKeyProjectRoot = "project.root"
const ConfigKeyDirectoryMaxRequests = "web.directory.max-requests"
const ConfigKeyFileMaxRequests = "web.file.max-requests"
const ConfigKeyPasswordMaxRequests = "password.max-requests"
const ConfigKeyTLSVerify = "web.tls.verify"
const DefaultDirectoryMaxRequests = 1000000
const DefaultFileMaxRequests = 200000
const DefaultPasswordMaxRequests = 10000

type ConfigEntry struct {
	Key          string
	Value        string
	DefaultValue string
}

func configPath() string {
	return filepath.Join(dataRoot(), "config.toml")
}

func LoadConfig() (*Config, error) {
	path := configPath()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{DirectoryMaxRequests: DefaultDirectoryMaxRequests, FileMaxRequests: DefaultFileMaxRequests, PasswordMaxRequests: DefaultPasswordMaxRequests, TLSVerify: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	config := Config{DirectoryMaxRequests: DefaultDirectoryMaxRequests, FileMaxRequests: DefaultFileMaxRequests, PasswordMaxRequests: DefaultPasswordMaxRequests, TLSVerify: true}
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
		if section != "project" && section != "web.directory" && section != "web.file" && section != "password" && section != "web.tls" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		parsed, err := strconv.Unquote(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("failed to parse configuration value in %s: %w", path, err)
		}
		switch {
		case section == "project" && strings.TrimSpace(key) == "root":
			config.ProjectRoot = parsed
		case section == "web.directory" && strings.TrimSpace(key) == "max-requests":
			limit, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || limit < 1 {
				return nil, fmt.Errorf("invalid directory request limit in %s", path)
			}
			config.DirectoryMaxRequests = limit
		case section == "web.file" && strings.TrimSpace(key) == "max-requests":
			limit, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || limit < 1 {
				return nil, fmt.Errorf("invalid file request limit in %s", path)
			}
			config.FileMaxRequests = limit
		case section == "password" && strings.TrimSpace(key) == "max-requests":
			limit, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || limit < 1 {
				return nil, fmt.Errorf("invalid password request limit in %s", path)
			}
			config.PasswordMaxRequests = limit
		case section == "web.tls" && strings.TrimSpace(key) == "verify":
			verify, parseErr := strconv.ParseBool(parsed)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid TLS verification setting in %s", path)
			}
			config.TLSVerify = verify
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
	if config.DirectoryMaxRequests > 0 && config.DirectoryMaxRequests != DefaultDirectoryMaxRequests {
		content += "\n[web.directory]\nmax-requests = " + strconv.Quote(strconv.Itoa(config.DirectoryMaxRequests)) + "\n"
	}
	if config.FileMaxRequests > 0 && config.FileMaxRequests != DefaultFileMaxRequests {
		content += "\n[web.file]\nmax-requests = " + strconv.Quote(strconv.Itoa(config.FileMaxRequests)) + "\n"
	}
	if config.PasswordMaxRequests > 0 && config.PasswordMaxRequests != DefaultPasswordMaxRequests {
		content += "\n[password]\nmax-requests = " + strconv.Quote(strconv.Itoa(config.PasswordMaxRequests)) + "\n"
	}
	if !config.TLSVerify {
		content += "\n[web.tls]\nverify = \"false\"\n"
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
	case ConfigKeyDirectoryMaxRequests:
		return strconv.Itoa(config.DirectoryMaxRequests), nil
	case ConfigKeyFileMaxRequests:
		return strconv.Itoa(config.FileMaxRequests), nil
	case ConfigKeyPasswordMaxRequests:
		return strconv.Itoa(config.PasswordMaxRequests), nil
	case ConfigKeyTLSVerify:
		return strconv.FormatBool(config.TLSVerify), nil
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
	case ConfigKeyDirectoryMaxRequests, ConfigKeyFileMaxRequests, ConfigKeyPasswordMaxRequests:
		limit, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || limit < 1 {
			return "", fmt.Errorf("request limit must be a positive integer")
		}
		if key == ConfigKeyDirectoryMaxRequests {
			config.DirectoryMaxRequests = limit
		} else if key == ConfigKeyFileMaxRequests {
			config.FileMaxRequests = limit
		} else {
			config.PasswordMaxRequests = limit
		}
		if err := SaveConfig(config); err != nil {
			return "", err
		}
		return strconv.Itoa(limit), nil
	case ConfigKeyTLSVerify:
		verify, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return "", fmt.Errorf("TLS verification must be true or false")
		}
		config.TLSVerify = verify
		if err := SaveConfig(config); err != nil {
			return "", err
		}
		return strconv.FormatBool(verify), nil
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
		{Key: ConfigKeyProjectRoot, Value: config.ProjectRoot, DefaultValue: "-"},
		{Key: ConfigKeyDirectoryMaxRequests, Value: strconv.Itoa(config.DirectoryMaxRequests), DefaultValue: strconv.Itoa(DefaultDirectoryMaxRequests)},
		{Key: ConfigKeyFileMaxRequests, Value: strconv.Itoa(config.FileMaxRequests), DefaultValue: strconv.Itoa(DefaultFileMaxRequests)},
		{Key: ConfigKeyPasswordMaxRequests, Value: strconv.Itoa(config.PasswordMaxRequests), DefaultValue: strconv.Itoa(DefaultPasswordMaxRequests)},
		{Key: ConfigKeyTLSVerify, Value: strconv.FormatBool(config.TLSVerify), DefaultValue: "true"},
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
