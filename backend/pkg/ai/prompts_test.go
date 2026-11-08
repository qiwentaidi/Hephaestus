package ai

import (
	"strings"
	"testing"
)

func TestWorkbenchSystemPromptExcludesCORSDetection(t *testing.T) {
	prompt := WorkbenchSystemPrompt()
	for _, want := range []string{"CORS", "不要提出、规划、选择或执行 CORS 漏洞检测"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q: %s", want, prompt)
		}
	}
}
