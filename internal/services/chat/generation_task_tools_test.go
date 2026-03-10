package chat

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func TestBuildExtrasIncludesScheduledTaskTools(t *testing.T) {
	svc := NewChatService(nil)
	svc.RegisterExtraToolFactory(func() ([]tool.BaseTool, error) {
		return []tool.BaseTool{
			&testTool{name: "scheduled_task_list"},
			&testTool{name: "agent_match_by_name"},
			&testTool{name: "scheduled_task_create_preview"},
			&testTool{name: "scheduled_task_create_confirm"},
			&testTool{name: "scheduled_task_delete"},
			&testTool{name: "scheduled_task_enable"},
			&testTool{name: "scheduled_task_disable"},
		}, nil
	})
	gc := &generationContext{
		service: svc,
	}

	extraTools, _ := svc.buildExtras(context.Background(), gc)
	names := toolNames(t, extraTools)

	for _, want := range []string{
		"scheduled_task_list",
		"agent_match_by_name",
		"scheduled_task_create_preview",
		"scheduled_task_create_confirm",
		"scheduled_task_delete",
		"scheduled_task_enable",
		"scheduled_task_disable",
	} {
		if !contains(names, want) {
			t.Fatalf("expected tool %q in extras, got %+v", want, names)
		}
	}
}

func toolNames(t *testing.T, tools []tool.BaseTool) []string {
	t.Helper()

	names := make([]string, 0, len(tools))
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatalf("tool Info returned error: %v", err)
		}
		names = append(names, info.Name)
	}
	return names
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

type testTool struct {
	name string
}

func (t *testTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
}

func (t *testTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return "{}", nil
}
