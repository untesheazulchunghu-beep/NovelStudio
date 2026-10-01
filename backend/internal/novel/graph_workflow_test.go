package novel

import "testing"

func TestWorkflowRejectsCycles(t *testing.T) {
	definition := WorkflowDefinition{
		Nodes: []WorkflowNode{{ID: "a", Kind: "context"}, {ID: "b", Kind: "generate"}},
		Edges: []WorkflowEdge{{ID: "ab", Source: "a", Target: "b"}, {ID: "ba", Source: "b", Target: "a"}},
	}
	if validateWorkflow(definition) {
		t.Fatal("cyclic workflows must be rejected")
	}
}

func TestWorkflowExecutionOrderFollowsConnections(t *testing.T) {
	definition := WorkflowDefinition{
		Nodes: []WorkflowNode{{ID: "output", Kind: "output"}, {ID: "write", Kind: "generate"}, {ID: "context", Kind: "context"}},
		Edges: []WorkflowEdge{{ID: "one", Source: "context", Target: "write"}, {ID: "two", Source: "write", Target: "output"}},
	}
	ordered := orderedWorkflowNodes(definition)
	if len(ordered) != 3 || ordered[0].ID != "context" || ordered[1].ID != "write" || ordered[2].ID != "output" {
		t.Fatalf("unexpected workflow execution order: %#v", ordered)
	}
}
