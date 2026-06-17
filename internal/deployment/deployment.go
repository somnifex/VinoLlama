package deployment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/llamacpp"
)

const OpenVINODocsURL = "https://github.com/ggml-org/llama.cpp/blob/master/docs/backend/OPENVINO.md"

type Report struct {
	Platform        string            `json:"platform"`
	OpenVINO        RuntimeStatus     `json:"openvino"`
	Tools           []ToolStatus      `json:"tools"`
	Binaries        []BinaryCandidate `json:"binaries"`
	Recommendations []string          `json:"recommendations"`
	BuildPlans      []BuildPlan       `json:"build_plans"`
	Reference       string            `json:"reference"`
}

type RuntimeStatus struct {
	Found       bool   `json:"found"`
	Source      string `json:"source"`
	Path        string `json:"path,omitempty"`
	SetupScript string `json:"setup_script,omitempty"`
	Fix         string `json:"fix,omitempty"`
}

type ToolStatus struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
	Path  string `json:"path,omitempty"`
	Fix   string `json:"fix,omitempty"`
}

type BinaryCandidate struct {
	Kind                 string `json:"kind"`
	Path                 string `json:"path"`
	Source               string `json:"source"`
	Usable               bool   `json:"usable"`
	Version              string `json:"version,omitempty"`
	OpenVINOCapable      bool   `json:"openvino_capable,omitempty"`
	CapabilityConfidence string `json:"capability_confidence,omitempty"`
	Reason               string `json:"reason,omitempty"`
}

type BuildPlan struct {
	Name        string      `json:"name"`
	Backend     string      `json:"backend"`
	Description string      `json:"description"`
	Steps       []BuildStep `json:"steps"`
}

type BuildStep struct {
	Shell   string `json:"shell"`
	Command string `json:"command"`
	Note    string `json:"note,omitempty"`
}

func Inspect(ctx context.Context, cfg config.Config) Report {
	report := Report{
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		OpenVINO:  detectOpenVINO(),
		Tools:     detectTools(),
		Reference: OpenVINODocsURL,
	}
	report.Binaries = discoverBinaries(ctx, cfg)
	report.BuildPlans = buildPlans(report.OpenVINO)
	report.Recommendations = recommendations(report)
	return report
}

func SelectBinary(ctx context.Context, cfg config.Config, kind, path string) (config.Config, BinaryCandidate, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	path = strings.TrimSpace(path)
	if kind != string(llamacpp.BinaryKindOpenVINO) && kind != string(llamacpp.BinaryKindCPU) {
		return cfg, BinaryCandidate{}, fmt.Errorf("kind must be one of openvino, cpu")
	}
	if path == "" {
		return cfg, BinaryCandidate{}, fmt.Errorf("path must not be empty")
	}
	candidate := inspectBinary(ctx, kind, path, "selected")
	if !candidate.Usable {
		return cfg, candidate, errors.New(candidate.Reason)
	}
	if kind == string(llamacpp.BinaryKindOpenVINO) && !candidate.OpenVINOCapable {
		return cfg, candidate, errors.New("selected binary is executable, but OpenVINO capability could not be confirmed")
	}
	next := cfg
	if kind == string(llamacpp.BinaryKindOpenVINO) {
		next.Runtime.LlamaOpenVINOBin = candidate.Path
	} else {
		next.Runtime.LlamaCPUBin = candidate.Path
	}
	return next, candidate, nil
}

func detectOpenVINO() RuntimeStatus {
	for _, key := range []string{"OPENVINO_RUNTIME", "INTEL_OPENVINO_DIR", "OPENVINO_DIR"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return RuntimeStatus{Found: true, Source: key, Path: value}
		}
	}
	for _, candidate := range openVINOSetupCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return RuntimeStatus{Found: true, Source: "setupvars", SetupScript: candidate}
		}
	}
	return RuntimeStatus{
		Found: false,
		Fix:   "Install OpenVINO Runtime, then run setupvars before building llama.cpp with -DGGML_OPENVINO=ON.",
	}
}

func detectTools() []ToolStatus {
	tools := []struct {
		name string
		fix  string
	}{
		{name: "git", fix: "Install Git and put git on PATH."},
		{name: "cmake", fix: "Install CMake and put cmake on PATH."},
		{name: "ninja", fix: "Install Ninja and put ninja on PATH."},
	}
	if runtime.GOOS == "windows" {
		tools = append(tools, struct {
			name string
			fix  string
		}{name: "cl", fix: "Run from a VS 2022 x64 Native Tools environment, or install Visual Studio Build Tools with Desktop development with C++."})
	}
	out := make([]ToolStatus, 0, len(tools))
	for _, tool := range tools {
		path, err := exec.LookPath(tool.name)
		if err != nil {
			out = append(out, ToolStatus{Name: tool.name, Found: false, Fix: tool.fix})
			continue
		}
		out = append(out, ToolStatus{Name: tool.name, Found: true, Path: path})
	}
	return out
}

func discoverBinaries(ctx context.Context, cfg config.Config) []BinaryCandidate {
	type pathSource struct {
		kind   string
		path   string
		source string
	}
	seen := map[string]bool{}
	var candidates []pathSource
	add := func(kind, path, source string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		key := kind + "\x00" + strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, pathSource{kind: kind, path: path, source: source})
	}
	add("openvino", os.Getenv("VINOLLAMA_LLAMA_OPENVINO_BIN"), "env")
	add("cpu", os.Getenv("VINOLLAMA_LLAMA_CPU_BIN"), "env")
	add("openvino", cfg.Runtime.LlamaOpenVINOBin, "config")
	add("cpu", cfg.Runtime.LlamaCPUBin, "config")
	for _, path := range binarySearchCandidates() {
		kind := "cpu"
		lower := strings.ToLower(path)
		if strings.Contains(lower, "openvino") || strings.Contains(lower, "releaseov") {
			kind = "openvino"
		}
		add(kind, path, "discovered")
	}
	for _, name := range pathCandidateNames() {
		if found, err := exec.LookPath(name); err == nil {
			add("cpu", found, "path")
		}
	}
	out := make([]BinaryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, inspectBinary(ctx, candidate.kind, candidate.path, candidate.source))
	}
	return out
}

func inspectBinary(ctx context.Context, kind, path, source string) BinaryCandidate {
	candidate := BinaryCandidate{Kind: kind, Path: path, Source: source}
	abs, err := filepath.Abs(path)
	if err == nil {
		candidate.Path = abs
	}
	info, err := os.Stat(candidate.Path)
	if err != nil {
		candidate.Reason = err.Error()
		return candidate
	}
	if info.IsDir() {
		candidate.Reason = "path is a directory"
		return candidate
	}
	if !isExecutablePath(candidate.Path, info) {
		candidate.Reason = "path is not executable"
		return candidate
	}
	help, err := runHelp(ctx, candidate.Path)
	if err != nil {
		candidate.Reason = err.Error()
		return candidate
	}
	candidate.Usable = true
	candidate.Version = parseVersion(help)
	if kind == "openvino" {
		helpText := strings.ToLower(help)
		base := strings.ToLower(filepath.Base(candidate.Path))
		parent := strings.ToLower(filepath.Base(filepath.Dir(candidate.Path)))
		switch {
		case strings.Contains(helpText, "openvino") || strings.Contains(base, "openvino") || strings.Contains(parent, "openvino") || parent == "releaseov":
			candidate.OpenVINOCapable = true
			candidate.CapabilityConfidence = "confirmed"
		default:
			candidate.CapabilityConfidence = "unknown"
			candidate.Reason = "binary is usable, but --help/path did not identify OpenVINO support"
		}
	}
	return candidate
}

func runHelp(ctx context.Context, path string) (string, error) {
	helpCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(helpCtx, path, "--help")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	text := output.String()
	if helpCtx.Err() != nil {
		return text, helpCtx.Err()
	}
	if err != nil {
		return text, fmt.Errorf("%s --help failed: %w", path, err)
	}
	return text, nil
}

func buildPlans(openvino RuntimeStatus) []BuildPlan {
	openvinoSetup := openvino.SetupScript
	if openvinoSetup == "" && openvino.Path != "" {
		openvinoSetup = openvino.Path
	}
	if openvinoSetup == "" {
		openvinoSetup = "<OpenVINO setupvars path>"
	}
	if runtime.GOOS == "windows" {
		return []BuildPlan{
			{
				Name:        "Build llama.cpp OpenVINO server on Windows",
				Backend:     "openvino",
				Description: "Use a VS 2022 x64 Native Tools shell and build llama-server with GGML_OPENVINO=ON.",
				Steps: []BuildStep{
					{Shell: "cmd", Command: fmt.Sprintf("%q", openvinoSetup), Note: "Initialize OpenVINO Runtime first."},
					{Shell: "cmd", Command: "git clone https://github.com/ggml-org/llama.cpp"},
					{Shell: "cmd", Command: "cd llama.cpp"},
					{Shell: "cmd", Command: "cmake -B build\\ReleaseOV -G Ninja -DCMAKE_BUILD_TYPE=Release -DGGML_OPENVINO=ON -DLLAMA_CURL=OFF -DCMAKE_TOOLCHAIN_FILE=C:\\vcpkg\\scripts\\buildsystems\\vcpkg.cmake"},
					{Shell: "cmd", Command: "cmake --build build\\ReleaseOV --parallel"},
					{Shell: "cmd", Command: "build\\ReleaseOV\\bin\\llama-server.exe --help"},
				},
			},
			{
				Name:        "Build llama.cpp CPU server on Windows",
				Backend:     "cpu",
				Description: "Build a plain CPU llama-server fallback binary.",
				Steps: []BuildStep{
					{Shell: "cmd", Command: "git clone https://github.com/ggml-org/llama.cpp"},
					{Shell: "cmd", Command: "cd llama.cpp"},
					{Shell: "cmd", Command: "cmake -B build\\Release -G Ninja -DCMAKE_BUILD_TYPE=Release -DLLAMA_CURL=OFF"},
					{Shell: "cmd", Command: "cmake --build build\\Release --parallel"},
					{Shell: "cmd", Command: "build\\Release\\bin\\llama-server.exe --help"},
				},
			},
		}
	}
	return []BuildPlan{
		{
			Name:        "Build llama.cpp OpenVINO server on Linux",
			Backend:     "openvino",
			Description: "Build llama-server with GGML_OPENVINO=ON after initializing OpenVINO Runtime.",
			Steps: []BuildStep{
				{Shell: "bash", Command: "source /opt/intel/openvino/setupvars.sh"},
				{Shell: "bash", Command: "git clone https://github.com/ggml-org/llama.cpp"},
				{Shell: "bash", Command: "cd llama.cpp"},
				{Shell: "bash", Command: "cmake -B build/ReleaseOV -G Ninja -DCMAKE_BUILD_TYPE=Release -DGGML_OPENVINO=ON"},
				{Shell: "bash", Command: "cmake --build build/ReleaseOV --parallel"},
				{Shell: "bash", Command: "./build/ReleaseOV/bin/llama-server --help"},
			},
		},
		{
			Name:        "Build llama.cpp CPU server on Linux",
			Backend:     "cpu",
			Description: "Build a plain CPU llama-server fallback binary.",
			Steps: []BuildStep{
				{Shell: "bash", Command: "git clone https://github.com/ggml-org/llama.cpp"},
				{Shell: "bash", Command: "cd llama.cpp"},
				{Shell: "bash", Command: "cmake -B build/Release -G Ninja -DCMAKE_BUILD_TYPE=Release"},
				{Shell: "bash", Command: "cmake --build build/Release --parallel"},
				{Shell: "bash", Command: "./build/Release/bin/llama-server --help"},
			},
		},
	}
}

func recommendations(report Report) []string {
	var out []string
	if !report.OpenVINO.Found {
		out = append(out, "Install OpenVINO Runtime before building or running the OpenVINO backend.")
	}
	for _, tool := range report.Tools {
		if !tool.Found {
			out = append(out, "Install "+tool.Name+" for local llama.cpp builds.")
		}
	}
	openvinoUsable := false
	cpuUsable := false
	for _, binary := range report.Binaries {
		if binary.Kind == "openvino" && binary.Usable && binary.OpenVINOCapable {
			openvinoUsable = true
		}
		if binary.Kind == "cpu" && binary.Usable {
			cpuUsable = true
		}
	}
	if !openvinoUsable {
		out = append(out, "Build or select an OpenVINO-enabled llama-server binary.")
	}
	if !cpuUsable {
		out = append(out, "Build or select a CPU llama-server fallback binary.")
	}
	if len(out) == 0 {
		out = append(out, "Deployment prerequisites look ready.")
	}
	return out
}

func openVINOSetupCandidates() []string {
	if runtime.GOOS == "windows" {
		var out []string
		for _, pattern := range []string{
			`C:\Program Files (x86)\Intel\openvino*\setupvars.bat`,
			`C:\Program Files\Intel\openvino*\setupvars.bat`,
		} {
			matches, _ := filepath.Glob(pattern)
			out = append(out, matches...)
		}
		return out
	}
	var out []string
	for _, pattern := range []string{"/opt/intel/openvino/setupvars.sh", "/opt/intel/openvino*/setupvars.sh"} {
		matches, _ := filepath.Glob(pattern)
		out = append(out, matches...)
	}
	return out
}

func binarySearchCandidates() []string {
	home, _ := os.UserHomeDir()
	root, _ := config.DefaultRootDir()
	exe := "llama-server"
	if runtime.GOOS == "windows" {
		exe = "llama-server.exe"
	}
	var bases []string
	if home != "" {
		bases = append(bases, filepath.Join(home, "llama.cpp"))
	}
	if root != "" {
		bases = append(bases, filepath.Join(root, "llama.cpp"))
		bases = append(bases, filepath.Join(root, "src", "llama.cpp"))
	}
	var out []string
	for _, base := range bases {
		out = append(out,
			filepath.Join(base, "build", "ReleaseOV", "bin", exe),
			filepath.Join(base, "build", "Release", "bin", exe),
			filepath.Join(base, "build", "bin", exe),
		)
	}
	return out
}

func pathCandidateNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"llama-server.exe", "llama-server.cmd", "llama-server.bat", "llama-server"}
	}
	return []string{"llama-server", "llama.cpp-server"}
}

func isExecutablePath(path string, info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".bat", ".cmd", ".com":
			return true
		default:
			return false
		}
	}
	return info.Mode()&0o111 != 0
}

func parseVersion(help string) string {
	for _, line := range strings.Split(help, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "version") && !strings.Contains(lower, "llama.cpp") {
			continue
		}
		fields := strings.Fields(line)
		for _, field := range fields {
			trimmed := strings.Trim(field, " ,;:")
			if strings.ContainsAny(trimmed, "0123456789") {
				return trimmed
			}
		}
	}
	return "unknown"
}
