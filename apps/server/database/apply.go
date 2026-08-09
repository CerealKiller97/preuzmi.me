package database

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// Apply creates tables from Schema and brings existing databases forward by
// adding any columns that appear in Schema but are missing from the live
// tables. Edit schema.sql to change the shape; a rebuild picks up the change
// and older databases gain the new columns on the next open. Data migrations
// (rewrites, backfills, imports) stay with the caller.
func Apply(db *sql.DB) error {
	if _, err := db.Exec(Schema); err != nil {
		return fmt.Errorf("database: applying schema: %w", err)
	}

	tables, err := parseSchemaTables(Schema)
	if err != nil {
		return fmt.Errorf("database: parsing schema: %w", err)
	}

	for _, table := range tables {
		if err := syncColumns(db, table); err != nil {
			return err
		}
	}

	return nil
}

type schemaTable struct {
	name    string
	columns []schemaColumn
}

type schemaColumn struct {
	name       string
	definition string // everything after the name, e.g. "INTEGER NOT NULL DEFAULT 0"
}

// parseSchemaTables extracts CREATE TABLE definitions from ddl. Only column
// definitions are returned; table-level constraints (UNIQUE, PRIMARY KEY, …)
// are skipped because SQLite cannot add them with a simple ALTER TABLE.
func parseSchemaTables(ddl string) ([]schemaTable, error) {
	cleaned := stripSQLComments(ddl)
	var tables []schemaTable

	remaining := cleaned
	for {
		upper := strings.ToUpper(remaining)
		idx := strings.Index(upper, "CREATE TABLE")
		if idx < 0 {
			break
		}
		remaining = remaining[idx:]

		rest := remaining[len("CREATE TABLE"):]
		rest = strings.TrimSpace(rest)
		if len(rest) >= len("IF NOT EXISTS") && strings.EqualFold(rest[:len("IF NOT EXISTS")], "IF NOT EXISTS") {
			rest = strings.TrimSpace(rest[len("IF NOT EXISTS"):])
		}

		name, afterName, ok := readIdent(rest)
		if !ok {
			return nil, fmt.Errorf("create table without a name near %q", truncate(remaining, 40))
		}

		body, afterBody, err := readParenBlock(strings.TrimSpace(afterName))
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", name, err)
		}

		cols, err := parseColumnDefs(body)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", name, err)
		}
		tables = append(tables, schemaTable{name: name, columns: cols})
		remaining = afterBody
	}

	if len(tables) == 0 {
		return nil, fmt.Errorf("no CREATE TABLE statements found")
	}

	return tables, nil
}

func parseColumnDefs(body string) ([]schemaColumn, error) {
	parts := splitTopLevelCommas(body)
	var cols []schemaColumn
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if isTableConstraint(part) {
			continue
		}

		name, rest, ok := readIdent(part)
		if !ok {
			return nil, fmt.Errorf("column definition without a name: %q", truncate(part, 40))
		}
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return nil, fmt.Errorf("column %s has no type", name)
		}
		cols = append(cols, schemaColumn{name: name, definition: rest})
	}

	return cols, nil
}

func isTableConstraint(part string) bool {
	upper := strings.ToUpper(strings.TrimSpace(part))
	for _, prefix := range []string{"CONSTRAINT", "PRIMARY KEY", "UNIQUE", "CHECK", "FOREIGN KEY"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}

	return false
}

func syncColumns(db *sql.DB, table schemaTable) error {
	existing, err := existingColumns(db, table.name)
	if err != nil {
		return fmt.Errorf("database: reading columns for %s: %w", table.name, err)
	}

	for _, col := range table.columns {
		if existing[col.name] {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table.name, col.name, col.definition)
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("database: adding %s.%s: %w", table.name, col.name, err)
		}
	}

	return nil
}

func existingColumns(db *sql.DB, table string) (map[string]bool, error) {
	// table comes from our own schema.sql identifiers, not user input.
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only rows

	out := make(map[string]bool)
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}

	return out, rows.Err()
}

// stripSQLComments removes -- line comments, preserving newlines so positions
// stay readable in error messages.
func stripSQLComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		if i := strings.Index(s, "--"); i >= 0 {
			b.WriteString(s[:i])
			s = s[i:]
			if j := strings.IndexByte(s, '\n'); j >= 0 {
				b.WriteByte('\n')
				s = s[j+1:]
			} else {
				break
			}
			continue
		}
		b.WriteString(s)
		break
	}

	return b.String()
}

func readIdent(s string) (name, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", s, false
	}

	// Quoted "identifier"
	if s[0] == '"' {
		end := strings.IndexByte(s[1:], '"')
		if end < 0 {
			return "", s, false
		}
		return s[1 : 1+end], s[2+end:], true
	}

	// Bracketed [identifier]
	if s[0] == '[' {
		end := strings.IndexByte(s[1:], ']')
		if end < 0 {
			return "", s, false
		}
		return s[1 : 1+end], s[2+end:], true
	}

	i := 0
	for i < len(s) {
		r := rune(s[i])
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return "", s, false
			}
		} else if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		i++
	}
	if i == 0 {
		return "", s, false
	}

	return s[:i], s[i:], true
}

func readParenBlock(s string) (body, rest string, err error) {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '(' {
		return "", s, fmt.Errorf("expected '(' after table name")
	}

	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[1:i], s[i+1:], nil
			}
		}
	}

	return "", s, fmt.Errorf("unclosed '(' in CREATE TABLE")
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])

	return parts
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}

	return s[:n] + "…"
}
