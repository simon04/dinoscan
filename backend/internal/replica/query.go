package replica

import (
	"fmt"
	"strings"
)

// Fragment is a piece of SQL text.
//
// Only untyped string constants convert to a Fragment implicitly, so SQL text
// cannot be assembled from runtime strings by accident: every value a query
// depends on goes through a placeholder of a prepared statement. An explicit
// Fragment(...) conversion is the (greppable) escape hatch; there should be
// no need for one.
type Fragment string

// Query is an SQL statement under construction, plus the values bound to its
// ? placeholders.
type Query struct {
	text strings.Builder
	args []any
}

// New starts a query.
func New(sql Fragment, args ...any) *Query {
	return new(Query).Add(sql, args...)
}

// Add appends sql and the values for the placeholders in it. It panics if the
// number of placeholders and values differ, which is a programming error.
func (q *Query) Add(sql Fragment, args ...any) *Query {
	if n := strings.Count(string(sql), "?"); n != len(args) {
		panic(fmt.Sprintf("replica: %d placeholders but %d values in %q", n, len(args), sql))
	}
	q.text.WriteString(string(sql))
	q.args = append(q.args, args...)
	return q
}

// In appends " IN (?,?,…)" with one placeholder per value. It panics on an
// empty list, which has no valid SQL form; callers check for it.
func (q *Query) In(values []any) *Query {
	if len(values) == 0 {
		panic("replica: empty IN list")
	}
	q.text.WriteString(" IN (")
	for i := range values {
		if i > 0 {
			q.text.WriteByte(',')
		}
		q.text.WriteByte('?')
	}
	q.text.WriteByte(')')
	q.args = append(q.args, values...)
	return q
}

// Append appends another query's text and values.
func (q *Query) Append(other *Query) *Query {
	if other == nil {
		return q
	}
	q.text.WriteString(other.text.String())
	q.args = append(q.args, other.args...)
	return q
}

// Clone returns an independent copy.
func (q *Query) Clone() *Query {
	return new(Query).Append(q)
}

// Empty reports whether nothing has been added.
func (q *Query) Empty() bool {
	return q == nil || q.text.Len() == 0
}

// SQL returns the statement text.
func (q *Query) SQL() string {
	return q.text.String()
}

// Args returns the bound values.
func (q *Query) Args() []any {
	return q.args
}

// Strings converts values for In.
func Strings(values []string) []any {
	ret := make([]any, len(values))
	for i, v := range values {
		ret[i] = v
	}
	return ret
}

// Ints converts values for In.
func Ints[T ~int | ~int32 | ~int64 | ~uint32 | ~uint64](values []T) []any {
	ret := make([]any, len(values))
	for i, v := range values {
		ret[i] = int64(v)
	}
	return ret
}
