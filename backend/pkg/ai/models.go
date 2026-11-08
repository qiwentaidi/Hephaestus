package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"hephaestus-backend/pkg/config"
)

// ModelOption contains only public model metadata, never provider credentials.
type ModelOption struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type ModelListError struct {
	Code string
}

func (e *ModelListError) Error() string { return e.Code }

func modelListError(code string) error { return &ModelListError{Code: code} }

// ListModels uses the draft provider directly: discovery does not require a
// selected model, an enabled provider, or a saved configuration.
func ListModels(ctx context.Context, provider config.AIProviderConfig) ([]ModelOption, error) {
	protocol := strings.TrimSpace(provider.Protocol)
	if protocol != config.AIProtocolOpenAICompatible && protocol != config.AIProtocolAnthropic {
		return nil, modelListError("invalid_config")
	}
	base, err := validateAIBaseURL(provider.API)
	if err != nil || strings.TrimSpace(provider.Key) == "" {
		return nil, modelListError("invalid_config")
	}
	if base.RawQuery != "" || base.Fragment != "" {
		return nil, modelListError("invalid_config")
	}
	transport := newSafeTransport(true)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   testRequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// x-api-key is not stripped automatically on cross-origin redirects.
			if len(via) >= 3 || req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
				return fmt.Errorf("model endpoint redirect rejected")
			}
			_, err := validateAIBaseURL(req.URL.String())
			return err
		},
	}
	endpoints := modelListEndpoints(base)
	for index, endpoint := range endpoints {
		models, fallback, err := fetchModelPages(ctx, client, endpoint, protocol, strings.TrimSpace(provider.Key))
		if err == nil {
			return models, nil
		}
		if !fallback || index == len(endpoints)-1 {
			return nil, err
		}
	}
	return nil, modelListError("request_failed")
}

func modelListEndpoints(base *url.URL) []string {
	root := *base
	root.Path = strings.TrimRight(root.Path, "/")
	for _, suffix := range []string{"/chat/completions", "/messages", "/models"} {
		if strings.HasSuffix(root.Path, suffix) {
			root.Path = strings.TrimSuffix(root.Path, suffix)
			break
		}
	}
	root.RawPath = ""
	endpoints := []string{root.String() + "/models"}
	if shouldAddV1EndpointFallback(root.String()) {
		endpoints = append(endpoints, root.String()+"/v1/models")
	}
	return endpoints
}

func fetchModelPages(ctx context.Context, client *http.Client, endpoint, protocol, key string) ([]ModelOption, bool, error) {
	models := make([]ModelOption, 0)
	seen := make(map[string]bool)
	cursors := make(map[string]bool)
	u, _ := url.Parse(endpoint)
	for page := 0; page < 20; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, false, modelListError("invalid_config")
		}
		req.Header.Set("Accept", "application/json")
		if protocol == config.AIProtocolAnthropic {
			req.Header.Set("x-api-key", key)
			req.Header.Set("anthropic-version", anthropicVersion)
		} else {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, false, modelListError("timeout")
			}
			// Do not expose request URLs, credentials or arbitrary upstream bodies.
			return nil, false, modelListError("request_failed")
		}
		const maxResponseBytes = 4 << 20
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		_ = resp.Body.Close()
		if readErr != nil || len(body) > maxResponseBytes {
			return nil, false, modelListError("invalid_response")
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, false, modelListError("authentication_failed")
		case http.StatusTooManyRequests:
			return nil, false, modelListError("rate_limited")
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			return nil, page == 0, modelListError("unsupported")
		}
		if isLikelyNonJSONResponse(resp.Header.Get("Content-Type"), body) {
			return nil, page == 0, modelListError("invalid_response")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, false, modelListError("request_failed")
		}
		var result struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if json.Unmarshal(body, &result) != nil || result.Data == nil {
			return nil, false, modelListError("invalid_response")
		}
		for _, item := range result.Data {
			id := strings.TrimSpace(item.ID)
			if id != "" && !seen[id] {
				seen[id] = true
				models = append(models, ModelOption{ID: id, Name: strings.TrimSpace(item.DisplayName)})
			}
		}
		if !result.HasMore {
			sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
			return models, false, nil
		}
		if result.LastID == "" || cursors[result.LastID] {
			return nil, false, modelListError("invalid_response")
		}
		cursors[result.LastID] = true
		query := u.Query()
		query.Set("after_id", result.LastID)
		u.RawQuery = query.Encode()
	}
	return nil, false, modelListError("invalid_response")
}
