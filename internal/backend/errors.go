package backend

import "fmt"

type ActionableError struct {
	What   string
	Reason string
	Fix    string
}

func (e ActionableError) Error() string {
	return fmt.Sprintf("What happened: %s\nReason: %s\nFix: %s", e.What, e.Reason, e.Fix)
}
