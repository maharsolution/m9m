package core

import (
	"fmt"
	"os"
)

// readWorkflowFile loads a workflow JSON from disk. It is split out from
// ExecuteWorkflowNode so the os import is isolated to a leaf file and
// the node code stays focused on resolution + execution semantics.
func readWorkflowFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read workflow file: %v", err)
	}
	return data, nil
}
