package httpclient

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/qiwentaidi/clients"
)

// New returns a client whose proxy is fixed for this scan. The scan gateway
// presents a per-task CA, while subdomain providers historically accept
// self-signed targets, so verification remains disabled at this client edge.
func New(proxy string, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if parsed, err := url.Parse(strings.TrimSpace(proxy)); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		transport.Proxy = http.ProxyURL(parsed)
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func Resty(proxy string, followRedirect bool) *resty.Client {
	return clients.NewRestyClientWithProxy(nil, followRedirect, strings.TrimSpace(proxy))
}
