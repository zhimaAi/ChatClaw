package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type loggingHandler struct {
	*adk.BaseChatModelAgentMiddleware
	logger    *slog.Logger
	logPrompt bool
}

func newLoggingHandler(logger *slog.Logger, logPrompt bool) adk.ChatModelAgentMiddleware {
	return &loggingHandler{logger: logger, logPrompt: logPrompt}
}

func (h *loggingHandler) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	toolNames := make([]string, 0, len(runCtx.Tools))
	for _, t := range runCtx.Tools {
		info, _ := t.Info(ctx)
		if info != nil && info.Name != "" {
			toolNames = append(toolNames, info.Name)
		}
	}
	h.logger.Info("[agent] run started", "tools", toolNames)
	if h.logPrompt {
		//h.logger.Info("[agent] system_prompt", "instruction", runCtx.Instruction)
	}
	ctx = context.WithValue(ctx, ctxKeyAgentStart{}, time.Now())
	return ctx, runCtx, nil
}

func (h *loggingHandler) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	h.logger.Debug("[agent] model call", "messages", len(state.Messages), "tools", len(mc.Tools))
	return ctx, state, nil
}

func (h *loggingHandler) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if n := len(state.Messages); n > 0 {
		last := state.Messages[n-1]
		if last.Role == schema.Assistant {
			var toolCalls int
			for _, tc := range last.ToolCalls {
				if tc.Function.Name != "" {
					toolCalls++
				}
			}
			if toolCalls > 0 {
				h.logger.Info("[agent] model → tool_calls", "count", toolCalls)
			}
		}
	}
	return ctx, state, nil
}

func (h *loggingHandler) WrapInvokableToolCall(ctx context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		start := time.Now()
		result, err := endpoint(ctx, args, opts...)
		elapsed := time.Since(start)
		if err != nil {
			h.logger.Warn("[agent] tool failed", "tool", tCtx.Name, "call_id", tCtx.CallID, "elapsed", elapsed, "error", err)
		} else {
			h.logger.Info("[agent] tool done", "tool", tCtx.Name, "call_id", tCtx.CallID, "elapsed", elapsed, "result_len", len(result))
		}
		return result, err
	}, nil
}

type ctxKeyAgentStart struct{}
