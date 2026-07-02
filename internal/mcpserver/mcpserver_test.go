package mcpserver

// Pins the registered tool surface to ToolNames. The doc comment on New once
// said "8 tools" while 11 were registered — a count can drift silently, a
// list-equality test cannot. Any tool added to (or removed from) New without
// updating ToolNames fails here, and vice versa.

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNew_RegistersExactlyToolNames(t *testing.T) {
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	srv := New()
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "tool-surface-test", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	res, err := clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}

	for _, want := range ToolNames {
		if !got[want] {
			t.Errorf("tool %q in ToolNames but not registered by New()", want)
		}
	}
	if len(res.Tools) != len(ToolNames) {
		var names []string
		for _, tool := range res.Tools {
			names = append(names, tool.Name)
		}
		t.Errorf("New() registers %d tools, ToolNames declares %d; registered: %v",
			len(res.Tools), len(ToolNames), names)
	}
}
