package openvino

import "vinollama/internal/backend"

type OpenVINOBackend struct {
	*backend.LlamaCPPBackend
}

func New(binary string) OpenVINOBackend {
	return OpenVINOBackend{LlamaCPPBackend: backend.NewLlamaCPPBackend("openvino", binary)}
}
