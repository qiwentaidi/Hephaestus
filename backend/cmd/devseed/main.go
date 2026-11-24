package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"hephaestus-backend/lib/pentest"

	_ "github.com/lib/pq"
)

// Throwaway QA seeder: creates one clearly-labelled demo project so the project
// page can be exercised with realistic aggregates, then removes it again.
func main() {
	dsn := strings.TrimSpace(os.Getenv("HEPH_SEED_DSN"))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "HEPH_SEED_DSN is required (for example: host=127.0.0.1 port=5432 user=postgres dbname=hephaestus sslmode=disable)")
		os.Exit(2)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	ctx := context.Background()
	store := pentest.NewSQLStoreWithDialect(db, "postgres")

	if len(os.Args) > 2 && os.Args[1] == "cleanup" {
		if err := cleanup(ctx, store, os.Args[2]); err != nil {
			panic(err)
		}
		fmt.Println("cleaned")
		return
	}
	projectID, err := seed(ctx, store)
	if err != nil {
		panic(err)
	}
	fmt.Println(projectID)
}

func seed(ctx context.Context, store *pentest.SQLStore) (string, error) {
	project, err := store.UpsertProject(ctx, pentest.Project{
		Name:        "UI E2E 中转站（测试数据）",
		Description: "自动生成的端到端测试项目，验证后可删除",
		ScopeJSON:   pentest.ScopeJSON([]string{"tiger-sms.com", "supercloudsms.com", "receive-sms-free.cc", "sms.useaiplus.com", "untested.example"}),
		Status:      "active",
	})
	if err != nil {
		return "", err
	}
	newConv := func(title, target string) (pentest.Conversation, error) {
		return store.UpsertConversation(ctx, pentest.Conversation{
			ProjectID: project.ID, Title: title, Target: target, Mode: pentest.SoloPentestMode, Status: "completed",
		})
	}
	convA, err := newConv("短信接码平台 · Worker", "tiger-sms.com")
	if err != nil {
		return "", err
	}
	convB, err := newConv("后台入口排查 · Worker", "supercloudsms.com")
	if err != nil {
		return "", err
	}
	convC, err := newConv("API 文档探测 · Worker", "sms.useaiplus.com")
	if err != nil {
		return "", err
	}
	planner, err := newConv("中转站 · Planner", "tiger-sms.com")
	if err != nil {
		return "", err
	}

	addFact := func(convID, key, summary, category, confidence string) error {
		_, err := store.UpsertFact(ctx, pentest.Fact{
			ProjectID: project.ID, SourceConversationID: convID, FactKey: key,
			Summary: summary, Body: "E2E fixture body for " + key, Confidence: confidence, Category: category,
		}, nil)
		return err
	}
	facts := []struct{ conv, key, summary, category, confidence string }{
		{convA.ID, "e2e/login", "登录入口暴露且无失败锁定", "recon", pentest.ConfidenceConfirmed},
		{convA.ID, "e2e/api", "短信接收接口返回手机号与验证码字段", "recon", pentest.ConfidenceConfirmed},
		{convA.ID, "e2e/idor", "message_id 为自增，可遍历", "recon", pentest.ConfidenceConfirmed},
		{convA.ID, "blocker.waf", "WAF 拦截了目录爆破", "note", pentest.ConfidenceTentative},
		{convB.ID, "e2e/laravel", "Laravel 版本 8.4 与 /env 暴露", "recon", pentest.ConfidenceConfirmed},
		{convB.ID, "e2e/admin", "后台 /admin/login 未启用验证码", "recon", pentest.ConfidenceConfirmed},
		{convC.ID, "e2e/openapi", "/openapi.json 未授权可读", "recon", pentest.ConfidenceConfirmed},
		{convC.ID, "blocked.ratelimit", "接口限流导致遍历中断", "note", pentest.ConfidenceTentative},
	}
	for _, fact := range facts {
		if err := addFact(fact.conv, fact.key, fact.summary, fact.category, fact.confidence); err != nil {
			return "", err
		}
	}

	addFinding := func(convID, dedup, title, severity, status, target string) error {
		_, err := store.RecordFinding(ctx, pentest.Finding{
			ProjectID: project.ID, ConversationID: convID, DedupKey: dedup, Title: title,
			Severity: severity, Status: status, Target: target,
			Impact: "E2E fixture impact", Evidence: "HTTP 200 with cross-account data",
			ReproductionSteps: "1. 未带会话请求 /messages/981203\n2. 返回他人手机号与验证码",
		})
		return err
	}
	findings := []struct{ conv, dedup, title, severity, status, target string }{
		{convA.ID, "e2e-f1", "短信接收接口未鉴权，可遍历读取他人验证码", "critical", pentest.FindingStatusConfirmed, "https://tiger-sms.com/messages"},
		{convA.ID, "e2e-f2", "后台弱口令可枚举", "high", pentest.FindingStatusConfirmed, "https://tiger-sms.com/admin/login"},
		{convB.ID, "e2e-f3", "/env 暴露数据库凭据", "medium", pentest.FindingStatusOpen, "https://supercloudsms.com/.env"},
		{convC.ID, "e2e-f4", "API 文档暴露内部接口", "low", pentest.FindingStatusOpen, "https://sms.useaiplus.com/openapi.json"},
	}
	for _, finding := range findings {
		if err := addFinding(finding.conv, finding.dedup, finding.title, finding.severity, finding.status, finding.target); err != nil {
			return "", err
		}
	}

	run, err := store.CreatePlanRun(ctx, pentest.PlanRun{
		ProjectID: project.ID, PlannerConversationID: planner.ID,
		Objective: "对选中资产进行授权渗透测试：梳理攻击面并验证风险",
		ScopeJSON: pentest.ScopeJSON([]string{"tiger-sms.com", "supercloudsms.com", "receive-sms-free.cc", "sms.useaiplus.com"}),
		Status:    "planning", MaxWorkers: 4, PlannerModel: "baizhi-1", WorkerModel: "baizhi-2",
	})
	if err != nil {
		return "", err
	}
	if err := store.SetAgentRole(ctx, planner.ID, pentest.AgentRoleController, "", run.ID); err != nil {
		return "", err
	}
	for _, conv := range []pentest.Conversation{convA, convB, convC} {
		if err := store.SetAgentRole(ctx, conv.ID, pentest.AgentRoleWorker, "", run.ID); err != nil {
			return "", err
		}
	}

	targets := []string{"tiger-sms.com", "supercloudsms.com", "receive-sms-free.cc", "sms.useaiplus.com"}
	titles := []struct{ title, summary string }{
		{"短信接收站点的消息与号码查询接口 IDOR / 越权遍历", "已确认 1 个发现：未授权读取他人短信（严重）"},
		{"后台管理入口与认证机制探测", "后台登录缺少失败锁定，弱口令可枚举（高危）"},
		{"Laravel 组件版本指纹与已知 NVD 漏洞验证", "Laravel 8.4 命中 /env 暴露，RCE 未能验证"},
		{"敏感文件与备份路径探测", "60 余条路径中命中 /env、/.git/config"},
		{"API 文档与隐藏接口探测", "/openapi.json 未授权可读，暴露 12 个接口"},
	}
	for seq := 1; seq <= 24; seq++ {
		title := titles[(seq-1)%len(titles)]
		item, err := store.CreatePlanItem(ctx, pentest.PlanItem{
			RunID: run.ID, Seq: seq, BatchNo: (seq-1)/8 + 1,
			Title: fmt.Sprintf("%s（#%d）", title.title, seq), Instruction: "E2E fixture instruction",
			Target: targets[(seq-1)%len(targets)], Status: "pending",
		})
		if err != nil {
			return "", err
		}
		switch {
		case seq <= 14:
			if err := store.FinishPlanItem(ctx, item.ID, "completed", title.summary, ""); err != nil {
				return "", err
			}
		case seq <= 17:
			if err := store.FinishPlanItem(ctx, item.ID, "blocked", "", "模型不可用：主备渠道均返回 unexpected EOF，14:27 中断"); err != nil {
				return "", err
			}
		case seq <= 20:
			if err := store.FinishPlanItem(ctx, item.ID, "failed", "", "执行失败：目标连接超时（28s）"); err != nil {
				return "", err
			}
		case seq == 21:
			if _, err := store.ClaimPlanItemStart(ctx, item.ID, convC.ID); err != nil {
				return "", err
			}
		}
	}
	return project.ID, nil
}

func cleanup(ctx context.Context, store *pentest.SQLStore, projectID string) error {
	runs, err := store.ListPlanRunsPage(ctx, projectID, 100, 0)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if err := store.DeletePlanRun(ctx, run.ID); err != nil {
			return err
		}
	}
	return store.DeleteProject(ctx, projectID)
}
