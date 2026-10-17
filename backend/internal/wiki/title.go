// Package wiki talks to the MediaWiki API and models page titles.
package wiki

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Title is a page title without namespace prefix, plus its namespace.
// Name uses spaces, not underscores. Title is comparable and identifies a
// page within one wiki.
type Title struct {
	NS   int
	Name string
}

// NewTitle returns the title name in namespace ns; underscores become spaces.
func NewTitle(name string, ns int) Title {
	return Title{NS: ns, Name: UnderscoresToSpaces(name)}
}

// Underscored returns the name with underscores, as stored in the database.
func (t Title) Underscored() string {
	return SpacesToUnderscores(t.Name)
}

// UnderscoresToSpaces replaces underscores by spaces and trims.
func UnderscoresToSpaces(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "_", " "))
}

// SpacesToUnderscores trims and replaces spaces by underscores.
func SpacesToUnderscores(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), " ", "_")
}

// UcFirst upper-cases the first letter, as MediaWiki does for titles in
// namespaces with "first-letter" case.
func UcFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToTitle(r)) + s[size:]
}
