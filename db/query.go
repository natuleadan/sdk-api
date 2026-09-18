package db

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// SQLDialect is a SQL family the query helpers translate to. PostgreSQL is the
// pivot syntax: queries are written with $n placeholders and translated on
// demand for the other families.
type SQLDialect string

const (
	DialectPostgres SQLDialect = "postgres"
	DialectMySQL    SQLDialect = "mysql"  // MySQL and MariaDB
	DialectSQLite   SQLDialect = "sqlite" // Turso / libSQL
)

// DialectForDriver maps a YAML database driver to its SQL dialect.
func DialectForDriver(driver string) SQLDialect {
	switch driver {
	case "mysql", "mariadb":
		return DialectMySQL
	case "turso", "turso-serverless", "libsql", "go-libsql", "sqlite":
		return DialectSQLite
	default:
		return DialectPostgres
	}
}

// Placeholder returns the placeholder for the i-th (1-based) argument.
func Placeholder(d SQLDialect, i int) string {
	if d == DialectPostgres {
		return "$" + strconv.Itoa(i)
	}
	return "?"
}

// Rewrite converts a query written with PostgreSQL syntax to the dialect:
// $n placeholders become positional (with argument expansion), and the generic
// expressions now() and gen_random_uuid() are translated. PostgreSQL is a
// no-op. Placeholders and expressions inside single-quoted string literals are
// left untouched.
func Rewrite(d SQLDialect, query string, args ...any) (string, []any) {
	if d == DialectPostgres {
		return query, args
	}
	if d == DialectMySQL {
		query = translateMySQLUpsert(query)
	}
	var b strings.Builder
	out := make([]any, 0, len(args))
	usedDollar := false
	inQuote := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == '\'' {
			if inQuote && i+1 < len(query) && query[i+1] == '\'' {
				b.WriteString("''")
				i++
				continue
			}
			inQuote = !inQuote
			b.WriteByte(c)
			continue
		}
		if inQuote {
			b.WriteByte(c)
			continue
		}
		if n, end, ok := takePlaceholder(query, i, len(args)); ok {
			b.WriteString(Placeholder(d, len(out)+1))
			out = append(out, args[n-1])
			usedDollar = true
			i = end
			continue
		}
		if expr, end, ok := takeFunction(query, i, d); ok {
			b.WriteString(expr)
			i = end
			continue
		}
		b.WriteByte(c)
	}
	if !usedDollar {
		// The query already used positional (?) placeholders, so the caller's
		// arguments line up with them unchanged.
		return b.String(), args
	}
	return b.String(), out
}

// takePlaceholder reports the $n argument index ending at the closing digit and
// the index of that digit, when query[i] starts a valid placeholder.
func takePlaceholder(query string, i, nargs int) (int, int, bool) {
	if query[i] != '$' || i+1 >= len(query) || !isDigit(query[i+1]) {
		return 0, 0, false
	}
	j := i + 1
	for j < len(query) && isDigit(query[j]) {
		j++
	}
	n, _ := strconv.Atoi(query[i+1 : j])
	if n < 1 || n > nargs {
		return 0, 0, false
	}
	return n, j - 1, true
}

// takeFunction translates a call to now() or gen_random_uuid() at query[i].
func takeFunction(query string, i int, d SQLDialect) (string, int, bool) {
	if !isIdentStart(query[i]) {
		return "", 0, false
	}
	j := i + 1
	for j < len(query) && isIdentPart(query[j]) {
		j++
	}
	end, ok := matchCall(query, j)
	if !ok {
		return "", 0, false
	}
	switch strings.ToLower(query[i:j]) {
	case "now":
		if expr, e, ok := takeInterval(query, end, d); ok {
			return expr, e, true
		}
		return Now(d), end, true
	case "gen_random_uuid":
		return RandomUUIDExpr(d), end, true
	default:
		return "", 0, false
	}
}

// takeInterval translates "now() + interval 'N unit'" into the dialect form.
func takeInterval(query string, from int, d SQLDialect) (string, int, bool) {
	i := skipSpace(query, from+1)
	if i >= len(query) || query[i] != '+' {
		return "", 0, false
	}
	i = skipSpace(query, i+1)
	const kw = "interval"
	if !strings.HasPrefix(strings.ToLower(query[i:]), kw) {
		return "", 0, false
	}
	lit, end, ok := takeQuoted(query, skipSpace(query, i+len(kw)))
	if !ok {
		return "", 0, false
	}
	return AddInterval(d, Now(d), lit), end, true
}

func skipSpace(q string, i int) int {
	for i < len(q) && q[i] == ' ' {
		i++
	}
	return i
}

func takeQuoted(q string, i int) (string, int, bool) {
	if i >= len(q) || q[i] != '\'' {
		return "", 0, false
	}
	j := i + 1
	for j < len(q) && q[j] != '\'' {
		j++
	}
	if j >= len(q) {
		return "", 0, false
	}
	return q[i+1 : j], j, true
}

var excludedRe = regexp.MustCompile(`EXCLUDED\.([A-Za-z0-9_]+)`)

// translateMySQLUpsert converts a PostgreSQL ON CONFLICT clause into the
// MySQL equivalent (INSERT ... ON DUPLICATE KEY UPDATE).
func translateMySQLUpsert(query string) string {
	idx := strings.Index(strings.ToLower(query), "on conflict")
	if idx < 0 {
		return query
	}
	head := query[:idx]
	rest := strings.TrimLeft(query[idx+len("on conflict"):], " ")
	col := firstColumn(head)
	if strings.HasPrefix(rest, "(") {
		depth, k := 1, 1
		for k < len(rest) && depth > 0 {
			switch rest[k] {
			case '(':
				depth++
			case ')':
				depth--
			}
			k++
		}
		if c := firstColumn(rest[:k]); c != "" {
			col = c
		}
		rest = strings.TrimLeft(rest[k:], " ")
	}
	upper := strings.ToUpper(rest)
	switch {
	case strings.HasPrefix(upper, "DO NOTHING"):
		if col == "" {
			return query
		}
		return head + "ON DUPLICATE KEY UPDATE " + col + " = " + col
	case strings.HasPrefix(upper, "DO UPDATE SET"):
		sets := strings.TrimSpace(rest[len("DO UPDATE SET"):])
		return head + "ON DUPLICATE KEY UPDATE " + excludedRe.ReplaceAllString(sets, "VALUES($1)")
	default:
		return query
	}
}

// firstColumn returns the first identifier inside the first parentheses.
func firstColumn(s string) string {
	_, after, ok := strings.Cut(s, "(")
	if !ok {
		return ""
	}
	rest := after
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		if rest[i] == ',' || rest[i] == ')' {
			end = i
			break
		}
	}
	return strings.Trim(strings.TrimSpace(rest[:end]), "`\"")
}

// RandomUUIDExpr returns a server-side random id expression for the dialect.
func RandomUUIDExpr(d SQLDialect) string {
	switch d {
	case DialectMySQL:
		return "(UUID())"
	case DialectSQLite:
		return "(lower(hex(randomblob(16))))"
	default:
		return "gen_random_uuid()"
	}
}

// matchCall reports whether the identifier ending at `from` is followed by an
// (optionally spaced) empty-argument call "()", returning the index of the
// closing parenthesis.
func matchCall(query string, from int) (int, bool) {
	i := from
	for i < len(query) && query[i] == ' ' {
		i++
	}
	if i >= len(query) || query[i] != '(' {
		return 0, false
	}
	j := i + 1
	for j < len(query) && query[j] == ' ' {
		j++
	}
	if j >= len(query) || query[j] != ')' {
		return 0, false
	}
	return j, true
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || isDigit(b)
}

// Now returns the current-timestamp expression for the dialect.
func Now(d SQLDialect) string {
	switch d {
	case DialectMySQL:
		return "NOW()"
	case DialectSQLite:
		return "CURRENT_TIMESTAMP"
	default:
		return "now()"
	}
}

// AddInterval returns an expression adding a human interval (e.g. "15 minutes",
// "24 hours") to base.
func AddInterval(d SQLDialect, base, interval string) string {
	amount, unit := parseInterval(interval)
	switch d {
	case DialectMySQL:
		return fmt.Sprintf("DATE_ADD(%s, INTERVAL %d %s)", base, amount, strings.ToUpper(unit))
	case DialectSQLite:
		return fmt.Sprintf("datetime(%s, '+%d %s')", base, amount, unit)
	default:
		return fmt.Sprintf("(%s + interval '%d %s')", base, amount, unit)
	}
}

// InList returns "column IN (<placeholders>)" and its arguments. Avoids
// arrays/ANY, which SQLite and MySQL do not support.
func InList(d SQLDialect, column string, values []string) (string, []any) {
	if len(values) == 0 {
		return "1 = 0", nil
	}
	ph := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		ph[i] = Placeholder(d, i+1)
		args[i] = v
	}
	return column + " IN (" + strings.Join(ph, ", ") + ")", args
}

// UpsertClause returns the conflict clause appended to an INSERT. conflict are
// the columns of the unique constraint; updates are the columns to overwrite.
func UpsertClause(d SQLDialect, conflict, updates []string) string {
	if d == DialectMySQL {
		if len(updates) == 0 {
			col := conflict[0]
			return fmt.Sprintf("ON DUPLICATE KEY UPDATE %s = %s", col, col)
		}
		sets := make([]string, len(updates))
		for i, c := range updates {
			sets[i] = fmt.Sprintf("%s = VALUES(%s)", c, c)
		}
		return "ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", ")
	}
	if len(updates) == 0 {
		return fmt.Sprintf("ON CONFLICT (%s) DO NOTHING", strings.Join(conflict, ", "))
	}
	sets := make([]string, len(updates))
	for i, c := range updates {
		sets[i] = fmt.Sprintf("%s = EXCLUDED.%s", c, c)
	}
	return fmt.Sprintf("ON CONFLICT (%s) DO UPDATE SET %s", strings.Join(conflict, ", "), strings.Join(sets, ", "))
}

// ReturningClause returns "RETURNING <cols>" where supported. MySQL has no
// RETURNING; callers there read LastInsertId instead.
func ReturningClause(d SQLDialect, cols string) string {
	if d == DialectMySQL {
		return ""
	}
	return "RETURNING " + cols
}

// NewID returns a random UUIDv4 string, for drivers without a server-side
// gen_random_uuid() default (SQLite/Turso).
func NewID() string {
	return uuid.NewString()
}

func parseInterval(interval string) (int, string) {
	fields := strings.Fields(strings.TrimSpace(interval))
	if len(fields) < 2 {
		return 1, strings.TrimSuffix(strings.ToLower(interval), "s")
	}
	n, err := strconv.Atoi(fields[0])
	unit := strings.TrimSuffix(strings.ToLower(fields[len(fields)-1]), "s")
	if err != nil {
		return 1, unit
	}
	return n, unit
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
