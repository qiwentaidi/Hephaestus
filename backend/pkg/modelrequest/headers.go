package modelrequest

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"hephaestus-backend/pkg/config"
)

const openCodeSessionHeader = "x-opencode-session"

// ApplyProviderHeaders adds a channel's custom headers to one model request.
// Header values come from the same settings store as the API key and must never
// be logged.
//
// Requests replayed to a different host by an HTTP redirect are stripped:
// Authorization has the net/http redirect rules, but custom gateway secrets
// would otherwise leak to the redirect target.
func ApplyProviderHeaders(req *http.Request, headers []config.AIProviderHeader, providerID, baseURL, sessionID string) {
	if req == nil || req.URL == nil {
		return
	}
	if !sameRequestHost(req.URL, baseURL) {
		// A redirect to another host must not replay gateway credentials that
		// net/http copied from the original request.
		for _, header := range headers {
			if strings.TrimSpace(header.Name) != "" {
				req.Header.Del(header.Name)
			}
		}
		req.Header.Del(openCodeSessionHeader)
		return
	}
	for _, header := range headers {
		if strings.TrimSpace(header.Name) == "" || header.Value == "" {
			continue
		}
		req.Header.Set(header.Name, header.Value)
	}
	if !config.IsOpenCodeAPI(req.URL.String()) {
		return
	}
	if req.Header.Get(openCodeSessionHeader) != "" {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = providerSessionID(providerID, baseURL)
	}
	req.Header.Set(openCodeSessionHeader, sessionID)
}

func sameRequestHost(requestURL *url.URL, baseURL string) bool {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return true
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Hostname() == "" || requestURL.Hostname() == "" {
		return true
	}
	return strings.EqualFold(base.Hostname(), requestURL.Hostname())
}

// providerSessionID is stable across restarts for one channel. OpenCode uses the
// header for routing and prompt caching; a per-process random value would lose
// that cache affinity, so derive it from the provider identity instead.
func providerSessionID(providerID, baseURL string) string {
	seed := strings.TrimSpace(providerID)
	if seed == "" {
		seed = strings.TrimSpace(baseURL)
	}
	sum := sha256.Sum256([]byte("hephaestus/opencode-session\x00" + seed))
	return "ses_" + hex.EncodeToString(sum[:12])
}
