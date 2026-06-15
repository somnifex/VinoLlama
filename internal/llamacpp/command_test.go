package llamacpp

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildServerCommandUsesSupportedArgs(t *testing.T) {
	caps := DetectCapabilities(fakeHelp)
	cmd, err := BuildServerCommand(ServerStartRequest{
		BinaryPath:  "/bin/llama-server",
		ModelPath:   "/models/model.gguf",
		Host:        "127.0.0.1",
		Port:        21435,
		Backend:     "cpu",
		ContextSize: 4096,
		Threads:     8,
		BatchSize:   256,
	}, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-m /models/model.gguf", "--host 127.0.0.1", "--port 21435", "-c 4096", "-t 8", "-b 256"} {
		if !strings.Contains(got, want) {
			t.Fatalf("args %q missing %q", got, want)
		}
	}
}

func TestBuildServerCommandSkipsUnsupportedOptionalArgs(t *testing.T) {
	help := "llama.cpp server\n  --model FNAME\n  --host HOST\n  --port PORT\n"
	caps := DetectCapabilities(help)
	cmd, err := BuildServerCommand(ServerStartRequest{
		BinaryPath:  "/bin/llama-server",
		ModelPath:   "model.gguf",
		Host:        "127.0.0.1",
		Port:        21435,
		ContextSize: 4096,
		Threads:     8,
		BatchSize:   256,
	}, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cmd.Args, " ")
	for _, notWant := range []string{"-c", "--ctx-size", "-t", "--threads", "-b", "--batch-size"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("args %q unexpectedly contains %q", got, notWant)
		}
	}
	if len(cmd.Warnings) < 3 {
		t.Fatalf("Warnings = %#v, want optional flag warnings", cmd.Warnings)
	}
}

func TestBuildServerCommandFiltersExtraArgs(t *testing.T) {
	help := fakeHelp + "\n  --temp N\n  --repeat-penalty N\n"
	caps := DetectCapabilities(help)
	cmd, err := BuildServerCommand(ServerStartRequest{
		BinaryPath: "/bin/llama-server",
		ModelPath:  "model.gguf",
		Host:       "127.0.0.1",
		Port:       21435,
		ExtraArgs:  []string{"--temp", "0.4", "--unknown", "x", "--repeat-penalty=1.1"},
	}, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cmd.SkippedArgs, []string{"--unknown", "x"}) {
		t.Fatalf("SkippedArgs = %#v, want unknown flag skipped", cmd.SkippedArgs)
	}
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{"--temp 0.4", "--repeat-penalty=1.1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("args %q missing %q", got, want)
		}
	}
}

func TestBuildServerCommandAllowsUnverifiedExtraArgs(t *testing.T) {
	caps := DetectCapabilities(fakeHelp)
	cmd, err := BuildServerCommand(ServerStartRequest{
		BinaryPath: "/bin/llama-server",
		ModelPath:  "model.gguf",
		Host:       "127.0.0.1",
		Port:       21435,
		ExtraArgs:  []string{"--unknown", "x"},
	}, caps, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.SkippedArgs) != 0 {
		t.Fatalf("SkippedArgs = %#v, want none", cmd.SkippedArgs)
	}
	if !strings.Contains(strings.Join(cmd.Args, " "), "--unknown x") {
		t.Fatalf("args = %#v, want unknown extra arg passed", cmd.Args)
	}
	if len(cmd.Warnings) == 0 {
		t.Fatal("expected warning when allow_unverified_flags is enabled")
	}
}

func TestBuildServerCommandPreservesModelPathWithSpaces(t *testing.T) {
	caps := DetectCapabilities(fakeHelp)
	modelPath := `C:\models\qwen model.gguf`
	cmd, err := BuildServerCommand(ServerStartRequest{
		BinaryPath: `C:\tools\llama-server.exe`,
		ModelPath:  modelPath,
		Host:       "127.0.0.1",
		Port:       21435,
	}, caps, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) < 2 || cmd.Args[1] != modelPath {
		t.Fatalf("Args = %#v, want model path as a single argument", cmd.Args)
	}
}

func TestBuildServerCommandRejectsMissingRequiredCapabilities(t *testing.T) {
	caps := DetectCapabilities("llama.cpp server\n  --host HOST\n")
	_, err := BuildServerCommand(ServerStartRequest{
		BinaryPath: "/bin/llama-server",
		ModelPath:  "model.gguf",
		Host:       "127.0.0.1",
		Port:       21435,
	}, caps, false)
	if err == nil {
		t.Fatal("expected required flag error")
	}
}
