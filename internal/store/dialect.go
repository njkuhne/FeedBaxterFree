package store

import "strings"

// Dialect identifies the SQL database behind a DB.
type Dialect string

const (
	// SQLite is the self-host database (modernc.org/sqlite).
	SQLite Dialect = "sqlite"
	// Postgres is the SaaS database (pgx).
	Postgres Dialect = "postgres"
)

// Rebind rewrites ? placeholders to $1, $2, ... for Postgres; SQLite returns
// the query unchanged. A ? inside a single-quoted string literal (with ”
// escapes) or a double-quoted identifier is left alone. It does not understand
// comments or the Postgres ? JSONB operators, so queries must not use either.
func (d Dialect) Rebind(query string) string {
	if d != Postgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	var quote byte // 0, '\'' or '"'
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case quote != 0:
			b.WriteByte(c)
			if c == quote {
				// A doubled quote is an escape and stays inside the literal:
				// closing then immediately reopening gives the same result.
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
			b.WriteByte(c)
		case c == '?':
			n++
			b.WriteByte('$')
			b.WriteString(itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// JSONType is the column type for JSON documents: TEXT or JSONB.
func (d Dialect) JSONType() string {
	if d == Postgres {
		return "JSONB"
	}
	return "TEXT"
}

// Now is the SQL expression for the current timestamp.
func (d Dialect) Now() string {
	if d == Postgres {
		return "NOW()"
	}
	return "CURRENT_TIMESTAMP"
}

func itoa(n int) string {
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
