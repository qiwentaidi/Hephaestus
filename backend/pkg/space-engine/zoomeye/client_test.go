package zoomeye

import "testing"

func TestBuildAPIURL(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		endpoint string
		want     string
	}{
		{
			name:     "default address",
			endpoint: "v2/search",
			want:     "https://api.zoomeye.ai/v2/search",
		},
		{
			name:     "adds scheme",
			address:  "api.zoomeye.ai",
			endpoint: "/domain/search",
			want:     "https://api.zoomeye.ai/domain/search",
		},
		{
			name:     "trims duplicate slashes",
			address:  "https://api.zoomeye.org/",
			endpoint: "/v2/userinfo",
			want:     "https://api.zoomeye.org/v2/userinfo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildAPIURL(tt.address, tt.endpoint); got != tt.want {
				t.Fatalf("BuildAPIURL(%q, %q) = %q, want %q", tt.address, tt.endpoint, got, tt.want)
			}
		})
	}
}
