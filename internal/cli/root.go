package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"vinollama/internal/config"
	"vinollama/internal/diagnostic"
	"vinollama/internal/llamacpp"
	applog "vinollama/internal/logging"
	"vinollama/internal/models"
	vinoruntime "vinollama/internal/runtime"
	"vinollama/internal/server"
)

const version = "0.1.0"

// Execute runs the VinoLlama command line interface.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("vinollama", flag.ContinueOnError)
	flags.SetOutput(stderr)

	configPath := flags.String("config", "", "Path to config.yaml")
	backend := flags.String("backend", "", "Runtime backend: auto, openvino, or cpu")
	verbose := flags.Bool("verbose", false, "Enable debug logging")
	help := flags.Bool("help", false, "Show help")
	flags.BoolVar(help, "h", false, "Show help")

	if err := flags.Parse(args); err != nil {
		printActionableError(stderr, "Failed to parse command line flags.", err.Error(), "Run `vinollama --help` to see supported usage.")
		return 2
	}

	rest := flags.Args()
	if *help || len(rest) == 0 {
		printHelp(stdout)
		return 0
	}

	if *backend != "" && !config.ValidBackend(*backend) {
		printActionableError(stderr, "Invalid backend value.", fmt.Sprintf("%q is not supported.", *backend), "Use one of: auto, openvino, cpu.")
		return 2
	}

	switch rest[0] {
	case "help":
		printHelp(stdout)
		return 0
	case "doctor":
		return runDoctor(ctx, stdout, stderr, *configPath, *backend, *verbose, rest[1:])
	case "list":
		return runList(stdout, stderr, *configPath, *backend)
	case "import":
		return runImport(stdout, stderr, *configPath, *backend, rest[1:])
	case "rm":
		return runRemove(stdout, stderr, *configPath, *backend, rest[1:])
	case "ps":
		return runPS(ctx, stdout, stderr, *configPath, *backend)
	case "stop":
		return runStop(ctx, stdout, stderr, *configPath, *backend, rest[1:])
	case "serve":
		return runServe(ctx, stdout, stderr, *configPath, *backend, *verbose)
	case "run":
		printActionableError(stderr, "Command is planned but not implemented in this phase.", fmt.Sprintf("`vinollama %s` belongs to a later development phase.", rest[0]), "Use `vinollama doctor` for the current stage-1 diagnostic command.")
		return 2
	default:
		printActionableError(stderr, "Unknown command.", fmt.Sprintf("`%s` is not a VinoLlama command.", rest[0]), "Run `vinollama --help` to see supported commands.")
		return 2
	}
}

func runServe(ctx context.Context, stdout, stderr io.Writer, configPath, backend string, verbose bool) int {
	loaded, err := config.Load(configPath)
	if err != nil {
		printActionableError(stderr, "Configuration could not be loaded.", err.Error(), "Check the config path and YAML syntax, or run without --config to use safe defaults.")
		return 1
	}
	if backend != "" {
		loaded.Config.Runtime.Backend = backend
	}
	if verbose {
		loaded.Config.Logging.Level = "debug"
	}
	if loaded.Config.Server.Host == "0.0.0.0" || loaded.Config.Server.Host == "::" {
		printActionableError(stderr, "Service bind address is unsafe.", fmt.Sprintf("configured host is %s", loaded.Config.Server.Host), "Use 127.0.0.1 unless remote access is explicitly designed and secured.")
		return 1
	}
	store, err := storeFromLoadedConfig(loaded.Config)
	if err != nil {
		printActionableError(stderr, "Model store could not be opened.", err.Error(), "Check the config path, VINOLLAMA_MODELS, and model directory permissions.")
		return 1
	}
	manager, err := vinoruntime.NewManager(vinoruntime.ManagerOptions{Config: loaded.Config, Store: store})
	if err != nil {
		printActionableError(stderr, "Runtime manager could not be initialized.", err.Error(), "Check runtime configuration and model directory permissions.")
		return 1
	}
	addr := fmt.Sprintf("%s:%d", loaded.Config.Server.Host, loaded.Config.Server.Port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewHandler(loaded.Config, manager, store),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()
	fmt.Fprintf(stdout, "VinoLlama serving on http://%s\n", addr)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			printActionableError(stderr, "VinoLlama service stopped unexpectedly.", err.Error(), "Check that the host and port are available.")
			return 1
		}
		return 0
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		_ = manager.ShutdownAll(shutdownCtx)
		return 0
	}
}

func runPS(ctx context.Context, stdout, stderr io.Writer, configPath, backend string) int {
	loaded, err := config.Load(configPath)
	if err != nil {
		printActionableError(stderr, "Configuration could not be loaded.", err.Error(), "Check the config path and YAML syntax, or run without --config to use safe defaults.")
		return 1
	}
	if backend != "" {
		loaded.Config.Runtime.Backend = backend
	}
	processes, ok := fetchRuntimeProcesses(ctx, loaded.Config)
	if !ok {
		manager, err := runtimeManagerFromConfig(loaded.Config)
		if err != nil {
			printActionableError(stderr, "Runtime manager could not be initialized.", err.Error(), "Check model directory configuration and permissions.")
			return 1
		}
		processes = manager.ListProcesses()
	}
	printRuntimeProcesses(stdout, processes)
	return 0
}

func runStop(ctx context.Context, stdout, stderr io.Writer, configPath, backend string, args []string) int {
	if len(args) != 1 {
		printActionableError(stderr, "Stop arguments are invalid.", fmt.Sprintf("expected model name, got %d argument(s)", len(args)), "Use `vinollama stop <model>`.")
		return 2
	}
	modelName := args[0]
	loaded, err := config.Load(configPath)
	if err != nil {
		printActionableError(stderr, "Configuration could not be loaded.", err.Error(), "Check the config path and YAML syntax, or run without --config to use safe defaults.")
		return 1
	}
	if backend != "" {
		loaded.Config.Runtime.Backend = backend
	}
	if ok, err := stopViaService(ctx, loaded.Config, modelName); ok {
		if err != nil {
			printActionableError(stderr, "Model process could not be stopped.", err.Error(), "Check `vinollama ps` and runtime logs.")
			return 1
		}
		fmt.Fprintf(stdout, "Stopped %s\n", modelName)
		return 0
	}
	manager, err := runtimeManagerFromConfig(loaded.Config)
	if err != nil {
		printActionableError(stderr, "Runtime manager could not be initialized.", err.Error(), "Check model directory configuration and permissions.")
		return 1
	}
	stopped, err := manager.StopModel(ctx, modelName)
	if err != nil {
		printActionableError(stderr, "Model process could not be stopped.", err.Error(), "Check `vinollama ps` and runtime logs.")
		return 1
	}
	if !stopped {
		fmt.Fprintf(stdout, "No running process found for %s\n", modelName)
		return 0
	}
	fmt.Fprintf(stdout, "Stopped %s\n", modelName)
	return 0
}

func runList(stdout, stderr io.Writer, configPath, backend string) int {
	store, err := storeFromConfig(configPath, backend)
	if err != nil {
		printActionableError(stderr, "Model store could not be opened.", err.Error(), "Check the config path, VINOLLAMA_MODELS, and model directory permissions.")
		return 1
	}
	manifests, err := store.List()
	if err != nil {
		printActionableError(stderr, "Models could not be listed.", err.Error(), "Check that the model manifest directory is readable.")
		return 1
	}

	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPARAMETERS\tQUANTIZATION\tSIZE\tBACKEND_HINT\tMODIFIED")
	for _, manifest := range manifests {
		modified := "unknown"
		if !manifest.ModifiedAt.IsZero() {
			modified = manifest.ModifiedAt.Format("2006-01-02T15:04:05Z")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", manifest.Name, manifest.Parameters, manifest.Quantization, formatBytes(manifest.Size), manifest.BackendHint, modified)
	}
	_ = tw.Flush()
	return 0
}

func runImport(stdout, stderr io.Writer, configPath, backend string, args []string) int {
	loaded, err := config.Load(configPath)
	if err != nil {
		printActionableError(stderr, "Configuration could not be loaded.", err.Error(), "Check the config path and YAML syntax, or run without --config to use safe defaults.")
		return 1
	}
	if backend != "" {
		loaded.Config.Runtime.Backend = backend
	}
	name, path, mode, err := parseImportArgs(args, loaded.Config.Models.DefaultImportMode)
	if err != nil {
		printActionableError(stderr, "Import arguments are invalid.", err.Error(), "Use `vinollama import <name> <path-to-gguf> [--reference|--copy|--link]`.")
		return 2
	}

	store, err := storeFromLoadedConfig(loaded.Config)
	if err != nil {
		printActionableError(stderr, "Model store could not be opened.", err.Error(), "Set VINOLLAMA_MODELS or models.directory to a writable directory.")
		return 1
	}
	manifest, err := store.Import(models.ImportRequest{Name: name, Path: path, Mode: mode})
	if err != nil {
		printActionableError(stderr, "Model could not be imported.", err.Error(), "Confirm the path points to a local GGUF file and the model directory is writable.")
		return 1
	}

	fmt.Fprintf(stdout, "Imported %s\n", manifest.Name)
	fmt.Fprintf(stdout, "Source: %s\n", manifest.Source)
	fmt.Fprintf(stdout, "Path: %s\n", manifest.Path)
	fmt.Fprintf(stdout, "Parameters: %s\n", manifest.Parameters)
	fmt.Fprintf(stdout, "Quantization: %s\n", manifest.Quantization)
	return 0
}

func runRemove(stdout, stderr io.Writer, configPath, backend string, args []string) int {
	name, deleteFile, yes, err := parseRemoveArgs(args)
	if err != nil {
		printActionableError(stderr, "Remove arguments are invalid.", err.Error(), "Use `vinollama rm <model> [--delete-file] [--yes]`.")
		return 2
	}
	if !yes {
		fmt.Fprintf(stdout, "Delete model record %q? Type yes to continue: ", name)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "yes" && answer != "y" {
			fmt.Fprintln(stdout, "Cancelled.")
			return 0
		}
	}

	store, err := storeFromConfig(configPath, backend)
	if err != nil {
		printActionableError(stderr, "Model store could not be opened.", err.Error(), "Check the config path, VINOLLAMA_MODELS, and model directory permissions.")
		return 1
	}
	result, err := store.Delete(name, deleteFile)
	if err != nil {
		printActionableError(stderr, "Model could not be removed.", err.Error(), "Check the model name and only use --delete-file when you really want to remove the GGUF file.")
		return 1
	}

	fmt.Fprintf(stdout, "Removed manifest: %s\n", result.ManifestPath)
	if result.FileDeleted {
		fmt.Fprintf(stdout, "Deleted model file: %s\n", result.ModelPath)
	} else {
		fmt.Fprintln(stdout, "Model file left untouched.")
	}
	return 0
}

func runDoctor(ctx context.Context, stdout, stderr io.Writer, configPath, backend string, verbose bool, args []string) int {
	doctorFlags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	doctorFlags.SetOutput(stderr)
	model := doctorFlags.String("model", "", "Model name to validate")
	startCheck := doctorFlags.Bool("start-check", false, "Start the backend for the selected model")
	if err := doctorFlags.Parse(args); err != nil {
		printActionableError(stderr, "Doctor arguments are invalid.", err.Error(), "Use `vinollama doctor [--model <name>] [--start-check]`.")
		return 2
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		printActionableError(stderr, "Configuration could not be loaded.", err.Error(), "Check the config path and YAML syntax, or run without --config to use safe defaults.")
		return 1
	}

	if backend != "" {
		loaded.Config.Runtime.Backend = backend
	}
	if verbose {
		loaded.Config.Logging.Level = "debug"
	}

	logger, err := applog.New(loaded.Config.Logging.Level, stderr)
	if err != nil {
		printActionableError(stderr, "Logger could not be initialized.", err.Error(), "Use a logging.level value of debug, info, warn, or error.")
		return 1
	}
	logger.Debug("doctor starting", slog.String("config_path", loaded.Path), slog.Bool("config_found", loaded.Found))

	report := diagnostic.Run(ctx, loaded.Config, loaded.Path, loaded.Found, diagnostic.Options{Model: *model, StartCheck: *startCheck})
	printDoctor(stdout, report)
	if report.HasFailures() {
		return 1
	}
	return 0
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, `VinoLlama %s

Local-first LLM chat tooling for Intel laptops and processors.

Usage:
  vinollama [flags] <command> [args]

Implemented commands:
  doctor                  Run local environment diagnostics
  import <name> <path>    Import a GGUF model manifest
  list                    List local GGUF model manifests
  rm <model>              Remove a model manifest
  serve                   Start the local HTTP API on 127.0.0.1:11435 by default

Planned commands:
  run <model>             Start an interactive local chat

Runtime commands:
  ps                      Show running model processes
  stop <model>            Stop a running model process

Flags:
  --config <path>         Path to config.yaml
  --backend <backend>     Runtime backend: auto, openvino, or cpu
  --verbose               Enable debug logging
  -h, --help              Show this help

Safe defaults:
  HTTP bind: 127.0.0.1
  HTTP port: 11435
  Telemetry: false

`, version)
}

func printDoctor(w io.Writer, report diagnostic.Report) {
	fmt.Fprintln(w, "VinoLlama doctor")
	fmt.Fprintln(w, "================")
	for _, check := range report {
		fmt.Fprintf(w, "[%s] %s\n", check.Level, check.Name)
		if check.What != "" {
			fmt.Fprintf(w, "  What happened: %s\n", check.What)
		}
		if check.Reason != "" {
			fmt.Fprintf(w, "  Reason: %s\n", check.Reason)
		}
		if check.Fix != "" {
			fmt.Fprintf(w, "  Fix: %s\n", check.Fix)
		}
		if check.Details != "" {
			fmt.Fprintf(w, "  Details: %s\n", check.Details)
		}
	}
}

func printActionableError(w io.Writer, what, reason, fix string) {
	fmt.Fprintf(w, "What happened: %s\n", strings.TrimSpace(what))
	fmt.Fprintf(w, "Reason: %s\n", strings.TrimSpace(reason))
	fmt.Fprintf(w, "Fix: %s\n", strings.TrimSpace(fix))
}

func storeFromConfig(configPath, backend string) (models.Store, error) {
	loaded, err := config.Load(configPath)
	if err != nil {
		return models.Store{}, err
	}
	if backend != "" {
		loaded.Config.Runtime.Backend = backend
	}
	return storeFromLoadedConfig(loaded.Config)
}

func storeFromLoadedConfig(cfg config.Config) (models.Store, error) {
	modelDir, err := config.ModelsDirectory(cfg)
	if err != nil {
		return models.Store{}, err
	}
	return models.NewStore(modelDir)
}

func parseImportArgs(args []string, defaultMode string) (name, path, mode string, err error) {
	mode = defaultMode
	if mode == "" {
		mode = models.SourceReference
	}
	var positionals []string
	modeSet := false
	for _, arg := range args {
		switch arg {
		case "--reference":
			if modeSet && mode != models.SourceReference {
				return "", "", "", fmt.Errorf("only one import mode can be selected")
			}
			mode = models.SourceReference
			modeSet = true
		case "--copy":
			if modeSet && mode != models.SourceCopy {
				return "", "", "", fmt.Errorf("only one import mode can be selected")
			}
			mode = models.SourceCopy
			modeSet = true
		case "--link":
			if modeSet && mode != models.SourceLink {
				return "", "", "", fmt.Errorf("only one import mode can be selected")
			}
			mode = models.SourceLink
			modeSet = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", "", fmt.Errorf("unknown import flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 2 {
		return "", "", "", fmt.Errorf("expected model name and GGUF path, got %d argument(s)", len(positionals))
	}
	return positionals[0], positionals[1], mode, nil
}

func parseRemoveArgs(args []string) (name string, deleteFile bool, yes bool, err error) {
	var positionals []string
	for _, arg := range args {
		switch arg {
		case "--delete-file":
			deleteFile = true
		case "--yes", "-y":
			yes = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, false, fmt.Errorf("unknown remove flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return "", false, false, fmt.Errorf("expected model name, got %d argument(s)", len(positionals))
	}
	return positionals[0], deleteFile, yes, nil
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func runtimeManagerFromConfig(cfg config.Config) (*vinoruntime.Manager, error) {
	store, err := storeFromLoadedConfig(cfg)
	if err != nil {
		return nil, err
	}
	return vinoruntime.NewManager(vinoruntime.ManagerOptions{Config: cfg, Store: store})
}

func printRuntimeProcesses(w io.Writer, processes []llamacpp.ProcessHandle) {
	if len(processes) == 0 {
		fmt.Fprintln(w, "No running models.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "MODEL\tBACKEND\tPID\tPORT\tSTATE\tSTARTED\tLAST_USED\tLOG")
	for _, process := range processes {
		started := "unknown"
		if !process.StartedAt.IsZero() {
			started = process.StartedAt.Format(time.RFC3339)
		}
		lastUsed := "unknown"
		if !process.LastUsedAt.IsZero() {
			lastUsed = process.LastUsedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n", process.Model, process.Backend, process.PID, process.Port, process.State, started, lastUsed, process.LogPath)
	}
	_ = tw.Flush()
}

func fetchRuntimeProcesses(ctx context.Context, cfg config.Config) ([]llamacpp.ProcessHandle, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, fmt.Sprintf("http://%s:%d/api/runtime", cfg.Server.Host, cfg.Server.Port), nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false
	}
	var payload struct {
		Processes []llamacpp.ProcessHandle `json:"processes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false
	}
	return payload.Processes, true
}

func stopViaService(ctx context.Context, cfg config.Config, modelName string) (bool, error) {
	body, _ := json.Marshal(map[string]string{"model": modelName})
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, fmt.Sprintf("http://%s:%d/api/runtime/stop", cfg.Server.Host, cfg.Server.Port), bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		if payload.Error != "" {
			return true, fmt.Errorf("%s", payload.Error)
		}
		return true, fmt.Errorf("runtime stop endpoint returned HTTP %d", resp.StatusCode)
	}
	return true, nil
}
