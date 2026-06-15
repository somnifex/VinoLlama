package llamacpp

import (
	"fmt"
	"strings"
)

type ActionableError struct {
	What    string
	Reason  string
	Fix     string
	Details string
}

func (e ActionableError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "What happened: %s", strings.TrimSpace(e.What))
	if e.Reason != "" {
		fmt.Fprintf(&b, "\nReason: %s", strings.TrimSpace(e.Reason))
	}
	if e.Fix != "" {
		fmt.Fprintf(&b, "\nFix: %s", strings.TrimSpace(e.Fix))
	}
	if e.Details != "" {
		fmt.Fprintf(&b, "\nDetails: %s", strings.TrimSpace(e.Details))
	}
	return b.String()
}

func stderrTail(text string) string {
	const maxBytes = 8 * 1024
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > 80 {
		lines = lines[len(lines)-80:]
	}
	tail := strings.Join(lines, "\n")
	if len(tail) > maxBytes {
		tail = tail[len(tail)-maxBytes:]
	}
	return tail
}
