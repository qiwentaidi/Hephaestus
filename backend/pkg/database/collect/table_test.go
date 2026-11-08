package collect

import "testing"

func TestValidateSQLTableReferenceRejectsInjectedIdentifiers(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		table  string
	}{
		{name: "where clause", schema: "public", table: "users WHERE 1=1 --"},
		{name: "union", schema: "public", table: "users UNION SELECT password FROM users"},
		{name: "qualified table", schema: "public", table: "public.users"},
		{name: "empty schema", schema: "", table: "users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateSQLTableReference(tt.schema, tt.table); err == nil {
				t.Fatalf("validateSQLTableReference(%q, %q) accepted an unsafe identifier", tt.schema, tt.table)
			}
		})
	}
}

func TestQuoteSQLIdentifier(t *testing.T) {
	if got := quoteSQLIdentifier("users", "`", "`"); got != "`users`" {
		t.Fatalf("MySQL identifier quote = %q", got)
	}
	if got := quoteSQLIdentifier("users", `"`, `"`); got != `"users"` {
		t.Fatalf("SQL identifier quote = %q", got)
	}
}
