// Package dotenv losslessly parses Envseal's intentionally narrow dotenv subset.
package dotenv

import (
	"bytes"
	"errors"
	"sort"

	"github.com/kensonjohnson/envseal/internal/envelope"
)

// Category identifies a value-free validation failure suitable for a CLI
// diagnostic. Categories never contain dotenv values or caller-supplied text.
type Category string

const (
	CategoryDuplicateKey      Category = "duplicate-key"
	CategoryUnsupportedSyntax Category = "unsupported-syntax"
	CategoryMissingKey        Category = "missing-key"
	CategoryMalformedEnvelope Category = "malformed-envelope"
	CategoryTransformFailed   Category = "transform-failed"
)

// Issue describes a problem at a one-indexed dotenv line. Line is zero only
// when no line exists, such as a requested key that is absent from the file.
type Issue struct {
	Line     int
	Category Category
}

// Error contains only value-free diagnostics. It may wrap an internal cause
// for programmatic handling, while Error itself remains safe to print.
type Error struct {
	Issues []Issue
	cause  error
}

func (e *Error) Error() string { return "dotenv validation failed" }
func (e *Error) Unwrap() error { return e.cause }

// Document retains every original byte and records the exact value spans that
// are safe to transform. A Document is immutable after Parse.
type Document struct {
	source []byte
	lines  []line
}

type lineKind uint8

const (
	lineBlank lineKind = iota
	lineComment
	lineAssignment
	lineUnsupported
)

type line struct {
	number     int
	kind       lineKind
	key        string
	candidate  string
	valueStart int
	valueEnd   int
}

// ValueTransform receives the exact case-sensitive key and raw value bytes.
// It must return only the replacement value body, not an assignment or line
// ending.
type ValueTransform func(key, value []byte) ([]byte, error)

// Result is a lossless transform result. Data differs from the source only in
// selected value spans; Changed and Skipped are useful for command summaries.
type Result struct {
	Data    []byte
	Changed int
	Skipped int
}

// Parse retains LF and CRLF source bytes exactly. It accepts unsupported lines
// so encrypt/decrypt can preserve unselected syntax, but duplicate supported
// keys are a whole-file error for every operation.
func Parse(source []byte) (*Document, error) {
	doc := &Document{source: append([]byte(nil), source...)}
	seen := make(map[string][]int)

	for start, number := 0, 1; start < len(doc.source); number++ {
		bodyEnd, next := nextLine(doc.source, start)
		body := doc.source[start:bodyEnd]
		entry := classifyLine(body, number, start)
		doc.lines = append(doc.lines, entry)
		if entry.key != "" {
			seen[entry.key] = append(seen[entry.key], number)
		}
		start = next
	}

	var issues []Issue
	for _, lines := range seen {
		if len(lines) < 2 {
			continue
		}
		for _, number := range lines {
			issues = append(issues, Issue{Line: number, Category: CategoryDuplicateKey})
		}
	}
	if len(issues) != 0 {
		return nil, newError(issues, nil)
	}
	return doc, nil
}

// Encrypt replaces only named plaintext assignment values. It rejects missing
// or unsupported selected keys, structurally rejects malformed selected
// envelopes, and leaves valid pre-existing envelopes unchanged.
func (d *Document) Encrypt(keys []string, seal ValueTransform) (Result, error) {
	if seal == nil {
		return Result{}, newError([]Issue{{Category: CategoryTransformFailed}}, errors.New("nil seal transform"))
	}

	selected := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		selected[key] = struct{}{}
	}
	assignments, issues := d.selectedAssignments(selected)
	if len(issues) != 0 {
		return Result{}, newError(issues, nil)
	}

	result := Result{}
	replacements := make([]replacement, 0, len(assignments))
	for _, entry := range assignments {
		value := d.source[entry.valueStart:entry.valueEnd]
		if isEnvelope(value) {
			if err := envelope.Validate(string(value)); err != nil {
				return Result{}, newError([]Issue{{Line: entry.number, Category: CategoryMalformedEnvelope}}, err)
			}
			result.Skipped++
			continue
		}
		replacementValue, err := seal([]byte(entry.key), value)
		if err != nil {
			return Result{}, newError([]Issue{{Line: entry.number, Category: CategoryTransformFailed}}, err)
		}
		replacements = append(replacements, replacement{start: entry.valueStart, end: entry.valueEnd, value: replacementValue})
		result.Changed++
	}
	result.Data = d.render(replacements)
	return result, nil
}

// Decrypt replaces every structurally valid envelope in a recognized
// assignment, leaving plaintext assignments and unsupported lines untouched.
func (d *Document) Decrypt(open ValueTransform) (Result, error) {
	if open == nil {
		return Result{}, newError([]Issue{{Category: CategoryTransformFailed}}, errors.New("nil open transform"))
	}

	result := Result{}
	var replacements []replacement
	for _, entry := range d.lines {
		if entry.kind != lineAssignment {
			continue
		}
		value := d.source[entry.valueStart:entry.valueEnd]
		if !isEnvelope(value) {
			continue
		}
		if err := envelope.Validate(string(value)); err != nil {
			return Result{}, newError([]Issue{{Line: entry.number, Category: CategoryMalformedEnvelope}}, err)
		}
		plaintext, err := open([]byte(entry.key), value)
		if err != nil {
			return Result{}, newError([]Issue{{Line: entry.number, Category: CategoryTransformFailed}}, err)
		}
		replacements = append(replacements, replacement{start: entry.valueStart, end: entry.valueEnd, value: plaintext})
		result.Changed++
	}
	result.Data = d.render(replacements)
	return result, nil
}

// Check validates the complete v1 dotenv grammar and the structure and size
// limits of every Envseal envelope without requiring a password.
func (d *Document) Check() error {
	var issues []Issue
	for _, entry := range d.lines {
		switch entry.kind {
		case lineUnsupported:
			issues = append(issues, Issue{Line: entry.number, Category: CategoryUnsupportedSyntax})
		case lineAssignment:
			value := d.source[entry.valueStart:entry.valueEnd]
			if isEnvelope(value) {
				if err := envelope.Validate(string(value)); err != nil {
					issues = append(issues, Issue{Line: entry.number, Category: CategoryMalformedEnvelope})
				}
			}
		}
	}
	if len(issues) != 0 {
		return newError(issues, nil)
	}
	return nil
}

func (d *Document) selectedAssignments(selected map[string]struct{}) ([]line, []Issue) {
	if len(selected) == 0 {
		return nil, []Issue{{Category: CategoryMissingKey}}
	}

	found := make(map[string]bool, len(selected))
	var assignments []line
	var issues []Issue
	for _, entry := range d.lines {
		if _, ok := selected[entry.key]; ok {
			found[entry.key] = true
			if entry.kind == lineAssignment {
				assignments = append(assignments, entry)
			} else {
				issues = append(issues, Issue{Line: entry.number, Category: CategoryUnsupportedSyntax})
			}
			continue
		}
		if _, ok := selected[entry.candidate]; ok {
			found[entry.candidate] = true
			issues = append(issues, Issue{Line: entry.number, Category: CategoryUnsupportedSyntax})
		}
	}
	for key := range selected {
		if !found[key] {
			issues = append(issues, Issue{Category: CategoryMissingKey})
		}
	}
	return assignments, issues
}

type replacement struct {
	start int
	end   int
	value []byte
}

func (d *Document) render(replacements []replacement) []byte {
	if len(replacements) == 0 {
		return append([]byte(nil), d.source...)
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })

	output := make([]byte, 0, len(d.source))
	position := 0
	for _, replacement := range replacements {
		output = append(output, d.source[position:replacement.start]...)
		output = append(output, replacement.value...)
		position = replacement.end
	}
	output = append(output, d.source[position:]...)
	return output
}

func nextLine(source []byte, start int) (bodyEnd, next int) {
	newline := bytes.IndexByte(source[start:], '\n')
	if newline < 0 {
		return len(source), len(source)
	}
	newline += start
	if newline > start && source[newline-1] == '\r' {
		return newline - 1, newline + 1
	}
	return newline, newline + 1
}

func classifyLine(body []byte, number, start int) line {
	entry := line{number: number, kind: lineUnsupported}
	if isHorizontalSpaceOnly(body) {
		entry.kind = lineBlank
		return entry
	}
	trimmed := trimHorizontalSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '#' {
		entry.kind = lineComment
		return entry
	}

	equals := bytes.IndexByte(body, '=')
	if equals < 0 {
		return entry
	}
	key := body[:equals]
	if validKey(key) {
		entry.key = string(key)
		entry.candidate = entry.key
		entry.valueStart = start + equals + 1
		entry.valueEnd = start + len(body)
		if validValue(body[equals+1:]) {
			entry.kind = lineAssignment
		}
		return entry
	}
	entry.candidate = unsupportedCandidate(key)
	return entry
}

func isHorizontalSpaceOnly(value []byte) bool {
	for _, character := range value {
		if character != ' ' && character != '\t' {
			return false
		}
	}
	return true
}

func trimHorizontalSpace(value []byte) []byte {
	return bytes.Trim(value, " \t")
}

func validKey(key []byte) bool {
	if len(key) == 0 || !isKeyStart(key[0]) {
		return false
	}
	for _, character := range key[1:] {
		if !isKeyStart(character) && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func isKeyStart(character byte) bool {
	return character == '_' || (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z')
}

func validValue(value []byte) bool {
	if len(value) == 0 {
		return true
	}
	if isHorizontalSpace(value[0]) || isHorizontalSpace(value[len(value)-1]) || bytes.IndexByte(value, '\r') >= 0 {
		return false
	}
	for index, character := range value {
		switch character {
		case '#', '\'', '"', '`':
			return false
		case '$':
			if index+1 < len(value) && (isKeyStart(value[index+1]) || value[index+1] == '{' || value[index+1] == '(') {
				return false
			}
		}
	}
	return value[len(value)-1] != '\\'
}

func isHorizontalSpace(character byte) bool {
	return character == ' ' || character == '\t'
}

func unsupportedCandidate(key []byte) string {
	candidate := trimHorizontalSpace(key)
	const export = "export"
	if bytes.HasPrefix(candidate, []byte(export)) && len(candidate) > len(export) && isHorizontalSpace(candidate[len(export)]) {
		candidate = trimHorizontalSpace(candidate[len(export):])
	}
	return string(candidate)
}

func isEnvelope(value []byte) bool {
	return bytes.HasPrefix(value, []byte("ENVSEAL["))
}

func newError(issues []Issue, cause error) *Error {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Line == issues[j].Line {
			return issues[i].Category < issues[j].Category
		}
		if issues[i].Line == 0 {
			return false
		}
		if issues[j].Line == 0 {
			return true
		}
		return issues[i].Line < issues[j].Line
	})
	return &Error{Issues: issues, cause: cause}
}
