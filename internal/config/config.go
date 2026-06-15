package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 11435
)

type Config struct {
	Server     ServerConfig
	Runtime    RuntimeConfig
	Generation GenerationConfig
	Models     ModelsConfig
	Desktop    DesktopConfig
	Privacy    PrivacyConfig
	Logging    LoggingConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type RuntimeConfig struct {
	Backend              string
	IdleTimeout          time.Duration
	ReadyTimeout         time.Duration
	LlamaOpenVINOBin     string
	LlamaCPUBin          string
	InternalPortStart    int
	HealthPath           string
	ExtraOpenVINOArgs    []string
	ExtraCPUArgs         []string
	AllowUnverifiedFlags bool
}

type GenerationConfig struct {
	CtxSize     int
	Temperature float64
	TopP        float64
	Threads     int
}

type ModelsConfig struct {
	Directory         string
	DefaultImportMode string
}

type DesktopConfig struct {
	StartServiceOnLaunch bool
	StopServiceOnExit    bool
	Theme                string
	CompactMode          bool
}

type PrivacyConfig struct {
	Telemetry bool
}

type LoggingConfig struct {
	Level string
	File  string
}

type LoadResult struct {
	Config Config
	Path   string
	Found  bool
}

func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Host: DefaultHost,
			Port: DefaultPort,
		},
		Runtime: RuntimeConfig{
			Backend:           "auto",
			IdleTimeout:       10 * time.Minute,
			ReadyTimeout:      30 * time.Second,
			InternalPortStart: 21435,
		},
		Generation: GenerationConfig{
			CtxSize:     4096,
			Temperature: 0.7,
			TopP:        0.9,
		},
		Models: ModelsConfig{
			DefaultImportMode: "reference",
		},
		Desktop: DesktopConfig{
			StartServiceOnLaunch: true,
			Theme:                "system",
		},
		Privacy: PrivacyConfig{
			Telemetry: false,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}

func Load(path string) (LoadResult, error) {
	cfg := Defaults()
	explicitPath := path != ""
	if path == "" {
		defaultPath, err := DefaultConfigPath()
		if err != nil {
			return LoadResult{}, err
		}
		path = defaultPath
	}

	found := false
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || explicitPath {
			return LoadResult{}, err
		}
	} else {
		found = true
		if err := applyYAML(data, &cfg); err != nil {
			return LoadResult{}, fmt.Errorf("%s: %w", path, err)
		}
	}

	applyEnv(&cfg)
	if err := validate(cfg); err != nil {
		return LoadResult{}, err
	}
	return LoadResult{Config: cfg, Path: path, Found: found}, nil
}

func DefaultConfigPath() (string, error) {
	root, err := DefaultRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.yaml"), nil
}

func DefaultRootDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home directory is unavailable: %w", err)
	}
	return filepath.Join(home, ".vinollama"), nil
}

func ModelsDirectory(cfg Config) (string, error) {
	if cfg.Models.Directory != "" {
		return cfg.Models.Directory, nil
	}
	root, err := DefaultRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "models"), nil
}

func ValidBackend(value string) bool {
	switch value {
	case "auto", "openvino", "cpu":
		return true
	default:
		return false
	}
}

func applyEnv(cfg *Config) {
	if value := os.Getenv("VINOLLAMA_BACKEND"); value != "" {
		cfg.Runtime.Backend = value
	}
	if value := os.Getenv("VINOLLAMA_HOST"); value != "" {
		cfg.Server.Host = value
	}
	if value := os.Getenv("VINOLLAMA_PORT"); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.Server.Port = port
		}
	}
	if value := os.Getenv("VINOLLAMA_MODELS"); value != "" {
		cfg.Models.Directory = value
	}
	if value := os.Getenv("VINOLLAMA_LLAMA_OPENVINO_BIN"); value != "" {
		cfg.Runtime.LlamaOpenVINOBin = value
	}
	if value := os.Getenv("VINOLLAMA_LLAMA_CPU_BIN"); value != "" {
		cfg.Runtime.LlamaCPUBin = value
	}
	if value := os.Getenv("VINOLLAMA_LOG_LEVEL"); value != "" {
		cfg.Logging.Level = value
	}
}

func validate(cfg Config) error {
	if cfg.Server.Host == "" {
		return errors.New("server.host must not be empty")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", cfg.Server.Port)
	}
	if !ValidBackend(cfg.Runtime.Backend) {
		return fmt.Errorf("runtime.backend must be one of auto, openvino, cpu, got %q", cfg.Runtime.Backend)
	}
	if cfg.Models.DefaultImportMode != "reference" && cfg.Models.DefaultImportMode != "copy" && cfg.Models.DefaultImportMode != "link" {
		return fmt.Errorf("models.default_import_mode must be one of reference, copy, link, got %q", cfg.Models.DefaultImportMode)
	}
	return nil
}

func applyYAML(data []byte, cfg *Config) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	section := ""
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		trimmed := strings.TrimSpace(raw)
		if indent == 0 && strings.HasSuffix(trimmed, ":") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 || section == "" {
			return fmt.Errorf("invalid config line %d: %q", lineNo, raw)
		}
		key := strings.TrimSpace(parts[0])
		value := cleanScalar(parts[1])
		if err := setValue(cfg, section, key, value); err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func cleanScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}

func setValue(cfg *Config, section, key, value string) error {
	switch section {
	case "server":
		switch key {
		case "host":
			cfg.Server.Host = value
		case "port":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("server.port must be an integer: %w", err)
			}
			cfg.Server.Port = parsed
		}
	case "runtime":
		switch key {
		case "backend":
			cfg.Runtime.Backend = value
		case "idle_timeout":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("runtime.idle_timeout must be a duration like 10m: %w", err)
			}
			cfg.Runtime.IdleTimeout = parsed
		case "ready_timeout":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("runtime.ready_timeout must be a duration like 30s: %w", err)
			}
			cfg.Runtime.ReadyTimeout = parsed
		case "llama_openvino_bin":
			cfg.Runtime.LlamaOpenVINOBin = value
		case "llama_cpu_bin":
			cfg.Runtime.LlamaCPUBin = value
		case "internal_port_start":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("runtime.internal_port_start must be an integer: %w", err)
			}
			cfg.Runtime.InternalPortStart = parsed
		case "health_path":
			cfg.Runtime.HealthPath = value
		case "extra_openvino_args":
			parsed, err := parseStringList(value)
			if err != nil {
				return fmt.Errorf("runtime.extra_openvino_args must be an inline list: %w", err)
			}
			cfg.Runtime.ExtraOpenVINOArgs = parsed
		case "extra_cpu_args":
			parsed, err := parseStringList(value)
			if err != nil {
				return fmt.Errorf("runtime.extra_cpu_args must be an inline list: %w", err)
			}
			cfg.Runtime.ExtraCPUArgs = parsed
		case "allow_unverified_flags":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("runtime.allow_unverified_flags must be true or false: %w", err)
			}
			cfg.Runtime.AllowUnverifiedFlags = parsed
		}
	case "generation":
		switch key {
		case "ctx_size":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("generation.ctx_size must be an integer: %w", err)
			}
			cfg.Generation.CtxSize = parsed
		case "temperature":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("generation.temperature must be a number: %w", err)
			}
			cfg.Generation.Temperature = parsed
		case "top_p":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("generation.top_p must be a number: %w", err)
			}
			cfg.Generation.TopP = parsed
		case "threads":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("generation.threads must be an integer: %w", err)
			}
			cfg.Generation.Threads = parsed
		}
	case "models":
		switch key {
		case "directory":
			cfg.Models.Directory = value
		case "default_import_mode":
			cfg.Models.DefaultImportMode = value
		}
	case "desktop":
		switch key {
		case "start_service_on_launch":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("desktop.start_service_on_launch must be true or false: %w", err)
			}
			cfg.Desktop.StartServiceOnLaunch = parsed
		case "stop_service_on_exit":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("desktop.stop_service_on_exit must be true or false: %w", err)
			}
			cfg.Desktop.StopServiceOnExit = parsed
		case "theme":
			cfg.Desktop.Theme = value
		case "compact_mode":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("desktop.compact_mode must be true or false: %w", err)
			}
			cfg.Desktop.CompactMode = parsed
		}
	case "privacy":
		if key == "telemetry" {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("privacy.telemetry must be true or false: %w", err)
			}
			cfg.Privacy.Telemetry = parsed
		}
	case "logging":
		switch key {
		case "level":
			cfg.Logging.Level = value
		case "file":
			cfg.Logging.File = value
		}
	}
	return nil
}

func parseStringList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" {
		return nil, nil
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("expected [value, value]")
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return nil, nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		item = strings.Trim(item, `"`)
		item = strings.Trim(item, `'`)
		if item != "" {
			out = append(out, item)
		}
	}
	return out, nil
}
