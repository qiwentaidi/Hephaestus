package ai

import (
	"net/http"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/modelrequest"
)

func newModelRequestTransport(cfg config.AIConfig) http.RoundTripper {
	transport := newSafeTransport(true)
	// Generation may take minutes before response headers arrive. Connection and
	// TLS limits remain separate; the channel deadline bounds the full response.
	transport.ResponseHeaderTimeout = 0
	return modelrequest.ForProvider(transport, cfg.DefaultProviderID, cfg.API, cfg.Key, cfg.Model, cfg.RequestTimeoutMinutes)
}
