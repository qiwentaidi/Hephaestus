package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/space/engine"
)

func pentestNativeSpaceFallbackTool() pentest.AgentToolSchema {
	return pentest.AgentToolSchema{Type: "function", Function: pentest.AgentToolFunction{
		Name:        "space_engine_native_search",
		Description: "仅用于 space_engine_search 聚合查询明确报错后的单引擎回退。必须引用当前会话的 aggregateExecutionId，只重试 engineStatus 中失败的引擎；成功的空结果不属于错误。保持原查询目标，按失败引擎的原生语法修正 query。",
		Parameters: pentestNativeObjectSchema(map[string]any{
			"aggregateExecutionId": map[string]any{"type": "string", "description": "先前聚合查询工具结果中的 execution_id。"},
			"engine":               map[string]any{"type": "string", "enum": []string{"fofa", "hunter", "quake", "censys", "zoomeye", "shodan"}},
			"query":                map[string]any{"type": "string", "description": "同一查询意图对应的单引擎原生语法。"},
			"page":                 map[string]any{"type": "integer", "default": 1},
			"pageSize":             map[string]any{"type": "integer", "default": 20, "maximum": 100},
		}, "aggregateExecutionId", "engine", "query"),
	}}
}

// The agent executor checks the referenced failure before dispatching here.
func runPentestSpaceFallback(ctx context.Context, call pentest.AgentToolCall) (any, bool, error) {
	var args struct {
		Engine   string `json:"engine"`
		Query    string `json:"query"`
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
	}
	if err := decodePentestToolArgs(call.Function.Arguments, &args); err != nil {
		return nil, true, err
	}
	names := normalizePentestSpaceEngines([]string{args.Engine})
	if len(names) != 1 || strings.TrimSpace(args.Query) == "" {
		return nil, true, fmt.Errorf("有效的 engine 和非空 query 必填")
	}
	if err := ctx.Err(); err != nil {
		return nil, true, err
	}
	option, err := buildPentestSpaceOption(names, 1)
	if err != nil {
		return nil, true, err
	}
	aggregator, err := engine.NewAggregatorFromSpaceOption(&option)
	if err != nil {
		return nil, true, err
	}
	selected := aggregator.GetEngine(names[0])
	if selected == nil || !selected.IsConfigured() {
		return nil, true, fmt.Errorf("%s 未配置或不可用", names[0])
	}
	if args.Page <= 0 {
		args.Page = 1
	}
	if args.PageSize <= 0 {
		args.PageSize = 20
	}
	if args.PageSize > 100 {
		args.PageSize = 100
	}
	resp := selected.Search(&engine.SearchRequest{Query: strings.TrimSpace(args.Query), Page: args.Page, PageSize: args.PageSize})
	if resp == nil {
		return nil, true, fmt.Errorf("%s 查询失败: 空响应", names[0])
	}
	if resp.Error != nil {
		return nil, true, fmt.Errorf("%s 查询失败: %w", names[0], resp.Error)
	}
	return gin.H{"engine": names[0], "query": args.Query, "page": args.Page, "pageSize": args.PageSize, "total": resp.Total, "results": resp.Results}, true, nil
}
