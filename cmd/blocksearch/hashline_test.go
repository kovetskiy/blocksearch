package main

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestHashlineHashesKnownValues(t *testing.T) {
	hashes := hashlineHashes([]byte("alpha\nbeta\ngamma\n"))
	want := []string{"A9", "5E", "3E", "1A"}

	if !reflect.DeepEqual(hashes, want) {
		t.Fatalf("hashlineHashes = %#v, want %#v", hashes, want)
	}
}

func TestHashlineHashesStripBOMAndNormalizeCRLF(t *testing.T) {
	plain := hashlineHashes([]byte("alpha\nbeta\ngamma\n"))
	bomCRLF := hashlineHashes([]byte("\xef\xbb\xbfalpha\r\nbeta\r\ngamma\r\n"))
	bomCR := hashlineHashes([]byte("\xef\xbb\xbfalpha\rbeta\rgamma\r"))

	if !reflect.DeepEqual(bomCRLF, plain) {
		t.Fatalf("hashes with BOM and CRLF = %#v, want %#v", bomCRLF, plain)
	}
	if !reflect.DeepEqual(bomCR, plain) {
		t.Fatalf("hashes with BOM and CR = %#v, want %#v", bomCR, plain)
	}
}

func TestHashlineFormatUsesEditableAnchor(t *testing.T) {
	block := Block{
		{Line: 9, Hash: "0A", Text: "one"},
		{Line: 10, Hash: "F2", Text: "two"},
	}

	formatted := block.Format(FormatOptions{Hashline: true})
	if !regexp.MustCompile(`(?m)^\s*9#[0-9A-F]{2}│one$`).MatchString(formatted) {
		t.Fatalf("formatted hashline block %q does not contain an editable anchor", formatted)
	}
	if !regexp.MustCompile(`(?m)^10#[0-9A-F]{2}│two$`).MatchString(formatted) {
		t.Fatalf("formatted hashline block %q does not contain an unpadded max-width anchor", formatted)
	}
}

func TestBlockExportIncludesLineHashes(t *testing.T) {
	block := Block{
		{Line: 3, Hash: "AA", Text: "func f() {"},
		{Line: 4, Hash: "BB", Text: "}"},
	}

	encoded, err := block.EncodeJSON("function.go", true)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}

	var exported BlockExport
	if err := json.Unmarshal(encoded, &exported); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if !reflect.DeepEqual(exported.LineHashes, []string{"AA", "BB"}) {
		t.Fatalf("line_hashes = %#v, want %#v", exported.LineHashes, []string{"AA", "BB"})
	}
}

func TestBlockExportSuppressesLineHashes(t *testing.T) {
	block := Block{
		{Line: 3, Hash: "AA", Text: "func f() {"},
		{Line: 4, Hash: "BB", Text: "}"},
	}

	encoded, err := block.EncodeJSON("function.go", false)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}

	var exported BlockExport
	if err := json.Unmarshal(encoded, &exported); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if exported.LineHashes != nil {
		t.Fatalf("line_hashes = %#v, want nil (key omitted) when hashline is false", exported.LineHashes)
	}

	// The raw JSON must not contain the key at all.
	if strings.Contains(string(encoded), `"line_hashes"`) {
		t.Fatalf("encoded JSON contains line_hashes key, want it omitted: %s", encoded)
	}
}
