package wpi

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

const (
	defaultListLimit = 200
	maxListLimit     = 1000
)

// query is SQL text plus its parameters. Identifiers are quoted; values are
// always parameters.
type query struct {
	sql  string
	args []any
}

func (q *query) param(v any) string {
	q.args = append(q.args, paramValue(v))
	return "$" + strconv.Itoa(len(q.args))
}

// paramValue converts a decoded JSON value to a driver value. Numbers are
// passed as text so PostgreSQL converts them to the column's type; objects and
// arrays are passed as JSON text.
func paramValue(v any) any {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	}
	return v
}

func listLimit(limit *int) int {
	switch {
	case limit == nil || *limit <= 0:
		return defaultListLimit
	case *limit > maxListLimit:
		return maxListLimit
	}
	return *limit
}

// buildLoad builds the SELECT of a load action; where holds substituted values.
// Selected columns follow a.Fields, so row column i maps to a.Fields[i].
func buildLoad(a Action, where map[string]any) query {
	var q query
	cols := make([]string, len(a.Fields))
	for i, f := range a.Fields {
		cols[i] = pq.QuoteIdentifier(f.Column)
	}
	var sb strings.Builder
	sb.WriteString("SELECT " + strings.Join(cols, ", ") + " FROM " + quoteTable(a.Table))
	if len(where) > 0 {
		var conds []string
		for _, col := range slices.Sorted(maps.Keys(where)) {
			if where[col] == nil {
				conds = append(conds, pq.QuoteIdentifier(col)+" IS NULL")
			} else {
				conds = append(conds, pq.QuoteIdentifier(col)+" = "+q.param(where[col]))
			}
		}
		sb.WriteString(" WHERE " + strings.Join(conds, " AND "))
	}
	if a.Result == "record" {
		sb.WriteString(" LIMIT 1")
	} else {
		if len(a.OrderBy) > 0 {
			keys := make([]string, len(a.OrderBy))
			for i, o := range a.OrderBy {
				dir := "ASC"
				if o.Direction == "desc" {
					dir = "DESC"
				}
				keys[i] = pq.QuoteIdentifier(o.Column) + " " + dir
			}
			sb.WriteString(" ORDER BY " + strings.Join(keys, ", "))
		}
		sb.WriteString(" LIMIT " + q.param(listLimit(a.Limit)))
	}
	q.sql = sb.String()
	return q
}

// savedFields returns the mapped fields whose attribute is present in doc.
// The key column is written only when it is one of the mapped fields.
func savedFields(a Action, doc map[string]any, includeKey bool) []FieldMap {
	var out []FieldMap
	for _, f := range a.Fields {
		if _, present := doc[f.Attribute]; !present {
			continue
		}
		if f.Column == a.Key && !includeKey {
			continue
		}
		out = append(out, f)
	}
	return out
}

func buildInsert(a Action, doc map[string]any) query {
	var q query
	table := quoteTable(a.Table)
	key := pq.QuoteIdentifier(a.Key)
	fields := savedFields(a, doc, true)
	if len(fields) == 0 {
		q.sql = "INSERT INTO " + table + " DEFAULT VALUES RETURNING " + key
		return q
	}
	cols := make([]string, len(fields))
	vals := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = pq.QuoteIdentifier(f.Column)
		vals[i] = q.param(doc[f.Attribute])
	}
	q.sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s", table, strings.Join(cols, ", "), strings.Join(vals, ", "), key)
	return q
}

// buildUpdate builds the UPDATE of one row. ok is false when doc holds none of
// the action's attributes, so there is nothing to write.
func buildUpdate(a Action, doc map[string]any, keyValue any) (q query, ok bool) {
	fields := savedFields(a, doc, false)
	if len(fields) == 0 {
		return q, false
	}
	sets := make([]string, len(fields))
	for i, f := range fields {
		sets[i] = pq.QuoteIdentifier(f.Column) + " = " + q.param(doc[f.Attribute])
	}
	key := pq.QuoteIdentifier(a.Key)
	q.sql = fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s RETURNING %s", quoteTable(a.Table), strings.Join(sets, ", "), key, q.param(keyValue), key)
	return q, true
}

// isEmpty reports whether a key or required value counts as missing.
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	}
	return false
}
