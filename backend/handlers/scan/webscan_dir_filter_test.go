package scan

import "testing"

func TestParseDirResultFilterExpressionAccumulatesRepeatedNumericFields(t *testing.T) {
	expr, err := parseDirResultFilterExpression("dir.status=200 AND dir.status=301 AND dir.recursion>=1 AND dir.recursion<3")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(expr.Status) != 2 {
		t.Fatalf("expected 2 status conditions, got %d", len(expr.Status))
	}
	if expr.Status[0].kind != "=" || expr.Status[0].value != 200 {
		t.Fatalf("unexpected first status condition: %+v", expr.Status[0])
	}
	if expr.Status[1].kind != "=" || expr.Status[1].value != 301 {
		t.Fatalf("unexpected second status condition: %+v", expr.Status[1])
	}

	if len(expr.Recursion) != 2 {
		t.Fatalf("expected 2 recursion conditions, got %d", len(expr.Recursion))
	}
	if expr.Recursion[0].kind != ">=" || expr.Recursion[0].value != 1 {
		t.Fatalf("unexpected first recursion condition: %+v", expr.Recursion[0])
	}
	if expr.Recursion[1].kind != "<" || expr.Recursion[1].value != 3 {
		t.Fatalf("unexpected second recursion condition: %+v", expr.Recursion[1])
	}
}

func TestParseDirResultFilterExpressionAcceptsDoubleEqualsForNumericFields(t *testing.T) {
	expr, err := parseDirResultFilterExpression("dir.status==200")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(expr.Status) != 1 || expr.Status[0].kind != "=" || expr.Status[0].value != 200 {
		t.Fatalf("unexpected status condition: %+v", expr.Status)
	}
}

func TestParseDirResultFilterExpressionSupportsOrGroups(t *testing.T) {
	expr, err := parseDirResultFilterExpression("dir.status=200 OR dir.status=301 AND response.type~html")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(expr.OrGroups) != 2 {
		t.Fatalf("expected 2 OR groups, got %d", len(expr.OrGroups))
	}
	if len(expr.OrGroups[0].Status) != 1 || expr.OrGroups[0].Status[0].value != 200 {
		t.Fatalf("unexpected first OR group: %+v", expr.OrGroups[0])
	}
	if len(expr.OrGroups[1].Status) != 1 || expr.OrGroups[1].Status[0].value != 301 ||
		len(expr.OrGroups[1].Text) != 1 || expr.OrGroups[1].Text[0].column != "content_type" || expr.OrGroups[1].Text[0].value != "html" {
		t.Fatalf("unexpected second OR group: %+v", expr.OrGroups[1])
	}
}

func TestDirResultFilterSQLBuildsOrGroupClauses(t *testing.T) {
	expr, err := parseDirResultFilterExpression("dir.status=200 OR dir.status=301")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	clause, args := dirResultFilterSQL(expr.OrGroups[0])
	if clause != "status = ?" {
		t.Fatalf("unexpected clause: %s", clause)
	}
	if len(args) != 1 || args[0] != 200 {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestParseDirResultFilterExpressionSupportsContainsAndExactTextOperators(t *testing.T) {
	expr, err := parseDirResultFilterExpression("dir.url=admin AND dir.title==Login AND response.type=json")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(expr.Text) != 3 {
		t.Fatalf("expected 3 text conditions, got %d", len(expr.Text))
	}
	if expr.Text[0].column != "url" || expr.Text[0].value != "admin" || expr.Text[0].exact {
		t.Fatalf("unexpected url condition: %+v", expr.Text[0])
	}
	if expr.Text[1].column != "title" || expr.Text[1].value != "Login" || !expr.Text[1].exact {
		t.Fatalf("unexpected title condition: %+v", expr.Text[1])
	}
	if expr.Text[2].column != "content_type" || expr.Text[2].value != "json" || expr.Text[2].exact {
		t.Fatalf("unexpected response type condition: %+v", expr.Text[2])
	}
}
