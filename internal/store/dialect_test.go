package store

import "testing"

func TestRebind(t *testing.T) {
	cases := []struct{ name, in, pg string }{
		{"no params", "SELECT 1", "SELECT 1"},
		{"three params", "SELECT * FROM t WHERE a = ? AND b = ? AND c = ?", "SELECT * FROM t WHERE a = $1 AND b = $2 AND c = $3"},
		{"single quoted", "SELECT 'a?b', ?", "SELECT 'a?b', $1"},
		{"escaped quote", "SELECT 'it''s ?', ?", "SELECT 'it''s ?', $1"},
		{"double quoted ident", `SELECT "col?" FROM t WHERE x = ?`, `SELECT "col?" FROM t WHERE x = $1`},
		{"twelve params", "? ? ? ? ? ? ? ? ? ? ? ?", "$1 $2 $3 $4 $5 $6 $7 $8 $9 $10 $11 $12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Postgres.Rebind(c.in); got != c.pg {
				t.Errorf("postgres: got %q, want %q", got, c.pg)
			}
			if got := SQLite.Rebind(c.in); got != c.in {
				t.Errorf("sqlite: got %q, want unchanged %q", got, c.in)
			}
		})
	}
}

func TestJSONTypeAndNow(t *testing.T) {
	if got := SQLite.JSONType(); got != "TEXT" {
		t.Errorf("sqlite JSONType = %q", got)
	}
	if got := Postgres.JSONType(); got != "JSONB" {
		t.Errorf("postgres JSONType = %q", got)
	}
	if got := SQLite.Now(); got != "CURRENT_TIMESTAMP" {
		t.Errorf("sqlite Now = %q", got)
	}
	if got := Postgres.Now(); got != "NOW()" {
		t.Errorf("postgres Now = %q", got)
	}
}
