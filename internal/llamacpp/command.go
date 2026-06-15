package llamacpp

import (
	"fmt"
	"strconv"
	"strings"
)

type ServerStartRequest struct {
	BinaryPath  string
	ModelPath   string
	Host        string
	Port        int
	Backend     string
	ContextSize int
	Threads     int
	BatchSize   int
	Temperature float64
	TopP        float64
	ExtraArgs   []string
	Env         []string
}

type ServerCommand struct {
	Path        string
	Args        []string
	Env         []string
	Warnings    []string
	SkippedArgs []string
}

type FlagCapability struct {
	Name      string
	Supported bool
}

type LlamaCapabilities struct {
	SupportsHost      bool
	SupportsPort      bool
	SupportsModel     bool
	SupportsCtxSize   bool
	SupportsThreads   bool
	SupportsBatchSize bool
	SupportsDevice    bool
	RawHelp           string
}

func DetectCapabilities(help string) LlamaCapabilities {
	return LlamaCapabilities{
		SupportsHost:      helpHasFlag(help, "--host"),
		SupportsPort:      helpHasFlag(help, "--port"),
		SupportsModel:     helpHasFlag(help, "-m") || helpHasFlag(help, "--model"),
		SupportsCtxSize:   helpHasFlag(help, "-c") || helpHasFlag(help, "--ctx-size") || helpHasFlag(help, "--ctx_size"),
		SupportsThreads:   helpHasFlag(help, "-t") || helpHasFlag(help, "--threads"),
		SupportsBatchSize: helpHasFlag(help, "-b") || helpHasFlag(help, "--batch-size") || helpHasFlag(help, "--batch_size"),
		SupportsDevice:    helpHasFlag(help, "--device"),
		RawHelp:           help,
	}
}

func BuildServerCommand(req ServerStartRequest, caps LlamaCapabilities, allowUnverifiedFlags bool) (ServerCommand, error) {
	if req.BinaryPath == "" {
		return ServerCommand{}, ActionableError{
			What:    "llama.cpp server command could not be built.",
			Reason:  "binary path is empty",
			Fix:     "Resolve the llama.cpp binary before building a server command.",
			Details: fmt.Sprintf("backend=%s model=%s", req.Backend, req.ModelPath),
		}
	}
	if req.ModelPath == "" {
		return ServerCommand{}, ActionableError{
			What:    "llama.cpp server command could not be built.",
			Reason:  "model path is empty",
			Fix:     "Pass the GGUF path from the model manifest.",
			Details: fmt.Sprintf("backend=%s", req.Backend),
		}
	}
	if req.Port <= 0 || req.Port > 65535 {
		return ServerCommand{}, ActionableError{
			What:    "llama.cpp server command could not be built.",
			Reason:  fmt.Sprintf("invalid internal port %d", req.Port),
			Fix:     "Allocate a free internal port between 1 and 65535.",
			Details: fmt.Sprintf("backend=%s model=%s", req.Backend, req.ModelPath),
		}
	}
	if req.Host == "" {
		req.Host = "127.0.0.1"
	}
	if req.Host == "0.0.0.0" || req.Host == "::" {
		return ServerCommand{}, ActionableError{
			What:    "llama.cpp server command could not be built.",
			Reason:  fmt.Sprintf("internal host %s would expose the llama.cpp process beyond localhost", req.Host),
			Fix:     "Use 127.0.0.1 for internal llama.cpp processes.",
			Details: fmt.Sprintf("backend=%s model=%s port=%d", req.Backend, req.ModelPath, req.Port),
		}
	}
	if !caps.SupportsModel || !caps.SupportsHost || !caps.SupportsPort {
		return ServerCommand{}, ActionableError{
			What:    "llama.cpp server command could not be built.",
			Reason:  fmt.Sprintf("required flags missing: model=%t host=%t port=%t", caps.SupportsModel, caps.SupportsHost, caps.SupportsPort),
			Fix:     "Use a llama.cpp server binary whose --help output includes model, host and port flags.",
			Details: fmt.Sprintf("backend=%s model=%s port=%d", req.Backend, req.ModelPath, req.Port),
		}
	}

	args := []string{preferredModelFlag(caps), req.ModelPath, "--host", req.Host, "--port", strconv.Itoa(req.Port)}
	var warnings []string
	if req.ContextSize > 0 {
		if caps.SupportsCtxSize {
			args = append(args, preferredCtxFlag(caps), strconv.Itoa(req.ContextSize))
		} else {
			warnings = append(warnings, "ctx size flag is not supported by this llama.cpp binary; skipping context size")
		}
	}
	if req.Threads > 0 {
		if caps.SupportsThreads {
			args = append(args, preferredThreadsFlag(caps), strconv.Itoa(req.Threads))
		} else {
			warnings = append(warnings, "threads flag is not supported by this llama.cpp binary; skipping threads")
		}
	}
	if req.BatchSize > 0 {
		if caps.SupportsBatchSize {
			args = append(args, preferredBatchFlag(caps), strconv.Itoa(req.BatchSize))
		} else {
			warnings = append(warnings, "batch size flag is not supported by this llama.cpp binary; skipping batch size")
		}
	}
	filtered, skipped := filterExtraArgs(req.ExtraArgs, caps.RawHelp, allowUnverifiedFlags)
	if len(skipped) > 0 {
		warnings = append(warnings, "one or more extra llama.cpp flags were skipped because they were not present in --help output")
	}
	if allowUnverifiedFlags && len(req.ExtraArgs) > 0 {
		warnings = append(warnings, "allow_unverified_flags is enabled; user-provided flags were passed without full capability validation")
	}
	args = append(args, filtered...)

	return ServerCommand{
		Path:        req.BinaryPath,
		Args:        args,
		Env:         req.Env,
		Warnings:    warnings,
		SkippedArgs: skipped,
	}, nil
}

func preferredModelFlag(caps LlamaCapabilities) string {
	if helpHasFlag(caps.RawHelp, "-m") {
		return "-m"
	}
	return "--model"
}

func preferredCtxFlag(caps LlamaCapabilities) string {
	if helpHasFlag(caps.RawHelp, "-c") {
		return "-c"
	}
	return "--ctx-size"
}

func preferredThreadsFlag(caps LlamaCapabilities) string {
	if helpHasFlag(caps.RawHelp, "-t") {
		return "-t"
	}
	return "--threads"
}

func preferredBatchFlag(caps LlamaCapabilities) string {
	if helpHasFlag(caps.RawHelp, "-b") {
		return "-b"
	}
	return "--batch-size"
}

func filterExtraArgs(extra []string, help string, allowUnverified bool) ([]string, []string) {
	if allowUnverified {
		return append([]string{}, extra...), nil
	}
	var filtered []string
	var skipped []string
	for i := 0; i < len(extra); i++ {
		arg := extra[i]
		if !strings.HasPrefix(arg, "-") {
			filtered = append(filtered, arg)
			continue
		}
		flag := arg
		if idx := strings.Index(flag, "="); idx >= 0 {
			flag = flag[:idx]
		}
		if helpHasFlag(help, flag) {
			filtered = append(filtered, arg)
			if !strings.Contains(arg, "=") && i+1 < len(extra) && !strings.HasPrefix(extra[i+1], "-") {
				i++
				filtered = append(filtered, extra[i])
			}
			continue
		}
		skipped = append(skipped, arg)
		if i+1 < len(extra) && !strings.HasPrefix(extra[i+1], "-") {
			i++
			skipped = append(skipped, extra[i])
		}
	}
	return filtered, skipped
}

func helpHasFlag(help, flag string) bool {
	if help == "" || flag == "" {
		return false
	}
	fields := strings.FieldsFunc(help, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ',', ';', '[', ']', '(', ')':
			return true
		default:
			return false
		}
	})
	for _, field := range fields {
		field = strings.TrimSpace(field)
		field = strings.TrimRight(field, ":")
		if field == flag || strings.HasPrefix(field, flag+"=") {
			return true
		}
	}
	return strings.Contains(help, flag+" ")
}
