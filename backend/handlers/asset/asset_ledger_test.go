package asset

import "testing"

func TestParseImportContent(t *testing.T) {
	content := "type,value\ndomain,Example.COM\nip,192.0.2.10\nhttps://api.example.com\nhttp://\ndomain,example.com\n"
	rows, errorsOut, duplicates, _ := parseImportContent(content)
	if len(rows) != 3 {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if len(errorsOut) != 1 || errorsOut[0].Value != "http://" {
		t.Fatalf("unexpected errors: %#v", errorsOut)
	}
	if duplicates != 1 {
		t.Fatalf("unexpected duplicate count: %d", duplicates)
	}
	byValue := map[string]ledgerImportRow{}
	for _, row := range rows {
		byValue[row.Value] = row
	}
	if byValue["example.com"].Type != "domain" {
		t.Fatalf("domain row missing: %#v", byValue["example.com"])
	}
	if byValue["192.0.2.10"].Type != "ip" {
		t.Fatalf("ip row missing: %#v", byValue["192.0.2.10"])
	}
	if byValue["https://api.example.com"].Type != "url" {
		t.Fatalf("url row missing: %#v", byValue["https://api.example.com"])
	}
}

func TestParseImportContentSupportsPastedValues(t *testing.T) {
	content := "# assets\nexample.com\n192.0.2.10\nhttps://example.com/login\n\n192.0.2.10\n"
	rows, errorsOut, duplicates, _ := parseImportContent(content)
	if len(rows) != 3 || len(errorsOut) != 0 || duplicates != 1 {
		t.Fatalf("unexpected parse result: rows=%#v errors=%#v duplicates=%d", rows, errorsOut, duplicates)
	}
}

func TestNormalizeImportType(t *testing.T) {
	cases := map[string]string{
		"domain": "domain", "hostname": "domain", "域名": "domain",
		"ip": "ip", "IP": "ip", "ipv6": "ip",
		"url": "url", "链接": "url", "unknown": "",
	}
	for input, want := range cases {
		if got := normalizeImportType(input); got != want {
			t.Fatalf("normalizeImportType(%q) = %q, want %q", input, got, want)
		}
	}
}
