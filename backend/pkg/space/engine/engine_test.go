package engine

import (
	"testing"
)

func TestTranspiler_SimpleQueries(t *testing.T) {
	transpiler := NewTranspiler()

	// FOFA IP query
	result := transpiler.TranslateQuery("1.1.1.1", "fofa", QueryTypeIP)
	if result != `ip="1.1.1.1"` {
		t.Errorf("FOFA IP query: got %v, want ip=\"1.1.1.1\"", result)
	}

	// Hunter IP query
	result = transpiler.TranslateQuery("1.1.1.1", "hunter", QueryTypeIP)
	if result != `ip="1.1.1.1"` {
		t.Errorf("Hunter IP query: got %v, want ip=\"1.1.1.1\"", result)
	}

	// Quake IP query
	result = transpiler.TranslateQuery("1.1.1.1", "quake", QueryTypeIP)
	if result != `ip:"1.1.1.1"` {
		t.Errorf("Quake IP query: got %v, want ip:\"1.1.1.1\"", result)
	}

	// Censys IP query
	result = transpiler.TranslateQuery("1.1.1.1", "censys", QueryTypeIP)
	if result != `host.ip=1.1.1.1` {
		t.Errorf("Censys IP query: got %v, want host.ip=1.1.1.1", result)
	}

	// FOFA domain query
	result = transpiler.TranslateQuery("example.com", "fofa", QueryTypeDomain)
	if result != `domain="example.com"` {
		t.Errorf("FOFA domain query: got %v, want domain=\"example.com\"", result)
	}

	// Hunter domain query
	result = transpiler.TranslateQuery("example.com", "hunter", QueryTypeDomain)
	if result != `domain.suffix="example.com"` {
		t.Errorf("Hunter domain query: got %v, want domain.suffix=\"example.com\"", result)
	}

	// Hunter title query
	result = transpiler.TranslateQuery("登录", "hunter", QueryTypeTitle)
	if result != `web.title="登录"` {
		t.Errorf("Hunter title query: got %v, want web.title=\"登录\"", result)
	}
}

func TestTranspiler_ComplexQueries(t *testing.T) {
	transpiler := NewTranspiler()

	// FOFA complex query with AND
	result := transpiler.TranslateQuery("ip:1.1.1.1 AND port:80", "fofa", QueryTypeDefault)
	expected := `ip="1.1.1.1" && port="80"`
	if result != expected {
		t.Errorf("FOFA complex query: got %v, want %v", result, expected)
	}

	// Quake complex query
	result = transpiler.TranslateQuery("ip:1.1.1.1 AND port:443", "quake", QueryTypeDefault)
	expected = `ip:"1.1.1.1" AND port:443`
	if result != expected {
		t.Errorf("Quake complex query: got %v, want %v", result, expected)
	}

	// Hunter complex domain query
	result = transpiler.TranslateQuery(`domain:example.com OR domain:test.com`, "hunter", QueryTypeDefault)
	expected = `domain.suffix="example.com" || domain.suffix="test.com"`
	if result != expected {
		t.Errorf("Hunter complex domain query: got %v, want %v", result, expected)
	}
}

func TestTranspiler_ICPQueries(t *testing.T) {
	transpiler := NewTranspiler()

	// FOFA ICP number query
	result := transpiler.TranslateQuery("icp:京ICP备12345678号", "fofa", QueryTypeDefault)
	expected := `icp="京ICP备12345678号"`
	if result != expected {
		t.Errorf("FOFA ICP query: got %v, want %v", result, expected)
	}

	// FOFA ICP number explicit query
	result = transpiler.TranslateQuery("icp.number:京ICP备12345678号", "fofa", QueryTypeDefault)
	expected = `icp="京ICP备12345678号"`
	if result != expected {
		t.Errorf("FOFA ICP number query: got %v, want %v", result, expected)
	}

	// Hunter ICP number query
	result = transpiler.TranslateQuery("icp.number:京ICP备12345678号", "hunter", QueryTypeDefault)
	expected = `icp.number="京ICP备12345678号"`
	if result != expected {
		t.Errorf("Hunter ICP number query: got %v, want %v", result, expected)
	}

	// Hunter ICP name query
	result = transpiler.TranslateQuery("icp.name:百度", "hunter", QueryTypeDefault)
	expected = `icp.name="百度"`
	if result != expected {
		t.Errorf("Hunter ICP name query: got %v, want %v", result, expected)
	}

	// Quake ICP name query (uses icp_keywords)
	result = transpiler.TranslateQuery("icp.name:百度", "quake", QueryTypeDefault)
	expected = `icp_keywords:"百度"`
	if result != expected {
		t.Errorf("Quake ICP name query: got %v, want %v", result, expected)
	}
}

func TestTranspiler_ExtendedQueries(t *testing.T) {
	transpiler := NewTranspiler()

	// FOFA cert query
	result := transpiler.TranslateQuery("cert.issuer.org:DigiCert", "fofa", QueryTypeDefault)
	expected := `cert.issuer.org="DigiCert"`
	if result != expected {
		t.Errorf("FOFA cert issuer query: got %v, want %v", result, expected)
	}

	// FOFA OS query
	result = transpiler.TranslateQuery("os:Windows", "fofa", QueryTypeDefault)
	expected = `os="Windows"`
	if result != expected {
		t.Errorf("FOFA OS query: got %v, want %v", result, expected)
	}

	// Hunter OS query (uses ip.os)
	result = transpiler.TranslateQuery("os:Windows", "hunter", QueryTypeDefault)
	expected = `ip.os="Windows"`
	if result != expected {
		t.Errorf("Hunter OS query: got %v, want %v", result, expected)
	}

	// Hunter port query (uses ip.port)
	result = transpiler.TranslateQuery("port:80", "hunter", QueryTypeDefault)
	expected = `ip.port="80"`
	if result != expected {
		t.Errorf("Hunter port query: got %v, want %v", result, expected)
	}

	// Hunter banner query (uses protocol.banner)
	result = transpiler.TranslateQuery("banner:nginx", "hunter", QueryTypeDefault)
	expected = `protocol.banner="nginx"`
	if result != expected {
		t.Errorf("Hunter banner query: got %v, want %v", result, expected)
	}

	// Quake jarm query
	result = transpiler.TranslateQuery("jarm:abc123", "quake", QueryTypeDefault)
	expected = `jarm:"abc123"`
	if result != expected {
		t.Errorf("Quake jarm query: got %v, want %v", result, expected)
	}

	// Quake transport query
	result = transpiler.TranslateQuery("transport:udp", "quake", QueryTypeDefault)
	expected = `transport:"udp"`
	if result != expected {
		t.Errorf("Quake transport query: got %v, want %v", result, expected)
	}

	// Quake province_cn query
	result = transpiler.TranslateQuery("province_cn:四川", "quake", QueryTypeDefault)
	expected = `province_cn:"四川"`
	if result != expected {
		t.Errorf("Quake province_cn query: got %v, want %v", result, expected)
	}

	// Hunter region query
	result = transpiler.TranslateQuery("region:北京", "hunter", QueryTypeDefault)
	expected = `ip.province="北京"`
	if result != expected {
		t.Errorf("Hunter region query: got %v, want %v", result, expected)
	}

	// Censys ASN query
	result = transpiler.TranslateQuery("asn:12345", "censys", QueryTypeDefault)
	expected = `host.autonomous_system.asn=12345`
	if result != expected {
		t.Errorf("Censys ASN query: got %v, want %v", result, expected)
	}
}

func TestTranspiler_FingerprintQueries(t *testing.T) {
	transpiler := NewTranspiler()

	tests := []struct {
		name      string
		query     string
		engine    string
		queryType QueryType
		expected  string
	}{
		{
			name:      "FOFA app simple query",
			query:     "Apache",
			engine:    "fofa",
			queryType: QueryTypeApp,
			expected:  `app="Apache"`,
		},
		{
			name:      "FOFA product simple query",
			query:     "NGINX",
			engine:    "fofa",
			queryType: QueryTypeProduct,
			expected:  `product="NGINX"`,
		},
		{
			name:      "Hunter product simple query uses app.name",
			query:     "Apache Tomcat",
			engine:    "hunter",
			queryType: QueryTypeProduct,
			expected:  `app.name="Apache Tomcat"`,
		},
		{
			name:      "Quake product simple query uses app",
			query:     "Apache Tomcat",
			engine:    "quake",
			queryType: QueryTypeProduct,
			expected:  `app:"Apache Tomcat"`,
		},
		{
			name:      "Censys app simple query uses software product",
			query:     "nginx",
			engine:    "censys",
			queryType: QueryTypeApp,
			expected:  `host.services.software.product="nginx"`,
		},
		{
			name:      "ZoomEye product simple query uses app",
			query:     "Apache httpd",
			engine:    "zoomeye",
			queryType: QueryTypeProduct,
			expected:  `app="Apache httpd"`,
		},
		{
			name:      "FOFA product complex query",
			query:     `product:"NGINX" AND app:"Apache"`,
			engine:    "fofa",
			queryType: QueryTypeDefault,
			expected:  `product="NGINX" && app="Apache"`,
		},
		{
			name:      "Hunter product complex query",
			query:     `product:"Apache Tomcat" OR app:"Jenkins"`,
			engine:    "hunter",
			queryType: QueryTypeDefault,
			expected:  `app.name="Apache Tomcat" || app.name="Jenkins"`,
		},
		{
			name:      "Quake product complex query",
			query:     `product:"Apache Tomcat" AND app:"Jenkins"`,
			engine:    "quake",
			queryType: QueryTypeDefault,
			expected:  `app:"Apache Tomcat" AND app:"Jenkins"`,
		},
		{
			name:      "Censys product complex query",
			query:     `product:"nginx" AND app:"OpenSSH"`,
			engine:    "censys",
			queryType: QueryTypeDefault,
			expected:  `host.services.software.product="nginx" and host.services.software.product="OpenSSH"`,
		},
		{
			name:      "ZoomEye product complex query",
			query:     `product:"Apache httpd" AND app:"OpenSSL"`,
			engine:    "zoomeye",
			queryType: QueryTypeDefault,
			expected:  `app="Apache httpd"  AND  app="OpenSSL"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transpiler.TranslateQuery(tt.query, tt.engine, tt.queryType)
			if result != tt.expected {
				t.Errorf("TranslateQuery() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTranspiler_DetectQueryType(t *testing.T) {
	transpiler := NewTranspiler()

	// IP address
	if transpiler.DetectQueryType("1.1.1.1") != QueryTypeIP {
		t.Error("Should detect IP address")
	}

	// IP with CIDR
	if transpiler.DetectQueryType("192.168.1.0/24") != QueryTypeIP {
		t.Error("Should detect IP with CIDR")
	}

	// Domain
	if transpiler.DetectQueryType("example.com") != QueryTypeDomain {
		t.Error("Should detect domain")
	}

	// Complex query
	if transpiler.DetectQueryType("ip:1.1.1.1 AND port:80") != QueryTypeDefault {
		t.Error("Should detect complex query as default")
	}
}

func TestQueryTypeFromString(t *testing.T) {
	if QueryTypeFromString("ip") != QueryTypeIP {
		t.Error("Should convert 'ip' to QueryTypeIP")
	}
	if QueryTypeFromString("IP") != QueryTypeIP {
		t.Error("Should convert 'IP' to QueryTypeIP")
	}
	if QueryTypeFromString("domain") != QueryTypeDomain {
		t.Error("Should convert 'domain' to QueryTypeDomain")
	}
	if QueryTypeFromString("icp.number") != QueryTypeICPNumber {
		t.Error("Should convert 'icp.number' to QueryTypeICPNumber")
	}
	if QueryTypeFromString("icp.name") != QueryTypeICPName {
		t.Error("Should convert 'icp.name' to QueryTypeICPName")
	}
	if QueryTypeFromString("app") != QueryTypeApp {
		t.Error("Should convert 'app' to QueryTypeApp")
	}
	if QueryTypeFromString("product") != QueryTypeProduct {
		t.Error("Should convert 'product' to QueryTypeProduct")
	}
	if QueryTypeFromString("unknown") != QueryTypeDefault {
		t.Error("Should convert unknown to QueryTypeDefault")
	}
}

func TestConvertLegacyQueryType(t *testing.T) {
	tests := []struct {
		legacy   string
		expected QueryType
	}{
		{"标题", QueryTypeTitle},
		{"备案号", QueryTypeICPNumber},
		{"备案名称", QueryTypeICPName},
		{"域名", QueryTypeDomain},
		{"IP", QueryTypeIP},
		{"Body", QueryTypeBody},
		{"应用", QueryTypeApp},
		{"产品", QueryTypeProduct},
		{"unknown", QueryTypeDefault},
	}

	for _, tt := range tests {
		result := convertLegacyQueryType(tt.legacy)
		if result != tt.expected {
			t.Errorf("convertLegacyQueryType(%s) = %v, want %v", tt.legacy, result, tt.expected)
		}
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	// Register factories
	registry.RegisterFactory("fofa", FofaEngineFactory)
	registry.RegisterFactory("hunter", HunterEngineFactory)

	// Create engine without config
	engine, err := registry.CreateEngine("fofa", nil)
	if err != nil {
		t.Errorf("CreateEngine() error = %v", err)
	}
	if engine == nil {
		t.Error("CreateEngine() returned nil")
	}

	// Verify engine name
	if engine.Name() != "fofa" {
		t.Errorf("Engine.Name() = %v, want fofa", engine.Name())
	}

	// Verify unconfigured state
	if engine.IsConfigured() {
		t.Error("Engine should not be configured")
	}

	// Get engine
	gotEngine := registry.GetEngine("fofa")
	if gotEngine != engine {
		t.Error("GetEngine() returned different engine")
	}

	// Get nonexistent engine
	if registry.GetEngine("nonexistent") != nil {
		t.Error("Should return nil for nonexistent engine")
	}
}

func TestFofaAdapter(t *testing.T) {
	// Test without config
	adapter, err := NewFofaAdapter(nil)
	if err != nil {
		t.Errorf("NewFofaAdapter() error = %v", err)
	}
	if adapter == nil {
		t.Error("NewFofaAdapter() returned nil")
	}
	if adapter.Name() != "fofa" {
		t.Errorf("Name() = %v, want fofa", adapter.Name())
	}
	if adapter.IsConfigured() {
		t.Error("Should not be configured")
	}

	// Test supported query types
	types := adapter.SupportedQueryTypes()
	if len(types) == 0 {
		t.Error("SupportedQueryTypes() should not be empty")
	}

	// Test with config
	config := &FofaConfig{
		Email: "test@example.com",
		Key:   "test-key",
	}
	configuredAdapter, _ := NewFofaAdapter(config)
	if !configuredAdapter.IsConfigured() {
		t.Error("Should be configured")
	}
}

func TestHunterAdapter(t *testing.T) {
	adapter, err := NewHunterAdapter(nil)
	if err != nil {
		t.Errorf("NewHunterAdapter() error = %v", err)
	}
	if adapter.Name() != "hunter" {
		t.Errorf("Name() = %v, want hunter", adapter.Name())
	}
	if adapter.IsConfigured() {
		t.Error("Should not be configured")
	}
}

func TestQuakeAdapter(t *testing.T) {
	adapter, err := NewQuakeAdapter(nil)
	if err != nil {
		t.Errorf("NewQuakeAdapter() error = %v", err)
	}
	if adapter.Name() != "quake" {
		t.Errorf("Name() = %v, want quake", adapter.Name())
	}
	if adapter.IsConfigured() {
		t.Error("Should not be configured")
	}
}

func TestCensysAdapter(t *testing.T) {
	adapter, err := NewCensysAdapter(nil)
	if err != nil {
		t.Errorf("NewCensysAdapter() error = %v", err)
	}
	if adapter.Name() != "censys" {
		t.Errorf("Name() = %v, want censys", adapter.Name())
	}
	if adapter.IsConfigured() {
		t.Error("Should not be configured")
	}
}

func TestUnifiedResult(t *testing.T) {
	result := UnifiedResult{
		URL:        "https://example.com",
		IP:         "1.1.1.1",
		Port:       443,
		Protocol:   "https",
		Domain:     "example.com",
		Title:      "Example",
		Components: []string{"nginx/1.20", "php/8.0"},
		Source:     "fofa",
	}

	if result.URL != "https://example.com" {
		t.Errorf("URL = %v, want https://example.com", result.URL)
	}
	if result.Port != 443 {
		t.Errorf("Port = %v, want 443", result.Port)
	}
	if len(result.Components) != 2 {
		t.Errorf("Components length = %v, want 2", len(result.Components))
	}
}

func TestUnifiedToSpaceResultKeepsICP(t *testing.T) {
	result := UnifiedToSpaceResult(UnifiedResult{
		URL:     "https://example.com",
		IP:      "1.1.1.1",
		Port:    443,
		ICP:     "京ICP备12345678号",
		ICPName: "示例公司",
		Source:  "quake",
	})

	if result.ICP != "京ICP备12345678号" {
		t.Errorf("ICP = %q, want 京ICP备12345678号", result.ICP)
	}
	if result.ICPName != "示例公司" {
		t.Errorf("ICPName = %q, want 示例公司", result.ICPName)
	}

	roundTrip := SpaceResultToUnified(result)
	if roundTrip.ICP != result.ICP || roundTrip.ICPName != result.ICPName {
		t.Errorf("round trip lost ICP facts: %#v", roundTrip)
	}
}

func TestParseRegion(t *testing.T) {
	country, province, city := parseRegion("中国/北京/北京")
	if country != "中国" || province != "北京" || city != "北京" {
		t.Error("Should parse full region string")
	}

	country, province, city = parseRegion("美国/加利福尼亚")
	if country != "美国" || province != "加利福尼亚" || city != "" {
		t.Error("Should parse partial region string")
	}

	country, province, city = parseRegion("中国")
	if country != "中国" || province != "" || city != "" {
		t.Error("Should parse country only")
	}

	country, province, city = parseRegion("")
	if country != "" || province != "" || city != "" {
		t.Error("Should handle empty string")
	}
}
