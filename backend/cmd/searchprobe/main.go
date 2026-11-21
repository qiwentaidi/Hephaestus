// Command searchprobe runs one live web search through the agent search tool,
// using a stored LLM profile's endpoint and key. It lets an operator verify a
// channel (official endpoint or a gateway) without starting a task.
//
//	go run ./cmd/searchprobe -backend qwen-search -model qwen-plus
//	go run ./cmd/searchprobe -backend deepseek-search
//
// The API key is read from the profile or SEARCH_PROBE_KEY and is never printed.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	actool "github.com/Autumn-27/norma/tool"
)

func main() {
	var (
		dsn     = flag.String("dsn", env("HEPHAESTUS_PG_DSN", "postgres://postgres:123456@127.0.0.1:5432/hephaestus?sslmode=disable"), "postgres DSN of the host database")
		profile = flag.String("profile", "", "profile name; empty = the default profile")
		backend = flag.String("backend", "qwen-search", "ddgs | brave-free | tavily | deepseek-search | qwen-search | deepseek")
		base    = flag.String("base", "", "override the base URL")
		model   = flag.String("model", "", "override the model (qwen-search/deepseek)")
		key     = flag.String("key", "", "override the API key (prefer SEARCH_PROBE_KEY)")
		query   = flag.String("query", "DeepSeek 最新模型发布", "search query")
		limit   = flag.Int("limit", 3, "max results")
		list    = flag.Bool("list", false, "list stored profiles (base/model only) and exit")
		models  = flag.Bool("models", false, "list model ids exposed by the endpoint and exit")
		timeout = flag.Duration("timeout", 60*time.Second, "per-request timeout")
	)
	flag.Parse()
	if *list {
		if err := listProfiles(*dsn); err != nil {
			fail("list profiles: %v", err)
		}
		return
	}
	if *key == "" {
		*key = os.Getenv("SEARCH_PROBE_KEY")
	}

	baseURL, apiKey, modelName := *base, *key, *model
	if baseURL == "" || apiKey == "" || modelName == "" {
		b, k, m, err := loadProfile(*dsn, *profile)
		if err != nil {
			fail("read profile: %v", err)
		}
		if baseURL == "" {
			baseURL = b
		}
		if apiKey == "" {
			apiKey = k
		}
		if modelName == "" {
			modelName = m
		}
	}
	if baseURL == "" || apiKey == "" {
		fail("no endpoint/key: pass -base/-key or configure a profile")
	}
	if *models {
		if err := listModels(baseURL, apiKey); err != nil {
			fail("list models: %v", err)
		}
		return
	}

	cfg := actool.WebSearchConfig{
		Backend:         *backend,
		QwenBaseURL:     baseURL,
		QwenAPIKey:      apiKey,
		QwenModel:       modelName,
		DeepSeekBaseURL: baseURL,
		DeepSeekAPIKey:  apiKey,
		DeepSeekModel:   modelName,
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	results, err := actool.WebSearchProbe(ctx, cfg, *query, *limit)
	if err != nil {
		fail("backend=%s base=%s model=%s: %v", *backend, baseURL, modelName, err)
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	fmt.Printf("OK backend=%s base=%s model=%s results=%d\n%s\n", *backend, baseURL, modelName, len(results), out)
}

// loadProfile reads endpoint/key/model from llm_profiles, wherever its schema is.
func loadProfile(dsn, name string) (base, key, model string, err error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return "", "", "", err
	}
	defer db.Close()
	var schema string
	if err := db.QueryRow(`SELECT table_schema FROM information_schema.tables
		WHERE table_name = 'llm_profiles' ORDER BY (table_schema = 'public') DESC LIMIT 1`).Scan(&schema); err != nil {
		return "", "", "", fmt.Errorf("locate llm_profiles: %w", err)
	}
	q := fmt.Sprintf(`SELECT COALESCE(base_url,''), COALESCE(api_key,''), COALESCE(model,'') FROM %q.llm_profiles `, schema)
	args := []any{}
	if name != "" {
		q += "WHERE name = $1 LIMIT 1"
		args = append(args, name)
	} else {
		q += "ORDER BY is_default DESC, id DESC LIMIT 1"
	}
	err = db.QueryRow(q, args...).Scan(&base, &key, &model)
	if err == sql.ErrNoRows {
		return "", "", "", fmt.Errorf("no usable profile")
	}
	return base, key, model, err
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL "+format+"\n", args...)
	os.Exit(1)
}

// listProfiles prints every stored profile without its key.
func listProfiles(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT table_schema FROM information_schema.tables
		WHERE table_name = 'llm_profiles' ORDER BY (table_schema = 'public') DESC`)
	if err != nil {
		return err
	}
	var schemas []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		schemas = append(schemas, s)
	}
	rows.Close()
	for _, schema := range schemas {
		q := fmt.Sprintf(`SELECT COALESCE(name,''), COALESCE(base_url,''), COALESCE(model,''), is_default FROM %q.llm_profiles ORDER BY is_default DESC, id`, schema)
		r, err := db.Query(q)
		if err != nil {
			continue
		}
		fmt.Printf("schema %s:\n", schema)
		for r.Next() {
			var name, base, model string
			var isDefault bool
			if err := r.Scan(&name, &base, &model, &isDefault); err != nil {
				continue
			}
			fmt.Printf("  %-16s default=%-5v %s  model=%s\n", name, isDefault, base, model)
		}
		r.Close()
	}
	return nil
}

// listModels prints the model ids an OpenAI-compatible endpoint exposes.
func listModels(base, key string) error {
	u := strings.TrimRight(base, "/")
	u = strings.TrimSuffix(u, "/chat/completions")
	if !strings.HasSuffix(u, "/v1") {
		u += "/v1"
	}
	u += "/models"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("parse models: %w", err)
	}
	for _, m := range payload.Data {
		if strings.Contains(strings.ToLower(m.ID), "qwen") {
			fmt.Println(m.ID)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
