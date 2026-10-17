package petscan

import (
	"context"
	"regexp"
	"strings"
)

// Combination is a tree telling how the results of the data sources are
// combined, e.g. "categories AND (sparql OR manual)".
type Combination struct {
	Op          string // "source", "and", "or" or "not"
	Source      string // for Op "source"
	Left, Right *Combination
}

func (c *Combination) String() string {
	if c == nil {
		return "nothing"
	}
	switch c.Op {
	case "source":
		return c.Source
	case "and":
		return "(" + c.Left.String() + " AND " + c.Right.String() + ")"
	case "or":
		return "(" + c.Left.String() + " OR " + c.Right.String() + ")"
	default:
		return "(" + c.Left.String() + " NOT " + c.Right.String() + ")"
	}
}

var reCombinationToken = regexp.MustCompile(`\w+(?:'\w+)?|[^\w\s]`)

var sourceNames = []string{"categories", "sparql", "manual", "pagepile", "wikidata", "search"}

// parseCombination parses the source_combination parameter. Operators are
// right-associative: "a AND b OR c" is "a AND (b OR c)". It returns nil for
// an empty or invalid expression.
func parseCombination(s string) *Combination {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return nil
	}
	for _, name := range sourceNames {
		if lower == name {
			return &Combination{Op: "source", Source: name}
		}
	}

	tokens := reCombinationToken.FindAllString(s, -1)
	if len(tokens) < 3 {
		return nil
	}
	var left string
	if tokens[0] == "(" {
		inner, rest, ok := splitParenthesized(tokens)
		if !ok {
			return nil
		}
		left, tokens = inner, rest
	} else {
		left, tokens = tokens[0], tokens[1:]
	}
	if len(tokens) == 0 {
		return parseCombination(left)
	}
	op := strings.ToLower(tokens[0])
	if op != "and" && op != "or" && op != "not" {
		return nil
	}
	return &Combination{
		Op:    op,
		Left:  parseCombination(left),
		Right: parseCombination(strings.Join(tokens[1:], " ")),
	}
}

// splitParenthesized takes tokens starting with "(" and returns the text
// up to the matching ")" and the tokens after it.
func splitParenthesized(tokens []string) (string, []string, bool) {
	depth := 0
	var inner []string
	for i, token := range tokens {
		switch token {
		case "(":
			if depth > 0 {
				inner = append(inner, token)
			}
			depth++
		case ")":
			depth--
			if depth == 0 {
				return strings.Join(inner, " "), tokens[i+1:], true
			}
			if depth < 0 {
				return "", nil, false
			}
			inner = append(inner, token)
		default:
			inner = append(inner, token)
		}
	}
	return "", nil, false
}

// defaultCombination intersects all available sources.
func defaultCombination(sources []string) *Combination {
	var c *Combination
	for _, name := range sources {
		s := &Combination{Op: "source", Source: name}
		if c == nil {
			c = s
		} else {
			c = &Combination{Op: "and", Left: s, Right: c}
		}
	}
	return c
}

// combine evaluates c over the source results, each of which may be used
// once. When two lists are on different wikis, the right one is converted to
// the wiki of the left one.
func (r *Run) combine(ctx context.Context, c *Combination, results map[string]*PageList) (*PageList, error) {
	if c == nil {
		return nil, invalidf("invalid source combination")
	}
	if c.Op == "source" {
		pl, ok := results[c.Source]
		if !ok {
			return nil, invalidf("no result for source %s, or it is used twice", c.Source)
		}
		delete(results, c.Source)
		return pl, nil
	}

	// A missing side of OR, or right side of NOT, leaves the other side.
	switch {
	case c.Op == "or" && c.Left == nil:
		return r.combine(ctx, c.Right, results)
	case (c.Op == "or" || c.Op == "not") && c.Right == nil:
		return r.combine(ctx, c.Left, results)
	case c.Left == nil || c.Right == nil:
		return nil, invalidf("invalid source combination %s", c)
	}

	left, err := r.combine(ctx, c.Left, results)
	if err != nil {
		return nil, err
	}
	right, err := r.combine(ctx, c.Right, results)
	if err != nil {
		return nil, err
	}
	if err := r.convertToWiki(ctx, right, left.Wiki); err != nil {
		return nil, err
	}
	switch c.Op {
	case "and":
		left.Intersect(right)
	case "or":
		left.Union(right)
	case "not":
		left.Subtract(right)
	}
	return left, nil
}
