package backend

import (
	"context"
	"fmt"
)

type AutoBackend struct {
	OpenVINO Backend
	CPU      Backend
}

func NewAutoBackend(openvino Backend, cpu Backend) AutoBackend {
	return AutoBackend{OpenVINO: openvino, CPU: cpu}
}

func (b AutoBackend) Name() string {
	return "auto"
}

func (b AutoBackend) Check(ctx context.Context) DiagnosticResult {
	openvino := b.OpenVINO.Check(ctx)
	if openvino.Level == LevelPass {
		return DiagnosticResult{
			Name:   b.Name(),
			Level:  LevelPass,
			What:   "Auto backend selected OpenVINO.",
			Reason: openvino.Reason,
		}
	}
	cpu := b.CPU.Check(ctx)
	if cpu.Level == LevelPass {
		return DiagnosticResult{
			Name:   b.Name(),
			Level:  LevelWarn,
			What:   "Auto backend will fall back to CPU.",
			Reason: fmt.Sprintf("OpenVINO unavailable: %s", openvino.Reason),
			Fix:    "Configure the OpenVINO binary to use acceleration, or keep CPU fallback.",
		}
	}
	return DiagnosticResult{
		Name:   b.Name(),
		Level:  LevelFail,
		What:   "Auto backend has no available runtime.",
		Reason: fmt.Sprintf("OpenVINO unavailable: %s; CPU unavailable: %s", openvino.Reason, cpu.Reason),
		Fix:    "Configure VINOLLAMA_LLAMA_OPENVINO_BIN or VINOLLAMA_LLAMA_CPU_BIN.",
	}
}

func (b AutoBackend) Start(ctx context.Context, req StartRequest) (*ProcessHandle, error) {
	if b.OpenVINO.Check(ctx).Level == LevelPass {
		return b.OpenVINO.Start(ctx, req)
	}
	if b.CPU.Check(ctx).Level == LevelPass {
		return b.CPU.Start(ctx, req)
	}
	result := b.Check(ctx)
	return nil, ActionableError{
		What:   result.What,
		Reason: result.Reason,
		Fix:    result.Fix,
	}
}

func (b AutoBackend) Stop(ctx context.Context, handle *ProcessHandle) error {
	if handle == nil {
		return nil
	}
	switch handle.BackendName {
	case "openvino":
		return b.OpenVINO.Stop(ctx, handle)
	case "cpu":
		return b.CPU.Stop(ctx, handle)
	default:
		return nil
	}
}

func (b AutoBackend) Health(ctx context.Context, handle *ProcessHandle) error {
	if handle == nil {
		return nil
	}
	switch handle.BackendName {
	case "openvino":
		return b.OpenVINO.Health(ctx, handle)
	case "cpu":
		return b.CPU.Health(ctx, handle)
	default:
		return nil
	}
}
