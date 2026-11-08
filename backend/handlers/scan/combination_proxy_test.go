package scan

import "testing"

func TestNormalizeCombinationProxyURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty means direct", input: "", want: ""},
		{name: "http", input: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "http with credentials", input: "http://user:pass@proxy.example:8080", want: "http://user:pass@proxy.example:8080"},
		{name: "socks5", input: " socks5://127.0.0.1:1080 ", want: "socks5://127.0.0.1:1080"},
		{name: "unsupported scheme", input: "ftp://127.0.0.1:21", wantErr: true},
		{name: "missing host", input: "http://", wantErr: true},
		{name: "relative address", input: "127.0.0.1:8080", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCombinationProxyURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestInjectCombinationProxy(t *testing.T) {
	const proxy = "http://127.0.0.1:8080"
	nodes := []combinationPipelineNode{
		{ModuleID: "subdomain"},
		{ModuleID: "portscan", Config: map[string]interface{}{"threads": float64(1000)}},
		{ModuleID: "webscan", Config: map[string]interface{}{"proxy": "http://node-proxy:3128"}},
		{ModuleID: "jsscan", Config: map[string]interface{}{"proxy": ""}},
	}

	injectCombinationProxy(nodes, proxy)

	if got := nodes[0].Config["proxy"]; got != proxy {
		t.Fatalf("subdomain proxy = %v, want %q", got, proxy)
	}
	if got := nodes[1].Config["proxy"]; got != proxy {
		t.Fatalf("portscan proxy = %v, want %q", got, proxy)
	}
	if got := nodes[1].Config["threads"]; got != float64(1000) {
		t.Fatalf("portscan config was modified: %v", nodes[1].Config)
	}
	if got := nodes[2].Config["proxy"]; got != "http://node-proxy:3128" {
		t.Fatalf("node-level proxy was overwritten: %v", got)
	}
	if got := nodes[3].Config["proxy"]; got != "" {
		t.Fatalf("explicit direct access was overwritten: %v", got)
	}

	direct := []combinationPipelineNode{{ModuleID: "webscan"}}
	injectCombinationProxy(direct, "")
	if _, exists := direct[0].Config["proxy"]; exists {
		t.Fatalf("empty task proxy should not be injected")
	}
}
