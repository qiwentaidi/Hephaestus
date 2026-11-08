package scan

import "testing"

func TestParseVulnerabilityFilterExpressionSupportsOrGroups(t *testing.T) {
	expr, err := parseVulnerabilityFilterExpression("vuln.level=high OR vuln.method=POST AND response.type~json")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(expr.OrGroups) != 2 {
		t.Fatalf("expected 2 OR groups, got %d", len(expr.OrGroups))
	}
	if expr.OrGroups[0].Level != "high" {
		t.Fatalf("unexpected first OR group: %+v", expr.OrGroups[0])
	}
	if expr.OrGroups[1].Method != "POST" || expr.OrGroups[1].ResponseType != "json" {
		t.Fatalf("unexpected second OR group: %+v", expr.OrGroups[1])
	}
}

func TestVulnerabilityFilterSQLBuildsGroupClause(t *testing.T) {
	expr, err := parseVulnerabilityFilterExpression("vuln.level=high OR vuln.level=critical")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	clause, args := vulnerabilityFilterSQL(expr.OrGroups[0])
	if clause != "level = ?" {
		t.Fatalf("unexpected clause: %s", clause)
	}
	if len(args) != 1 || args[0] != "high" {
		t.Fatalf("unexpected args: %#v", args)
	}
}
