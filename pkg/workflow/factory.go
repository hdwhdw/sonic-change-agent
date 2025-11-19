package workflow

import (
	"fmt"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/client"
)

// NewWorkflow creates a workflow instance based on operation-operationAction type
func NewWorkflow(workflowType string, gnoiClient client.Client) (Workflow, error) {
	switch workflowType {
	// Legacy support for old workflow types
	case "preload":
		return NewPreloadWorkflow(gnoiClient), nil
	// New operation-operationAction based workflows
	case "OSUpgrade-PreloadImage":
		return NewPreloadWorkflow(gnoiClient), nil
	default:
		return nil, fmt.Errorf("unknown workflow type: %s", workflowType)
	}
}
