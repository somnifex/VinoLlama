package cpu

import "vinollama/internal/backend"

type CPUBackend struct {
	*backend.LlamaCPPBackend
}

func New(binary string) CPUBackend {
	return CPUBackend{LlamaCPPBackend: backend.NewLlamaCPPBackend("cpu", binary)}
}
