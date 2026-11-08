package fofa

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestBuildSearchURLDefaultOmitsOptionalFilters(t *testing.T) {
	client := NewFofaClient(&types.FofaAuth{
		Address: "https://fofa.info/",
		Email:   "user@example.com",
		Key:     "secret",
	})

	search := `ip="1.1.1.1" || ip="8.8.8.8"`
	rawURL := client.buildSearchURL(search, "10", "1", false, false)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "fofa.info" || parsed.Path != "/api/v1/search/all" {
		t.Fatalf("unexpected API URL: %s", rawURL)
	}

	query := parsed.Query()
	if got := query.Get("qbase64"); got != FOFABaseEncode(search) {
		t.Fatalf("qbase64 mismatch: got %q, want %q", got, FOFABaseEncode(search))
	}
	if _, ok := query["cert.is_valid"]; ok {
		t.Fatalf("cert.is_valid should be omitted by default")
	}
	if _, ok := query["is_fraud"]; ok {
		t.Fatalf("is_fraud should be omitted by default")
	}
	if _, ok := query["is_honeypot"]; ok {
		t.Fatalf("is_honeypot should be omitted by default")
	}
}

func TestBuildSearchURLAppliesRequestedFilters(t *testing.T) {
	client := NewFofaClient(&types.FofaAuth{
		Address: "https://fofa.info",
		Email:   "user@example.com",
		Key:     "secret",
	})

	rawURL := client.buildSearchURL(`ip="1.1.1.1"`, "10", "1", true, true)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("cert.is_valid"); got != "true" {
		t.Fatalf("cert.is_valid mismatch: got %q, want true", got)
	}
	if got := query.Get("is_fraud"); got != "false" {
		t.Fatalf("is_fraud mismatch: got %q, want false", got)
	}
	if got := query.Get("is_honeypot"); got != "false" {
		t.Fatalf("is_honeypot mismatch: got %q, want false", got)
	}
}

func TestSplitSimpleOrQueryChunksLongIPQuery(t *testing.T) {
	clauses := make([]string, 45)
	for i := range clauses {
		clauses[i] = `ip="192.0.2.` + strconv.Itoa(i+1) + `"`
	}

	chunks, ok := splitSimpleOrQuery(strings.Join(clauses, " || "), 20)
	if !ok {
		t.Fatalf("expected long simple OR query to be chunked")
	}
	if len(chunks) != 3 {
		t.Fatalf("chunk count mismatch: got %d, want 3", len(chunks))
	}

	gotCounts := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		gotCounts = append(gotCounts, strings.Count(chunk, "||")+1)
	}
	wantCounts := []int{20, 20, 5}
	for i := range wantCounts {
		if gotCounts[i] != wantCounts[i] {
			t.Fatalf("chunk %d clause count mismatch: got %d, want %d", i, gotCounts[i], wantCounts[i])
		}
	}
}

func TestSplitSimpleOrQueryOnlyHandlesSafeBatchQueries(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "short query",
			query: `ip="192.0.2.1" || ip="192.0.2.2"`,
		},
		{
			name:  "mixed boolean query",
			query: `ip="192.0.2.1" || ip="192.0.2.2" && port="443"`,
		},
		{
			name:  "parenthesized query",
			query: `(ip="192.0.2.1" || ip="192.0.2.2")`,
		},
		{
			name:  "non batch field",
			query: strings.Repeat(`product="nginx" || `, 21) + `product="apache"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if chunks, ok := splitSimpleOrQuery(tt.query, 20); ok {
				t.Fatalf("expected query not to be chunked, got %v", chunks)
			}
		})
	}
}
