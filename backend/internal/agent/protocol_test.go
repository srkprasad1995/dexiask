package agent

import (
	"encoding/json"
	"testing"
)

// TestEventUnmarshal_TypeNames verifies the Event wire shape maps the engine's
// NDJSON event fields onto the struct — the type names must survive round-trip
// unchanged because the backend re-frames them without translation.
func TestEventUnmarshal_TypeNames(t *testing.T) {
	cases := []struct {
		name string
		line string
		want Event
	}{
		{
			name: "text.delta",
			line: `{"type":"text.delta","text":"hello"}`,
			want: Event{Type: "text.delta", Text: "hello"},
		},
		{
			name: "tool.start",
			line: `{"type":"tool.start","name":"Read","id":"t1"}`,
			want: Event{Type: "tool.start", Name: "Read", ID: "t1"},
		},
		{
			name: "result carries sessionId",
			line: `{"type":"result","status":"ok","sessionId":"sess-123","model":"claude-x"}`,
			want: Event{Type: "result", Status: "ok", SessionID: "sess-123", Model: "claude-x"},
		},
		{
			name: "error",
			line: `{"type":"error","message":"boom"}`,
			want: Event{Type: "error", Message: "boom"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got Event
			if err := json.Unmarshal([]byte(tc.line), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Type != tc.want.Type || got.Text != tc.want.Text ||
				got.Name != tc.want.Name || got.ID != tc.want.ID ||
				got.Status != tc.want.Status || got.SessionID != tc.want.SessionID ||
				got.Model != tc.want.Model || got.Message != tc.want.Message {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestJobMarshal_FieldNames verifies the Job serializes to the protocol field
// names the engine expects.
func TestJobMarshal_FieldNames(t *testing.T) {
	job := Job{
		Role:           "ask",
		Model:          "claude-x",
		SystemPrompt:   "prompt",
		AllowedTools:   []string{"Read"},
		PermissionMode: "dontAsk",
		SkillsPath:     "/skills",
		WorkspacePath:  "/workspace/.dexiask",
		SessionID:      "sess-1",
		MCPServers:     []MCPServerConfig{{Name: "indexer", Type: "http", URL: "http://x/mcp"}},
	}
	b, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"role", "model", "systemPrompt", "allowedTools", "permissionMode", "skillsPath", "workspacePath", "sessionId", "mcpServers"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing wire field %q in %s", key, b)
		}
	}
}

func TestAllowedToolsForRole_Ask(t *testing.T) {
	tools := AllowedToolsForRole("ask")
	want := map[string]bool{"Read": true, "Glob": true, "Grep": true, "WebSearch": true, "WebFetch": true, "AskChoice": true}
	if len(tools) != len(want) {
		t.Fatalf("got %v", tools)
	}
	for _, tool := range tools {
		if !want[tool] {
			t.Errorf("unexpected tool %q", tool)
		}
	}
}

func TestWorkspacePaths(t *testing.T) {
	if got := WorkspacePath(); got != "/workspace/.dexiask" {
		t.Errorf("WorkspacePath = %q", got)
	}
	if got := SessionStorePathFor("conv-1"); got != "/workspace/.dexiask/conversations/conv-1/session" {
		t.Errorf("SessionStorePathFor = %q", got)
	}
	if got := SessionStorePathFor(""); got != "" {
		t.Errorf("SessionStorePathFor(empty) = %q, want empty", got)
	}
}

func TestSystemPromptForRole_AskNonEmpty(t *testing.T) {
	p := SystemPromptForRole("ask")
	if len(p) == 0 {
		t.Fatal("ask prompt is empty")
	}
}
