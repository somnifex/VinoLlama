package diagnostic

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/llamacpp"
	"vinollama/internal/models"
	vinoruntime "vinollama/internal/runtime"
)

type Level string

const (
	Pass Level = "PASS"
	Warn Level = "WARN"
	Fail Level = "FAIL"
)

type Check struct {
	Name    string
	Level   Level
	What    string
	Reason  string
	Fix     string
	Details string
}

type Report []Check

type Options struct {
	Model      string
	StartCheck bool
}

func (r Report) HasFailures() bool {
	for _, check := range r {
		if check.Level == Fail {
			return true
		}
	}
	return false
}

func Run(ctx context.Context, cfg config.Config, configPath string, configFound bool, options ...Options) Report {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	var report Report
	report = append(report, checkOS())
	report = append(report, checkArch())
	report = append(report, checkIntelCPU())
	report = append(report, checkOpenVINORuntime())
	report = append(report, checkOpenVINODevice())
	report = append(report, checkLlamaBackends(ctx, cfg)...)
	report = append(report, checkInternalPortAllocation(cfg))
	report = append(report, checkModelDirectory(cfg))
	report = append(report, checkRuntimeLogDirectory(cfg))
	if opts.Model != "" {
		report = append(report, checkDoctorModel(ctx, cfg, opts)...)
	}
	report = append(report, checkConfigFile(configPath, configFound))
	report = append(report, checkBindSafety(cfg))
	report = append(report, checkPortAvailable(cfg))
	report = append(report, checkServiceStatus(ctx, cfg))
	return report
}

func checkOS() Check {
	switch runtime.GOOS {
	case "windows", "linux":
		return pass("Operating system", fmt.Sprintf("%s is a priority VinoLlama platform.", runtime.GOOS))
	default:
		return warn("Operating system", fmt.Sprintf("%s is not a priority VinoLlama platform yet.", runtime.GOOS), "Use Windows or Linux for the first supported path.")
	}
}

func checkArch() Check {
	if runtime.GOARCH == "amd64" {
		return pass("CPU architecture", "x86_64/amd64 detected.")
	}
	return warn("CPU architecture", fmt.Sprintf("%s detected; x86_64 is the initial target.", runtime.GOARCH), "Use an x86_64 Intel machine for the preferred OpenVINO path.")
}

func checkIntelCPU() Check {
	brand := cpuBrand()
	if brand == "" {
		return warn("Intel CPU", "CPU brand could not be detected without platform-specific tools.", "Run `vinollama doctor` on the target Intel laptop, or verify CPU details manually.")
	}
	if strings.Contains(strings.ToLower(brand), "intel") {
		return pass("Intel CPU", brand)
	}
	return warn("Intel CPU", fmt.Sprintf("Detected CPU does not look like Intel: %s", brand), "CPU fallback may work later, but OpenVINO acceleration is intended for Intel systems.")
}

func checkOpenVINORuntime() Check {
	for _, key := range []string{"OPENVINO_RUNTIME", "INTEL_OPENVINO_DIR", "OPENVINO_DIR"} {
		if value := os.Getenv(key); value != "" {
			return pass("OpenVINO Runtime", fmt.Sprintf("%s is set.", key))
		}
	}
	return warn("OpenVINO Runtime", "No OpenVINO runtime environment variable was detected.", "Install OpenVINO or configure the llama.cpp OpenVINO binary before using the openvino backend.")
}

func checkOpenVINODevice() Check {
	device := strings.TrimSpace(os.Getenv("GGML_OPENVINO_DEVICE"))
	if device == "" {
		return warn("OpenVINO device selection", "GGML_OPENVINO_DEVICE is not set; llama.cpp OpenVINO defaults will apply.", "Set GGML_OPENVINO_DEVICE to CPU, GPU, NPU, GPU.0, or another OpenVINO target when you need explicit device selection.")
	}
	upper := strings.ToUpper(device)
	stateful := strings.TrimSpace(os.Getenv("GGML_OPENVINO_STATEFUL_EXECUTION"))
	if strings.Contains(upper, "GPU") && stateful == "" {
		return warn("OpenVINO device selection", fmt.Sprintf("GGML_OPENVINO_DEVICE=%s and stateful execution is not enabled.", device), "For GPU targets, set GGML_OPENVINO_STATEFUL_EXECUTION=1 if you hit OpenVINO GPU stateless execution issues.")
	}
	return pass("OpenVINO device selection", fmt.Sprintf("GGML_OPENVINO_DEVICE=%s.", device))
}

func checkLlamaBackends(ctx context.Context, cfg config.Config) []Check {
	resolver := llamacpp.NewBinaryResolver()
	cpu, cpuErr := resolver.Resolve(ctx, llamacpp.BinaryKindCPU, cfg)
	openvino, openvinoErr := resolver.Resolve(ctx, llamacpp.BinaryKindOpenVINO, cfg)
	checks := []Check{
		binaryResolveCheck("llama.cpp CPU binary", llamacpp.BinaryKindCPU, cpu, cpuErr),
		binaryResolveCheck("llama.cpp OpenVINO binary", llamacpp.BinaryKindOpenVINO, openvino, openvinoErr),
		binaryExecutableCheck("CPU binary executable", cpu, cpuErr),
		binaryExecutableCheck("OpenVINO binary executable", openvino, openvinoErr),
		commandBuildCheck("CPU backend command build", cfg, "cpu", cpu, cpuErr),
		commandBuildCheck("OpenVINO backend command build", cfg, "openvino", openvino, openvinoErr),
	}
	switch cfg.Runtime.Backend {
	case "auto":
		if openvinoErr == nil {
			checks = append(checks, pass("Auto backend", "OpenVINO binary is available and will be preferred."))
		} else if cpuErr == nil {
			checks = append(checks, warn("Auto backend", fmt.Sprintf("OpenVINO unavailable: %v; CPU fallback is available.", openvinoErr), "Configure an OpenVINO binary to use acceleration, or keep CPU fallback."))
		} else {
			checks = append(checks, fail("Auto backend", fmt.Sprintf("OpenVINO unavailable: %v; CPU unavailable: %v", openvinoErr, cpuErr), "Configure VINOLLAMA_LLAMA_OPENVINO_BIN or VINOLLAMA_LLAMA_CPU_BIN."))
		}
	case "openvino":
		if openvinoErr != nil {
			checks = append(checks, fail("Selected backend", openvinoErr.Error(), "Configure a valid OpenVINO llama.cpp server binary."))
		}
	case "cpu":
		if cpuErr != nil {
			checks = append(checks, fail("Selected backend", cpuErr.Error(), "Configure a valid CPU llama.cpp server binary."))
		}
	}
	return checks
}

func binaryResolveCheck(name string, kind llamacpp.BinaryKind, resolved llamacpp.ResolvedBinary, err error) Check {
	if err != nil {
		return warn(name, err.Error(), fmt.Sprintf("Configure a valid llama.cpp %s binary path or put it on PATH.", kind))
	}
	return pass(name, fmt.Sprintf("%s resolved from %s.", resolved.Path, resolved.Source))
}

func binaryExecutableCheck(name string, resolved llamacpp.ResolvedBinary, err error) Check {
	if err != nil {
		return warn(name, "Binary was not resolved, so executable status could not be confirmed.", "Fix the binary resolution check first.")
	}
	return pass(name, fmt.Sprintf("%s passed executable and --help checks.", resolved.Path))
}

func commandBuildCheck(name string, cfg config.Config, backend string, resolved llamacpp.ResolvedBinary, resolveErr error) Check {
	if resolveErr != nil {
		return warn(name, "Command build skipped because binary was not resolved.", "Fix the binary resolution check first.")
	}
	extraArgs := cfg.Runtime.ExtraCPUArgs
	if backend == "openvino" {
		extraArgs = cfg.Runtime.ExtraOpenVINOArgs
	}
	cmd, err := llamacpp.BuildServerCommand(llamacpp.ServerStartRequest{
		BinaryPath:  resolved.Path,
		ModelPath:   "doctor-model.gguf",
		Host:        "127.0.0.1",
		Port:        cfg.Runtime.InternalPortStart,
		Backend:     backend,
		ContextSize: cfg.Generation.CtxSize,
		Threads:     cfg.Generation.Threads,
		ExtraArgs:   extraArgs,
	}, llamacpp.DetectCapabilities(resolved.HelpOutput), cfg.Runtime.AllowUnverifiedFlags)
	if err != nil {
		return fail(name, err.Error(), "Use a llama.cpp server binary whose --help output supports model, host and port flags.")
	}
	if len(cmd.Warnings) > 0 || len(cmd.SkippedArgs) > 0 {
		return Check{Name: name, Level: Warn, What: "Check needs attention.", Reason: strings.Join(cmd.Warnings, " "), Fix: "Review unsupported or unverified llama.cpp flags.", Details: strings.Join(cmd.SkippedArgs, " ")}
	}
	return pass(name, fmt.Sprintf("Command can be built with %d argument(s).", len(cmd.Args)))
}

func checkInternalPortAllocation(cfg config.Config) Check {
	port, err := vinoruntime.AllocatePort(cfg.Runtime.InternalPortStart)
	if err != nil {
		return fail("internal port allocation", err.Error(), "Change runtime.internal_port_start or stop processes using the internal port range.")
	}
	return pass("internal port allocation", fmt.Sprintf("Port %d is available for llama.cpp.", port))
}

func checkModelDirectory(cfg config.Config) Check {
	dir, err := config.ModelsDirectory(cfg)
	if err != nil {
		return fail("model directory writable", err.Error(), "Set VINOLLAMA_MODELS or models.directory to a writable directory.")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return warn("model directory writable", fmt.Sprintf("%s does not exist yet.", dir), "It will be created by model-management commands, or create it manually.")
		}
		return fail("model directory writable", fmt.Sprintf("%s could not be accessed: %v", dir, err), "Set VINOLLAMA_MODELS or models.directory to a writable directory.")
	}
	if !info.IsDir() {
		return fail("model directory writable", fmt.Sprintf("%s is not a directory.", dir), "Set VINOLLAMA_MODELS or models.directory to a directory path.")
	}
	if err := writeProbe(dir); err != nil {
		return fail("model directory writable", err.Error(), "Fix model directory permissions.")
	}
	return pass("model directory writable", dir)
}

func checkRuntimeLogDirectory(cfg config.Config) Check {
	dir, err := config.RuntimeLogDirectory(cfg)
	if err != nil {
		return fail("runtime log directory writable", err.Error(), "Set the user home directory or run in a normal user session.")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail("runtime log directory writable", err.Error(), "Fix permissions for the VinoLlama root directory.")
	}
	if err := writeProbe(dir); err != nil {
		return fail("runtime log directory writable", err.Error(), "Fix runtime log directory permissions.")
	}
	return pass("runtime log directory writable", dir)
}

func checkDoctorModel(ctx context.Context, cfg config.Config, opts Options) []Check {
	store, err := models.NewStore(mustModelDirectory(cfg))
	if err != nil {
		return []Check{fail("model manifest exists", err.Error(), "Set VINOLLAMA_MODELS or models.directory.")}
	}
	manifest, err := store.ReadManifest(opts.Model)
	if err != nil {
		return []Check{fail("model manifest exists", err.Error(), "Import the model first with `vinollama import <name> <path-to-gguf>`.")}
	}
	checks := []Check{pass("model manifest exists", store.ManifestPath(manifest.Name))}
	if info, err := os.Stat(manifest.Path); err != nil {
		checks = append(checks, fail("model file exists", err.Error(), "Update the manifest path or re-import the model."))
	} else if info.IsDir() {
		checks = append(checks, fail("model file exists", fmt.Sprintf("%s is a directory", manifest.Path), "Use a GGUF file path."))
	} else {
		checks = append(checks, pass("model file exists", manifest.Path))
	}
	resolver := llamacpp.NewBinaryResolver()
	backend := cfg.Runtime.Backend
	if backend == "" || backend == "auto" {
		backend = "cpu"
	}
	kind := llamacpp.BinaryKindCPU
	if backend == "openvino" {
		kind = llamacpp.BinaryKindOpenVINO
	}
	binary, resolveErr := resolver.Resolve(ctx, kind, cfg)
	if resolveErr != nil {
		checks = append(checks, warn("can build llama.cpp command", resolveErr.Error(), "Fix binary resolution first."))
	} else {
		_, err := llamacpp.BuildServerCommand(llamacpp.ServerStartRequest{
			BinaryPath:  binary.Path,
			ModelPath:   manifest.Path,
			Host:        "127.0.0.1",
			Port:        cfg.Runtime.InternalPortStart,
			Backend:     backend,
			ContextSize: cfg.Generation.CtxSize,
			Threads:     cfg.Generation.Threads,
		}, llamacpp.DetectCapabilities(binary.HelpOutput), cfg.Runtime.AllowUnverifiedFlags)
		if err != nil {
			checks = append(checks, fail("can build llama.cpp command", err.Error(), "Use a compatible llama.cpp server binary."))
		} else {
			checks = append(checks, pass("can build llama.cpp command", fmt.Sprintf("backend=%s model=%s", backend, manifest.Name)))
		}
	}
	if opts.StartCheck {
		manager, err := vinoruntime.NewManager(vinoruntime.ManagerOptions{Config: cfg, Store: store})
		if err != nil {
			checks = append(checks, fail("can start backend", err.Error(), "Fix runtime configuration."))
		} else {
			handle, err := manager.GetOrStartModel(ctx, manifest.Name, vinoruntime.StartOptions{Backend: cfg.Runtime.Backend})
			if err != nil {
				checks = append(checks, fail("can start backend", err.Error(), "Inspect llama.cpp stderr and runtime logs."))
			} else {
				checks = append(checks, pass("can start backend", fmt.Sprintf("backend=%s pid=%d port=%d", handle.Backend, handle.PID, handle.Port)))
				stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = manager.ShutdownAll(stopCtx)
				cancel()
			}
		}
	}
	return checks
}

func checkConfigFile(path string, found bool) Check {
	if found {
		return pass("Config file", fmt.Sprintf("Loaded %s.", path))
	}
	return warn("Config file", fmt.Sprintf("No config file found at %s; safe defaults are active.", path), "Create this file when you need persistent settings.")
}

func checkBindSafety(cfg config.Config) Check {
	if cfg.Server.Host == "0.0.0.0" || cfg.Server.Host == "::" {
		return fail("HTTP bind safety", fmt.Sprintf("Configured host is %s, which exposes the service beyond localhost.", cfg.Server.Host), "Use 127.0.0.1 unless you explicitly design and secure remote access.")
	}
	return pass("HTTP bind safety", fmt.Sprintf("Configured host is %s.", cfg.Server.Host))
}

func checkPortAvailable(cfg config.Config) Check {
	address := net.JoinHostPort("127.0.0.1", fmt.Sprint(cfg.Server.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return warn("Default port availability", fmt.Sprintf("%s is not available: %v", address, err), "Stop the process using this port or configure VINOLLAMA_PORT.")
	}
	_ = listener.Close()
	return pass("Default port availability", fmt.Sprintf("%s is available.", address))
}

func checkServiceStatus(ctx context.Context, cfg config.Config) Check {
	select {
	case <-ctx.Done():
		return warn("Current service status", "Diagnostic was cancelled before probing the local service.", "Run `vinollama doctor` again.")
	default:
	}

	address := net.JoinHostPort(cfg.Server.Host, fmt.Sprint(cfg.Server.Port))
	dialer := net.Dialer{Timeout: 150 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return warn("Current service status", fmt.Sprintf("No VinoLlama service responded at %s.", address), "Start the local API with `vinollama serve` when you need service endpoints.")
	}
	_ = conn.Close()
	return pass("Current service status", fmt.Sprintf("A service is listening at %s.", address))
}

func cpuBrand() string {
	for _, key := range []string{"PROCESSOR_IDENTIFIER", "PROCESSOR_ARCHITECTURE"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.ToLower(line), "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func pass(name, reason string) Check {
	return Check{Name: name, Level: Pass, What: "Check passed.", Reason: reason}
}

func warn(name, reason, fix string) Check {
	return Check{Name: name, Level: Warn, What: "Check needs attention.", Reason: reason, Fix: fix}
}

func fail(name, reason, fix string) Check {
	return Check{Name: name, Level: Fail, What: "Check failed.", Reason: reason, Fix: fix}
}

func writeProbe(dir string) error {
	file, err := os.CreateTemp(dir, ".vinollama-write-probe-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if _, err := file.WriteString("ok"); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func mustModelDirectory(cfg config.Config) string {
	dir, err := config.ModelsDirectory(cfg)
	if err != nil {
		return ""
	}
	return dir
}
