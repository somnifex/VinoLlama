package llamacpp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"vinollama/internal/config"
)

type BinaryKind string

const (
	BinaryKindOpenVINO BinaryKind = "openvino"
	BinaryKindCPU      BinaryKind = "cpu"
)

type ResolvedBinary struct {
	Kind       BinaryKind
	Path       string
	Source     string
	Version    string
	HelpOutput string
}

type BinaryResolver interface {
	Resolve(ctx context.Context, kind BinaryKind, cfg config.Config) (ResolvedBinary, error)
}

type DefaultBinaryResolver struct {
	cliPaths    map[BinaryKind]string
	bundledRoot string
	lookPath    func(string) (string, error)
	stat        func(string) (os.FileInfo, error)
	runHelp     func(context.Context, string) (string, error)
}

type BinaryResolverOption func(*DefaultBinaryResolver)

func NewBinaryResolver(options ...BinaryResolverOption) *DefaultBinaryResolver {
	root, _ := config.DefaultRootDir()
	r := &DefaultBinaryResolver{
		cliPaths:    map[BinaryKind]string{},
		bundledRoot: filepath.Join(root, "bin"),
		lookPath:    exec.LookPath,
		stat:        os.Stat,
		runHelp:     runBinaryHelp,
	}
	for _, option := range options {
		option(r)
	}
	return r
}

func WithCLIBinary(kind BinaryKind, path string) BinaryResolverOption {
	return func(r *DefaultBinaryResolver) {
		r.cliPaths[kind] = path
	}
}

func WithBundledRoot(path string) BinaryResolverOption {
	return func(r *DefaultBinaryResolver) {
		r.bundledRoot = path
	}
}

func WithLookPath(fn func(string) (string, error)) BinaryResolverOption {
	return func(r *DefaultBinaryResolver) {
		r.lookPath = fn
	}
}

func WithHelpRunner(fn func(context.Context, string) (string, error)) BinaryResolverOption {
	return func(r *DefaultBinaryResolver) {
		r.runHelp = fn
	}
}

func (r *DefaultBinaryResolver) Resolve(ctx context.Context, kind BinaryKind, cfg config.Config) (ResolvedBinary, error) {
	if kind != BinaryKindOpenVINO && kind != BinaryKindCPU {
		return ResolvedBinary{}, ActionableError{
			What:    "llama.cpp binary could not be resolved.",
			Reason:  fmt.Sprintf("unsupported binary kind %q", kind),
			Fix:     "Use one of: openvino, cpu.",
			Details: fmt.Sprintf("kind=%s", kind),
		}
	}

	candidates := []struct {
		path   string
		source string
	}{}
	if path := strings.TrimSpace(r.cliPaths[kind]); path != "" {
		candidates = append(candidates, struct {
			path   string
			source string
		}{path: path, source: "cli"})
	}
	if path := strings.TrimSpace(os.Getenv(envName(kind))); path != "" {
		candidates = append(candidates, struct {
			path   string
			source string
		}{path: path, source: "env"})
	}
	if path := strings.TrimSpace(configPathForKind(kind, cfg)); path != "" {
		candidates = append(candidates, struct {
			path   string
			source string
		}{path: path, source: "config"})
	}
	for _, path := range r.bundledCandidates(kind) {
		candidates = append(candidates, struct {
			path   string
			source string
		}{path: path, source: "bundled"})
	}
	for _, name := range pathCandidateNames() {
		if found, err := r.lookPath(name); err == nil && found != "" {
			candidates = append(candidates, struct {
				path   string
				source string
			}{path: found, source: "path"})
		}
	}

	var reasons []string
	for _, candidate := range candidates {
		resolved, err := r.inspect(ctx, kind, candidate.path, candidate.source)
		if err == nil {
			return resolved, nil
		}
		reasons = append(reasons, fmt.Sprintf("%s=%s: %v", candidate.source, candidate.path, err))
		if candidate.source == "cli" || candidate.source == "env" || candidate.source == "config" {
			break
		}
	}

	reason := "No configured or discoverable llama.cpp binary was found."
	if len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}
	return ResolvedBinary{}, ActionableError{
		What:    fmt.Sprintf("llama.cpp %s binary could not be resolved.", kind),
		Reason:  reason,
		Fix:     fixForKind(kind),
		Details: fmt.Sprintf("kind=%s", kind),
	}
}

func (r *DefaultBinaryResolver) inspect(ctx context.Context, kind BinaryKind, path, source string) (ResolvedBinary, error) {
	if path == "" {
		return ResolvedBinary{}, errors.New("path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	info, err := r.stat(path)
	if err != nil {
		return ResolvedBinary{}, fmt.Errorf("inspect binary: %w", err)
	}
	if info.IsDir() {
		return ResolvedBinary{}, fmt.Errorf("%s is a directory", path)
	}
	if !isExecutablePath(path, info) {
		return ResolvedBinary{}, fmt.Errorf("%s is not executable", path)
	}
	help, err := r.runHelp(ctx, path)
	if err != nil {
		return ResolvedBinary{}, err
	}
	return ResolvedBinary{
		Kind:       kind,
		Path:       path,
		Source:     source,
		Version:    parseVersion(help),
		HelpOutput: help,
	}, nil
}

func (r *DefaultBinaryResolver) bundledCandidates(kind BinaryKind) []string {
	if r.bundledRoot == "" {
		return nil
	}
	dirs := []string{r.bundledRoot}
	switch kind {
	case BinaryKindOpenVINO:
		dirs = append([]string{filepath.Join(r.bundledRoot, "llama-openvino"), filepath.Join(r.bundledRoot, "openvino")}, dirs...)
	case BinaryKindCPU:
		dirs = append([]string{filepath.Join(r.bundledRoot, "llama-cpu"), filepath.Join(r.bundledRoot, "cpu")}, dirs...)
	}
	var candidates []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			info, err := entry.Info()
			if err == nil && isExecutablePath(path, info) {
				candidates = append(candidates, path)
			}
		}
	}
	return candidates
}

func runBinaryHelp(ctx context.Context, path string) (string, error) {
	helpCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(helpCtx, path, "--help")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	text := output.String()
	if helpCtx.Err() != nil {
		return text, ActionableError{
			What:    "llama.cpp binary help check timed out.",
			Reason:  helpCtx.Err().Error(),
			Fix:     "Run the binary manually with --help and confirm it exits quickly.",
			Details: fmt.Sprintf("command=%s --help", path),
		}
	}
	if err != nil {
		return text, ActionableError{
			What:    "llama.cpp binary help check failed.",
			Reason:  err.Error(),
			Fix:     "Confirm the path points to a llama.cpp server binary for this operating system.",
			Details: fmt.Sprintf("command=%s --help stderr_tail=%q", path, stderrTail(text)),
		}
	}
	return text, nil
}

func envName(kind BinaryKind) string {
	switch kind {
	case BinaryKindOpenVINO:
		return "VINOLLAMA_LLAMA_OPENVINO_BIN"
	case BinaryKindCPU:
		return "VINOLLAMA_LLAMA_CPU_BIN"
	default:
		return ""
	}
}

func configPathForKind(kind BinaryKind, cfg config.Config) string {
	switch kind {
	case BinaryKindOpenVINO:
		return cfg.Runtime.LlamaOpenVINOBin
	case BinaryKindCPU:
		return cfg.Runtime.LlamaCPUBin
	default:
		return ""
	}
}

func fixForKind(kind BinaryKind) string {
	switch kind {
	case BinaryKindOpenVINO:
		return "Set VINOLLAMA_LLAMA_OPENVINO_BIN or runtime.llama_openvino_bin, or install a llama.cpp OpenVINO server binary on PATH."
	case BinaryKindCPU:
		return "Set VINOLLAMA_LLAMA_CPU_BIN or runtime.llama_cpu_bin, or install a llama.cpp CPU server binary on PATH."
	default:
		return "Configure a llama.cpp server binary."
	}
}

func pathCandidateNames() []string {
	names := []string{"llama-server", "llama.cpp-server"}
	if runtime.GOOS == "windows" {
		return []string{"llama-server.exe", "llama-server.cmd", "llama-server.bat", "llama-server", "llama.cpp-server.exe"}
	}
	return names
}

func isExecutablePath(path string, info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".com"
	}
	return info.Mode()&0o111 != 0
}

func parseVersion(help string) string {
	re := regexp.MustCompile(`(?i)(?:version|llama\.cpp)[^\n\r0-9]*([0-9]+(?:\.[0-9]+)*(?:[-+._a-z0-9]*)?)`)
	if match := re.FindStringSubmatch(help); len(match) == 2 {
		return match[1]
	}
	return "unknown"
}
