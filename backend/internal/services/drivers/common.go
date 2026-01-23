package drivers

import (
	"context"
)

// IPMIExecutor defines the interface for low-level IPMI operations that drivers need
// Drivers must implement their own high-level logic using this interface
type IPMIExecutor interface {
	// RunCommand executes an IPMI command and returns the output
	RunCommand(ctx context.Context, args ...string) ([]byte, error)
}
