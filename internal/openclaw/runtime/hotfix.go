package openclawruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	openClawThinkingStreamPatchNeedle1 = `streamReasoning: reasoningMode === "stream" && typeof params.onReasoningStream === "function",`
	openClawThinkingStreamPatchValue1  = `streamReasoning: reasoningMode === "stream",`

	openClawThinkingStreamPatchNeedle2 = `if (!state.streamReasoning || !params.onReasoningStream) return;`
	openClawThinkingStreamPatchValue2  = `if (!state.streamReasoning) return;`

	openClawThinkingStreamPatchNeedle3 = `params.onReasoningStream({ text: formatted });`
	openClawThinkingStreamPatchValue3  = `if (params.onReasoningStream) params.onReasoningStream({ text: formatted });`

	openClawFallbackRetryPromptPatchNeedle = `return "Continue where you left off. The previous model attempt failed or timed out.";`
	openClawFallbackRetryPromptPatchValue  = `return params.body;/* chatclaw-hotfix: preserve original prompt on fallback */`
)

// applyBundledRuntimeHotfixes patches known upstream OpenClaw runtime issues in
// the resolved bundle. The hotfix is idempotent and only rewrites files when it
// finds the vulnerable code pattern.
func applyBundledRuntimeHotfixes(bundle *bundledRuntime) (int, error) {
	if bundle == nil || strings.TrimSpace(bundle.Root) == "" {
		return 0, nil
	}

	pattern := filepath.Join(
		bundle.Root,
		"lib",
		"node_modules",
		"openclaw",
		"dist",
		"auth-profiles-*.js",
	)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return 0, fmt.Errorf("glob runtime hotfix target: %w", err)
	}
	if len(files) == 0 {
		return 0, nil
	}

	patched := 0
	for _, path := range files {
		changed, err := applyOpenClawThinkingStreamHotfixFile(path)
		if err != nil {
			return patched, err
		}
		if changed {
			patched++
		}
	}
	return patched, nil
}

func applyOpenClawThinkingStreamHotfixFile(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read runtime hotfix target %s: %w", path, err)
	}
	content := string(raw)
	updated := content

	for _, patch := range []struct {
		needle string
		value  string
	}{
		{
			needle: openClawThinkingStreamPatchNeedle1,
			value:  openClawThinkingStreamPatchValue1,
		},
		{
			needle: openClawThinkingStreamPatchNeedle2,
			value:  openClawThinkingStreamPatchValue2,
		},
		{
			needle: openClawThinkingStreamPatchNeedle3,
			value:  openClawThinkingStreamPatchValue3,
		},
		{
			needle: openClawFallbackRetryPromptPatchNeedle,
			value:  openClawFallbackRetryPromptPatchValue,
		},
	} {
		if !strings.Contains(updated, patch.value) && strings.Contains(updated, patch.needle) {
			updated = strings.ReplaceAll(updated, patch.needle, patch.value)
		}
	}

	if updated == content {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return false, fmt.Errorf("write runtime hotfix target %s: %w", path, err)
	}
	return true, nil
}
