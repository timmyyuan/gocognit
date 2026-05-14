package gocognit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFilesGolangCILintJSON(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "sample.go")
	src := `package sample

func complex(x int) {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				println(i)
			}
		}
	}
}
`
	if err := os.WriteFile(filename, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	findings, err := CheckFiles([]string{filename}, Options{Over: 0, IncludeDiagnostics: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Path != filename {
		t.Fatalf("unexpected path: %q", findings[0].Path)
	}
	if findings[0].Complexity == 0 {
		t.Fatal("expected positive complexity")
	}

	report := FindingsToGolangCILintJSON(findings)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	var decoded GolangCILintJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	roundTrip := decoded.Findings()
	if len(roundTrip) != len(findings) {
		t.Fatalf("expected %d round-trip findings, got %d", len(findings), len(roundTrip))
	}
	if roundTrip[0].Path != findings[0].Path || roundTrip[0].Line != findings[0].Line {
		t.Fatalf("round-trip mismatch: %#v != %#v", roundTrip[0], findings[0])
	}
}
