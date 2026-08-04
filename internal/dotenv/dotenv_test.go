package dotenv

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestEncryptPreservesMixedLineEndingsAndUntouchedBytes(t *testing.T) {
	source := []byte("# retained\r\n\t# also retained\n\nEMPTY=\r\nAPI_TOKEN=secret-value\nUNSELECTED=keep\r\n")
	doc, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := doc.Encrypt([]string{"API_TOKEN", "EMPTY"}, func(key, value []byte) ([]byte, error) {
		return append([]byte("sealed-"), key...), nil
	})
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	want := []byte("# retained\r\n\t# also retained\n\nEMPTY=sealed-EMPTY\r\nAPI_TOKEN=sealed-API_TOKEN\nUNSELECTED=keep\r\n")
	if !bytes.Equal(result.Data, want) {
		t.Fatalf("Encrypt() = %q, want %q", result.Data, want)
	}
	if result.Changed != 2 || result.Skipped != 0 {
		t.Fatalf("Encrypt() counts = changed %d, skipped %d", result.Changed, result.Skipped)
	}
}

func TestEncryptLeavesUnselectedUnsupportedSyntaxUntouched(t *testing.T) {
	source := []byte("QUOTED=\"leave exactly alone\"\nSELECTED=value\nexport ALSO_UNTOUCHED=value\n")
	doc, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result, err := doc.Encrypt([]string{"SELECTED"}, func(_, _ []byte) ([]byte, error) {
		return []byte("replacement"), nil
	})
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	want := []byte("QUOTED=\"leave exactly alone\"\nSELECTED=replacement\nexport ALSO_UNTOUCHED=value\n")
	if !bytes.Equal(result.Data, want) {
		t.Fatalf("Encrypt() = %q, want %q", result.Data, want)
	}
}

func TestEncryptRejectsSelectedUnsupportedSyntaxAndMissingKeys(t *testing.T) {
	tests := []struct {
		name   string
		source string
		keys   []string
		line   int
		kind   Category
	}{
		{name: "quoted", source: "KEY=\"value\"\n", keys: []string{"KEY"}, line: 1, kind: CategoryUnsupportedSyntax},
		{name: "inline comment", source: "KEY=value # comment\n", keys: []string{"KEY"}, line: 1, kind: CategoryUnsupportedSyntax},
		{name: "export", source: "export KEY=value\n", keys: []string{"KEY"}, line: 1, kind: CategoryUnsupportedSyntax},
		{name: "whitespace", source: "KEY =value\n", keys: []string{"KEY"}, line: 1, kind: CategoryUnsupportedSyntax},
		{name: "expansion", source: "KEY=${VALUE}\n", keys: []string{"KEY"}, line: 1, kind: CategoryUnsupportedSyntax},
		{name: "missing", source: "OTHER=value\n", keys: []string{"KEY"}, line: 0, kind: CategoryMissingKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Parse([]byte(test.source))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			_, err = doc.Encrypt(test.keys, func(_, _ []byte) ([]byte, error) { return []byte("replacement"), nil })
			assertIssues(t, err, []Issue{{Line: test.line, Category: test.kind}})
		})
	}
}

func TestParseRejectsEveryDuplicateSupportedKey(t *testing.T) {
	_, err := Parse([]byte("KEY=first\nOTHER=value\nKEY=second\n"))
	assertIssues(t, err, []Issue{
		{Line: 1, Category: CategoryDuplicateKey},
		{Line: 3, Category: CategoryDuplicateKey},
	})
}

func TestDecryptReplacesOnlyEnvelopeSpans(t *testing.T) {
	source := []byte("# heading\r\nAPI_TOKEN=ENVSEAL[v1:pbkdf2-sha256:1000000:AAAAAAAAAAAAAAAAAAAAAA:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA]\nPLAIN=keep\r\n")
	doc, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result, err := doc.Decrypt(func(key, value []byte) ([]byte, error) {
		if !bytes.Equal(key, []byte("API_TOKEN")) || !bytes.HasPrefix(value, []byte("ENVSEAL[")) {
			t.Fatal("Decrypt() passed an unexpected value span to open")
		}
		return []byte("plaintext"), nil
	})
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	want := []byte("# heading\r\nAPI_TOKEN=plaintext\nPLAIN=keep\r\n")
	if !bytes.Equal(result.Data, want) || result.Changed != 1 || result.Skipped != 0 {
		t.Fatalf("Decrypt() = %#v, want %q with one change", result, want)
	}
}

func TestCheckRejectsUnsupportedSyntaxAndMalformedEnvelope(t *testing.T) {
	doc, err := Parse([]byte("GOOD=value\nBAD=\"quoted\"\nSEALED=ENVSEAL[v2:bad]\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	assertIssues(t, doc.Check(), []Issue{
		{Line: 2, Category: CategoryUnsupportedSyntax},
		{Line: 3, Category: CategoryMalformedEnvelope},
	})
}

func FuzzEncryptPreservesOutsideValueSpan(f *testing.F) {
	f.Add([]byte("safe"))
	f.Add([]byte("raw-value"))
	f.Fuzz(func(t *testing.T, value []byte) {
		if bytes.ContainsAny(value, "\r\n") {
			return
		}
		source := append([]byte("# comment\r\nKEY="), value...)
		source = append(source, []byte("\nOTHER=unchanged\r\n")...)
		doc, err := Parse(source)
		if err != nil {
			return
		}
		result, err := doc.Encrypt([]string{"KEY"}, func(_, _ []byte) ([]byte, error) {
			return []byte("replacement"), nil
		})
		if err != nil {
			return
		}
		want := []byte("# comment\r\nKEY=replacement\nOTHER=unchanged\r\n")
		if !bytes.Equal(result.Data, want) {
			t.Fatalf("Encrypt() changed bytes outside the selected value: got %q, want %q", result.Data, want)
		}
	})
}

func assertIssues(t *testing.T, err error, want []Issue) {
	t.Helper()
	var dotenvErr *Error
	if !errors.As(err, &dotenvErr) {
		t.Fatalf("error = %v, want *Error", err)
	}
	if !reflect.DeepEqual(dotenvErr.Issues, want) {
		t.Fatalf("issues = %#v, want %#v", dotenvErr.Issues, want)
	}
}
