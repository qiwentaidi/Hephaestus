package ai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	"hephaestus-backend/lib/pentest/taskengine/agent"

	"github.com/gin-gonic/gin"
)

func nativePromptKey(role string) string {
	switch role {
	case "planner", "worker":
		return role
	case "controller":
		return "mainagent"
	}
	return ""
}
func loadARTEXPrompt(ctx context.Context, rt *taskengine.Runtime, role string) (pentest.AgentPrompt, error) {
	key := nativePromptKey(role)
	def := agent.BuiltinPromptSeeds()[key]
	item := pentest.AgentPrompt{Role: role, DefaultPrompt: def}
	err := rt.DB.QueryRowContext(ctx, `SELECT p.template_text,p.version,p.created_at FROM agents a JOIN agent_prompts p ON p.id=a.current_prompt_id WHERE a.key=$1`, key).Scan(&item.Prompt, &item.Version, &item.UpdatedAt)
	item.Customized = item.Prompt != def
	return item, err
}
func saveARTEXPrompt(c *gin.Context) {
	var req struct {
		Prompt  string `json:"prompt"`
		Version int    `json:"version"`
		Reset   bool   `json:"reset"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	if err := c.ShouldBindJSON(&req); err != nil {
		nativeError(c, 400, err)
		return
	}
	key := nativePromptKey(c.Param("role"))
	def := agent.BuiltinPromptSeeds()[key]
	if req.Reset {
		req.Prompt = def
	}
	if req.Version < 1 || strings.TrimSpace(req.Prompt) == "" || len(req.Prompt) > pentest.MaxAgentPromptBytes {
		c.JSON(400, gin.H{"success": false, "code": "invalid_prompt"})
		return
	}
	rt, err := taskengine.Get(c.Request.Context())
	if err != nil {
		nativeError(c, 503, err)
		return
	}
	item, err := updateARTEXPrompt(c.Request.Context(), rt, c.Param("role"), req.Prompt, req.Version)
	if errors.Is(err, pentest.ErrAgentPromptConflict) {
		c.JSON(409, gin.H{"success": false, "code": "version_conflict"})
		return
	}
	if err != nil {
		nativeError(c, 500, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": item})
}
func updateARTEXPrompt(ctx context.Context, rt *taskengine.Runtime, role, prompt string, expected int) (pentest.AgentPrompt, error) {
	tx, err := rt.DB.BeginTx(ctx, nil)
	if err != nil {
		return pentest.AgentPrompt{}, err
	}
	defer tx.Rollback()
	var id int64
	var current sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT id,current_prompt_id FROM agents WHERE key=$1 FOR UPDATE`, nativePromptKey(role)).Scan(&id, &current); err != nil {
		return pentest.AgentPrompt{}, err
	}
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT version FROM agent_prompts WHERE id=$1`, current).Scan(&version); err != nil {
		return pentest.AgentPrompt{}, err
	}
	if version != expected {
		return pentest.AgentPrompt{}, pentest.ErrAgentPromptConflict
	}
	var pid int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by) VALUES($1,$2,$3,'Hephaestus Agent 管理','operator') RETURNING id`, id, version+1, prompt).Scan(&pid); err != nil {
		return pentest.AgentPrompt{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agents SET current_prompt_id=$1 WHERE id=$2`, pid, id); err != nil {
		return pentest.AgentPrompt{}, err
	}
	if err = tx.Commit(); err != nil {
		return pentest.AgentPrompt{}, err
	}
	def := agent.BuiltinPromptSeeds()[nativePromptKey(role)]
	return pentest.AgentPrompt{Role: role, Prompt: prompt, Version: version + 1, DefaultPrompt: def, Customized: prompt != def, UpdatedAt: time.Now()}, nil
}
