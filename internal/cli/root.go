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
	"os/signal"
	"path/filepath"
	"strconv"
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
	return ExecuteWithIO(ctx, args, os.Stdin, stdout, stderr)
}

// ExecuteWithIO runs the VinoLlama command line interface with injectable stdin.
func ExecuteWithIO(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
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

	command := rest[0]
	if command == "run" {
		return runInteractiveChat(ctx, stdin, stdout, stderr, *configPath, *backend, rest[1:])
	}

	cmdCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
	defer stopSignals()

	switch command {
	case "help":
		printHelp(stdout)
		return 0
	case "doctor":
		return runDoctor(cmdCtx, stdout, stderr, *configPath, *backend, *verbose, rest[1:])
	case "list":
		return runList(stdout, stderr, *configPath, *backend)
	case "import":
		return runImport(stdout, stderr, *configPath, *backend, rest[1:])
	case "rm":
		return runRemove(stdin, stdout, stderr, *configPath, *backend, rest[1:])
	case "ps":
		return runPS(cmdCtx, stdout, stderr, *configPath, *backend)
	case "stop":
		return runStop(cmdCtx, stdout, stderr, *configPath, *backend, rest[1:])
	case "serve":
		return runServe(cmdCtx, stdout, stderr, *configPath, *backend, *verbose)
	default:
		printActionableError(stderr, "Unknown command.", fmt.Sprintf("`%s` is not a VinoLlama command.", command), "Run `vinollama --help` to see supported commands.")
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

func runRemove(stdin io.Reader, stdout, stderr io.Writer, configPath, backend string, args []string) int {
	name, deleteFile, yes, err := parseRemoveArgs(args)
	if err != nil {
		printActionableError(stderr, "Remove arguments are invalid.", err.Error(), "Use `vinollama rm <model> [--delete-file] [--yes]`.")
		return 2
	}
	if !yes {
		fmt.Fprintf(stdout, "Delete model record %q? Type yes to continue: ", name)
		reader := bufio.NewReader(stdin)
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

type runCommandOptions struct {
	Model       string
	Backend     string
	ContextSize int
	Threads     int
	Stream      bool
}

func runInteractiveChat(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, configPath, globalBackend string, args []string) int {
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	return runInteractiveChatWithInterrupts(ctx, stdin, stdout, stderr, configPath, globalBackend, args, interrupts)
}

func runInteractiveChatWithInterrupts(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, configPath, globalBackend string, args []string, interrupts <-chan os.Signal) int {
	options, err := parseRunArgs(args, globalBackend)
	if err != nil {
		printActionableError(stderr, "Run arguments are invalid.", err.Error(), "Use `vinollama run <model> [--backend auto|openvino|cpu] [--ctx-size N] [--threads N] [--stream]`.")
		return 2
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		printActionableError(stderr, "Configuration could not be loaded.", err.Error(), "Check the config path and YAML syntax, or run without --config to use safe defaults.")
		return 1
	}
	if options.Backend != "" {
		loaded.Config.Runtime.Backend = options.Backend
	}
	store, err := storeFromLoadedConfig(loaded.Config)
	if err != nil {
		printActionableError(stderr, "Model store could not be opened.", err.Error(), "Check the config path, VINOLLAMA_MODELS, and model directory permissions.")
		return 1
	}
	modelName, imported, err := resolveRunModel(store, options.Model)
	if err != nil {
		printActionableError(stderr, "Model could not be prepared for chat.", err.Error(), "Import the GGUF first with `vinollama import <name> <path-to-gguf>` or pass a local .gguf path.")
		return 1
	}
	if imported {
		fmt.Fprintf(stdout, "Imported %s by reference.\n", modelName)
	}
	manager, err := vinoruntime.NewManager(vinoruntime.ManagerOptions{Config: loaded.Config, Store: store})
	if err != nil {
		printActionableError(stderr, "Runtime manager could not be initialized.", err.Error(), "Check runtime configuration and model directory permissions.")
		return 1
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = manager.ShutdownAll(shutdownCtx)
	}()

	startOptions := vinoruntime.StartOptions{
		Backend:     options.Backend,
		ContextSize: options.ContextSize,
		Threads:     options.Threads,
	}
	messages := []llamacpp.ChatMessage{}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lines, inputErrs := scanInputLines(scanner)

	fmt.Fprintf(stdout, "Running %s. Type /exit or /quit to stop.\n", modelName)
	for {
		if err := ctx.Err(); err != nil {
			fmt.Fprintln(stdout)
			printActionableError(stderr, "Chat was interrupted.", err.Error(), "Run `vinollama run <model>` again when you are ready to continue.")
			return 130
		}
		fmt.Fprint(stdout, "> ")
		text, ok, code := readChatInput(ctx, stdout, stderr, lines, inputErrs, interrupts)
		if code != 0 {
			return code
		}
		if !ok {
			fmt.Fprintln(stdout)
			return 0
		}
		text = strings.TrimSpace(text)
		switch strings.ToLower(text) {
		case "":
			continue
		case "/exit", "/quit":
			return 0
		}

		messages = append(messages, llamacpp.ChatMessage{Role: "user", Content: text})
		reply, interrupted, exitRequested, err := runChatTurnWithInterrupts(ctx, manager, modelName, messages, startOptions, options.Stream, stdout, interrupts)
		if exitRequested {
			return 130
		}
		if interrupted {
			messages = messages[:len(messages)-1]
			continue
		}
		if err != nil {
			printActionableError(stderr, "Chat request failed.", err.Error(), "Run `vinollama doctor --model "+modelName+" --start-check` and inspect runtime logs.")
			return 1
		}
		messages = append(messages, llamacpp.ChatMessage{Role: "assistant", Content: reply})
	}
}

func scanInputLines(scanner *bufio.Scanner) (<-chan string, <-chan error) {
	lines := make(chan string)
	errs := make(chan error, 1)
	go func() {
		defer close(lines)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			errs <- err
		}
		close(errs)
	}()
	return lines, errs
}

func readChatInput(ctx context.Context, stdout, stderr io.Writer, lines <-chan string, inputErrs <-chan error, interrupts <-chan os.Signal) (string, bool, int) {
	select {
	case <-ctx.Done():
		fmt.Fprintln(stdout)
		printActionableError(stderr, "Chat was interrupted.", ctx.Err().Error(), "Run `vinollama run <model>` again when you are ready to continue.")
		return "", false, 130
	case <-interrupts:
		fmt.Fprintln(stdout)
		return "", false, 130
	case line, ok := <-lines:
		if ok {
			return line, true, 0
		}
		if err, hasErr := <-inputErrs; hasErr && err != nil {
			printActionableError(stderr, "Chat input could not be read.", err.Error(), "Check terminal input and try again.")
			return "", false, 1
		}
		return "", false, 0
	}
}

func runChatTurnWithInterrupts(ctx context.Context, manager *vinoruntime.Manager, modelName string, messages []llamacpp.ChatMessage, startOptions vinoruntime.StartOptions, stream bool, stdout io.Writer, interrupts <-chan os.Signal) (reply string, interrupted bool, exitRequested bool, err error) {
	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type turnResult struct {
		reply string
		err   error
	}
	resultCh := make(chan turnResult, 1)
	go func() {
		reply, err := runChatTurn(turnCtx, manager, modelName, messages, startOptions, stream, stdout)
		resultCh <- turnResult{reply: reply, err: err}
	}()

	interrupted = false
	for {
		select {
		case <-ctx.Done():
			cancel()
			return "", interrupted, true, ctx.Err()
		case <-interrupts:
			if interrupted {
				cancel()
				fmt.Fprintln(stdout)
				return "", true, true, nil
			}
			interrupted = true
			cancel()
			fmt.Fprintln(stdout)
			fmt.Fprintln(stdout, "Generation interrupted. Press Ctrl+C again to exit, or enter another prompt.")
		case result := <-resultCh:
			if interrupted {
				return "", true, false, nil
			}
			return result.reply, false, false, result.err
		}
	}
}

func runChatTurn(ctx context.Context, manager *vinoruntime.Manager, modelName string, messages []llamacpp.ChatMessage, startOptions vinoruntime.StartOptions, stream bool, stdout io.Writer) (string, error) {
	req := llamacpp.ChatRequest{Model: modelName, Messages: messages, Stream: stream}
	if !stream {
		resp, err := manager.ProxyChatWithOptions(ctx, req, startOptions)
		if err != nil {
			return "", err
		}
		fmt.Fprintln(stdout, resp.Message.Content)
		return resp.Message.Content, nil
	}

	ch, err := manager.ProxyChatStreamWithOptions(ctx, req, startOptions)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range ch {
		if chunk.Error != "" {
			return b.String(), fmt.Errorf("%s", chunk.Error)
		}
		content := chunk.Message.Content
		if content != "" {
			fmt.Fprint(stdout, content)
			b.WriteString(content)
		}
		if chunk.Done {
			break
		}
	}
	fmt.Fprintln(stdout)
	return b.String(), nil
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
  run <model>             Start an interactive local chat
  run <path-to-gguf>      Import a local GGUF by reference, then chat
  serve                   Start the local HTTP API on 127.0.0.1:11435 by default

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

func parseRunArgs(args []string, defaultBackend string) (runCommandOptions, error) {
	options := runCommandOptions{Backend: defaultBackend}
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--backend":
			value, err := nextFlagValue(args, &i, arg)
			if err != nil {
				return runCommandOptions{}, err
			}
			if !config.ValidBackend(value) {
				return runCommandOptions{}, fmt.Errorf("backend must be one of auto, openvino, cpu, got %q", value)
			}
			options.Backend = value
		case "--ctx-size":
			value, err := nextFlagValue(args, &i, arg)
			if err != nil {
				return runCommandOptions{}, err
			}
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return runCommandOptions{}, fmt.Errorf("--ctx-size must be a positive integer")
			}
			options.ContextSize = parsed
		case "--threads":
			value, err := nextFlagValue(args, &i, arg)
			if err != nil {
				return runCommandOptions{}, err
			}
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				return runCommandOptions{}, fmt.Errorf("--threads must be zero or a positive integer")
			}
			options.Threads = parsed
		case "--stream":
			options.Stream = true
		default:
			if strings.HasPrefix(arg, "-") {
				return runCommandOptions{}, fmt.Errorf("unknown run flag %q", arg)
			}
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return runCommandOptions{}, fmt.Errorf("expected one model name or .gguf path, got %d argument(s)", len(positionals))
	}
	options.Model = positionals[0]
	return options, nil
}

func nextFlagValue(args []string, index *int, flagName string) (string, error) {
	if *index+1 >= len(args) || strings.HasPrefix(args[*index+1], "-") {
		return "", fmt.Errorf("%s requires a value", flagName)
	}
	*index = *index + 1
	return args[*index], nil
}

func resolveRunModel(store models.Store, modelSpec string) (string, bool, error) {
	if strings.ToLower(filepath.Ext(modelSpec)) != ".gguf" {
		clean, err := models.CleanName(modelSpec)
		if err != nil {
			return "", false, err
		}
		return clean, false, nil
	}
	absPath, err := filepath.Abs(modelSpec)
	if err != nil {
		return "", false, fmt.Errorf("resolve GGUF path: %w", err)
	}
	name := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	cleanName, err := models.CleanName(name)
	if err != nil {
		return "", false, err
	}
	if manifest, err := store.ReadManifest(cleanName); err == nil {
		manifestPath, _ := filepath.Abs(manifest.Path)
		if samePath(manifestPath, absPath) {
			return manifest.Name, false, nil
		}
		return "", false, fmt.Errorf("model %q already exists and points to %s", cleanName, manifest.Path)
	}
	manifest, err := store.Import(models.ImportRequest{Name: cleanName, Path: absPath, Mode: models.SourceReference})
	if err != nil {
		return "", false, err
	}
	return manifest.Name, true, nil
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) {
		return true
	}
	return filepath.Clean(a) == filepath.Clean(b)
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
