package ctx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	ProjectRoot                string
	ActiveProjectRoot          string
	ProjectRoots               map[string]string
	DirectoryMaxRequests       int
	FileMaxRequests            int
	VHostMaxRequests           int
	VHostCalibrationSamples    int
	VHostCalibrationConfidence int
	PasswordMaxRequests        int
	DNSMaxQueries              int
	TLSVerify                  bool
}

const ConfigKeyProjectRoot = "project.root"
const ConfigKeyDirectoryMaxRequests = "web.directory.max-requests"
const ConfigKeyFileMaxRequests = "web.file.max-requests"
const ConfigKeyVHostMaxRequests = "web.vhost.max-requests"
const ConfigKeyVHostCalibrationSamples = "web.vhost.calibration-samples"
const ConfigKeyVHostCalibrationConfidence = "web.vhost.calibration-confidence"
const ConfigKeyPasswordMaxRequests = "password.max-requests"
const ConfigKeyDNSMaxQueries = "dns.max-queries"
const ConfigKeyTLSVerify = "web.tls.verify"
const DefaultDirectoryMaxRequests = 1000000
const DefaultFileMaxRequests = 200000
const DefaultVHostMaxRequests = 10000
const DefaultVHostCalibrationSamples = 10
const DefaultVHostCalibrationConfidence = 90
const DefaultPasswordMaxRequests = 10000
const DefaultDNSMaxQueries = 10000

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
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	config := *defaultConfig()
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
		if section != "project" && section != "project.roots" && section != "web.directory" && section != "web.file" && section != "web.vhost" && section != "password" && section != "dns" && section != "web.tls" {
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
		case section == "project" && strings.TrimSpace(key) == "active-root":
			config.ActiveProjectRoot = parsed
		case section == "project.roots":
			name := strings.TrimSpace(key)
			if err := validateProjectRootName(name); err != nil {
				return nil, fmt.Errorf("invalid project root in %s: %w", path, err)
			}
			config.ProjectRoots[name] = parsed
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
		case section == "web.vhost" && strings.TrimSpace(key) == "max-requests":
			limit, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || limit < 1 {
				return nil, fmt.Errorf("invalid vhost request limit in %s", path)
			}
			config.VHostMaxRequests = limit
		case section == "web.vhost" && strings.TrimSpace(key) == "calibration-samples":
			limit, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || limit < 1 || limit > 100 {
				return nil, fmt.Errorf("invalid vhost calibration sample count in %s", path)
			}
			config.VHostCalibrationSamples = limit
		case section == "web.vhost" && strings.TrimSpace(key) == "calibration-confidence":
			confidence, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || confidence < 50 || confidence > 100 {
				return nil, fmt.Errorf("invalid vhost calibration confidence in %s", path)
			}
			config.VHostCalibrationConfidence = confidence
		case section == "password" && strings.TrimSpace(key) == "max-requests":
			limit, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || limit < 1 {
				return nil, fmt.Errorf("invalid password request limit in %s", path)
			}
			config.PasswordMaxRequests = limit
		case section == "dns" && strings.TrimSpace(key) == "max-queries":
			limit, parseErr := strconv.Atoi(parsed)
			if parseErr != nil || limit < 1 {
				return nil, fmt.Errorf("invalid DNS query limit in %s", path)
			}
			config.DNSMaxQueries = limit
		case section == "web.tls" && strings.TrimSpace(key) == "verify":
			verify, parseErr := strconv.ParseBool(parsed)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid TLS verification setting in %s", path)
			}
			config.TLSVerify = verify
		}
	}
	if err := normalizeProjectRoots(&config); err != nil {
		return nil, fmt.Errorf("invalid project root configuration in %s: %w", path, err)
	}

	return &config, nil
}

func defaultConfig() *Config {
	return &Config{
		ProjectRoots:               make(map[string]string),
		DirectoryMaxRequests:       DefaultDirectoryMaxRequests,
		FileMaxRequests:            DefaultFileMaxRequests,
		VHostMaxRequests:           DefaultVHostMaxRequests,
		VHostCalibrationSamples:    DefaultVHostCalibrationSamples,
		VHostCalibrationConfidence: DefaultVHostCalibrationConfidence,
		PasswordMaxRequests:        DefaultPasswordMaxRequests,
		DNSMaxQueries:              DefaultDNSMaxQueries,
		TLSVerify:                  true,
	}
}

func normalizeProjectRoots(config *Config) error {
	if config.ProjectRoots == nil {
		config.ProjectRoots = make(map[string]string)
	}
	if len(config.ProjectRoots) == 0 {
		if config.ProjectRoot == "" {
			config.ActiveProjectRoot = ""
			return nil
		}
		config.ActiveProjectRoot = "default"
		config.ProjectRoots[config.ActiveProjectRoot] = config.ProjectRoot
		return nil
	}
	if config.ActiveProjectRoot == "" {
		for name, root := range config.ProjectRoots {
			if root == config.ProjectRoot {
				config.ActiveProjectRoot = name
				break
			}
		}
	}
	if config.ActiveProjectRoot == "" {
		names := make([]string, 0, len(config.ProjectRoots))
		for name := range config.ProjectRoots {
			names = append(names, name)
		}
		sort.Strings(names)
		config.ActiveProjectRoot = names[0]
	}
	root, ok := config.ProjectRoots[config.ActiveProjectRoot]
	if !ok {
		return fmt.Errorf("active project root not found: %s", config.ActiveProjectRoot)
	}
	config.ProjectRoot = root
	return nil
}

func SaveConfig(config *Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := normalizeProjectRoots(config); err != nil {
		return err
	}
	content := "[project]\nroot = " + strconv.Quote(config.ProjectRoot) + "\n"
	content += "active-root = " + strconv.Quote(config.ActiveProjectRoot) + "\n"
	if len(config.ProjectRoots) > 0 {
		names := make([]string, 0, len(config.ProjectRoots))
		for name := range config.ProjectRoots {
			names = append(names, name)
		}
		sort.Strings(names)
		content += "\n[project.roots]\n"
		for _, name := range names {
			content += name + " = " + strconv.Quote(config.ProjectRoots[name]) + "\n"
		}
	}
	if config.DirectoryMaxRequests > 0 && config.DirectoryMaxRequests != DefaultDirectoryMaxRequests {
		content += "\n[web.directory]\nmax-requests = " + strconv.Quote(strconv.Itoa(config.DirectoryMaxRequests)) + "\n"
	}
	if config.FileMaxRequests > 0 && config.FileMaxRequests != DefaultFileMaxRequests {
		content += "\n[web.file]\nmax-requests = " + strconv.Quote(strconv.Itoa(config.FileMaxRequests)) + "\n"
	}
	vhostConfig := ""
	if config.VHostMaxRequests > 0 && config.VHostMaxRequests != DefaultVHostMaxRequests {
		vhostConfig += "max-requests = " + strconv.Quote(strconv.Itoa(config.VHostMaxRequests)) + "\n"
	}
	if config.VHostCalibrationSamples > 0 && config.VHostCalibrationSamples != DefaultVHostCalibrationSamples {
		vhostConfig += "calibration-samples = " + strconv.Quote(strconv.Itoa(config.VHostCalibrationSamples)) + "\n"
	}
	if config.VHostCalibrationConfidence > 0 && config.VHostCalibrationConfidence != DefaultVHostCalibrationConfidence {
		vhostConfig += "calibration-confidence = " + strconv.Quote(strconv.Itoa(config.VHostCalibrationConfidence)) + "\n"
	}
	if vhostConfig != "" {
		content += "\n[web.vhost]\n" + vhostConfig
	}
	if config.PasswordMaxRequests > 0 && config.PasswordMaxRequests != DefaultPasswordMaxRequests {
		content += "\n[password]\nmax-requests = " + strconv.Quote(strconv.Itoa(config.PasswordMaxRequests)) + "\n"
	}
	if config.DNSMaxQueries > 0 && config.DNSMaxQueries != DefaultDNSMaxQueries {
		content += "\n[dns]\nmax-queries = " + strconv.Quote(strconv.Itoa(config.DNSMaxQueries)) + "\n"
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
	case ConfigKeyVHostMaxRequests:
		return strconv.Itoa(config.VHostMaxRequests), nil
	case ConfigKeyVHostCalibrationSamples:
		return strconv.Itoa(config.VHostCalibrationSamples), nil
	case ConfigKeyVHostCalibrationConfidence:
		return strconv.Itoa(config.VHostCalibrationConfidence), nil
	case ConfigKeyPasswordMaxRequests:
		return strconv.Itoa(config.PasswordMaxRequests), nil
	case ConfigKeyDNSMaxQueries:
		return strconv.Itoa(config.DNSMaxQueries), nil
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
		name := config.ActiveProjectRoot
		if name == "" {
			name = "default"
		}
		config.ActiveProjectRoot = name
		config.ProjectRoot = root
		config.ProjectRoots[name] = root
		if err := SaveConfig(config); err != nil {
			return "", err
		}
		return root, nil
	case ConfigKeyDirectoryMaxRequests, ConfigKeyFileMaxRequests, ConfigKeyVHostMaxRequests, ConfigKeyVHostCalibrationSamples, ConfigKeyVHostCalibrationConfidence, ConfigKeyPasswordMaxRequests, ConfigKeyDNSMaxQueries:
		limit, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || limit < 1 {
			return "", fmt.Errorf("request limit must be a positive integer")
		}
		if key == ConfigKeyVHostCalibrationConfidence {
			if limit < 50 || limit > 100 {
				return "", fmt.Errorf("vhost calibration confidence must be between 50 and 100")
			}
			config.VHostCalibrationConfidence = limit
		} else if key == ConfigKeyVHostCalibrationSamples {
			if limit > 100 {
				return "", fmt.Errorf("vhost calibration samples must be between 1 and 100")
			}
			config.VHostCalibrationSamples = limit
		} else if key == ConfigKeyDirectoryMaxRequests {
			config.DirectoryMaxRequests = limit
		} else if key == ConfigKeyFileMaxRequests {
			config.FileMaxRequests = limit
		} else if key == ConfigKeyPasswordMaxRequests {
			config.PasswordMaxRequests = limit
		} else {
			config.DNSMaxQueries = limit
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
		{Key: ConfigKeyVHostMaxRequests, Value: strconv.Itoa(config.VHostMaxRequests), DefaultValue: strconv.Itoa(DefaultVHostMaxRequests)},
		{Key: ConfigKeyVHostCalibrationSamples, Value: strconv.Itoa(config.VHostCalibrationSamples), DefaultValue: strconv.Itoa(DefaultVHostCalibrationSamples)},
		{Key: ConfigKeyVHostCalibrationConfidence, Value: strconv.Itoa(config.VHostCalibrationConfidence), DefaultValue: strconv.Itoa(DefaultVHostCalibrationConfidence)},
		{Key: ConfigKeyPasswordMaxRequests, Value: strconv.Itoa(config.PasswordMaxRequests), DefaultValue: strconv.Itoa(DefaultPasswordMaxRequests)},
		{Key: ConfigKeyDNSMaxQueries, Value: strconv.Itoa(config.DNSMaxQueries), DefaultValue: strconv.Itoa(DefaultDNSMaxQueries)},
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
