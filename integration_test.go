package claude

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestIntegration_Run drives a real `claude` CLI turn end to end. Gated
// behind CLAUDE_SDK_INTEGRATION=1 because it needs the CLI installed and
// authenticated; skipped by default in unit runs.
func TestIntegration_Run(t *testing.T) {
	if os.Getenv("CLAUDE_SDK_INTEGRATION") != "1" {
		t.Skip("set CLAUDE_SDK_INTEGRATION=1 to run against a real claude CLI")
	}
	c := New(Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Plan mode is read-only: the run must not mutate the working tree.
	ch, err := c.Run(ctx, RunOptions{
		Prompt:         "Reply with exactly: ok",
		Model:          "haiku",
		PermissionMode: PermissionModePlan,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	sawInit := false
	terminals := 0
	result := ""
	for ev := range ch {
		if ev.Type == EventError {
			t.Fatalf("unexpected error event: %s", ev.Text)
		}
		if ev.Type == EventSystem && ev.Subtype == SubtypeInit && ev.SessionID != "" {
			sawInit = true
		}
		if ev.Type == EventResult {
			terminals++
			result = ev.Result
		}
	}
	if !sawInit {
		t.Error("no system/init event with a session_id")
	}
	if terminals != 1 {
		t.Errorf("terminal events = %d, want exactly 1", terminals)
	}
	if strings.TrimSpace(result) == "" {
		t.Error("result text is empty")
	}
}
