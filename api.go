package gocognit

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const LinterName = "gocognit"

type Options struct {
	Over                  int
	SkipTests             bool
	IncludeDiagnostics    bool
	ExcludePathSubstrings []string
}

type Finding struct {
	Path        string       `json:"Path"`
	Line        int          `json:"Line"`
	Column      int          `json:"Column"`
	Offset      int          `json:"Offset,omitempty"`
	Message     string       `json:"Message"`
	PkgName     string       `json:"PkgName,omitempty"`
	FuncName    string       `json:"FuncName,omitempty"`
	Complexity  int          `json:"Complexity,omitempty"`
	Over        int          `json:"Over,omitempty"`
	Diagnostics []Diagnostic `json:"Diagnostics,omitempty"`
}

type GolangCILintJSON struct {
	Issues []GolangCILintIssue    `json:"Issues"`
	Report GolangCILintJSONReport `json:"Report"`
}

type GolangCILintJSONReport struct {
	Linters []GolangCILintLinter `json:"Linters"`
}

type GolangCILintLinter struct {
	Name    string `json:"Name"`
	Enabled bool   `json:"Enabled,omitempty"`
}

type GolangCILintIssue struct {
	FromLinter           string               `json:"FromLinter"`
	Text                 string               `json:"Text"`
	Severity             string               `json:"Severity"`
	SourceLines          []string             `json:"SourceLines"`
	Pos                  GolangCILintPosition `json:"Pos"`
	ExpectNoLint         bool                 `json:"ExpectNoLint"`
	ExpectedNoLintLinter string               `json:"ExpectedNoLintLinter"`
}

type GolangCILintPosition struct {
	Filename string `json:"Filename"`
	Offset   int    `json:"Offset"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

func DefaultOptions() Options {
	return Options{}
}

func CheckPaths(paths []string, opts Options) ([]Finding, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	var files []string
	for _, path := range paths {
		if pathExcluded(path, opts.ExcludePathSubstrings) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !opts.SkipTests || !strings.HasSuffix(path, "_test.go") {
				files = append(files, path)
			}
			continue
		}

		err = filepath.Walk(path, func(filename string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if pathExcluded(filename, opts.ExcludePathSubstrings) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(filename, ".go") {
				return nil
			}
			if opts.SkipTests && strings.HasSuffix(filename, "_test.go") {
				return nil
			}
			if pathExcluded(filename, opts.ExcludePathSubstrings) {
				return nil
			}
			files = append(files, filename)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return CheckFiles(files, opts)
}

func CheckFiles(files []string, opts Options) ([]Finding, error) {
	var findings []Finding
	for _, filename := range files {
		if opts.SkipTests && strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if pathExcluded(filename, opts.ExcludePathSubstrings) {
			continue
		}
		next, err := checkFile(filename, opts)
		if err != nil {
			return nil, err
		}
		findings = append(findings, next...)
	}
	SortFindings(findings)
	return findings, nil
}

func pathExcluded(path string, substrings []string) bool {
	for _, substring := range substrings {
		if substring != "" && strings.Contains(path, substring) {
			return true
		}
	}
	return false
}

func CheckGolangCILintJSON(paths []string, opts Options) (GolangCILintJSON, error) {
	findings, err := CheckPaths(paths, opts)
	if err != nil {
		return GolangCILintJSON{}, err
	}
	return FindingsToGolangCILintJSON(findings), nil
}

func FindingsToGolangCILintJSON(findings []Finding) GolangCILintJSON {
	SortFindings(findings)
	issues := make([]GolangCILintIssue, 0, len(findings))
	for _, finding := range findings {
		issues = append(issues, finding.GolangCILintIssue())
	}
	return GolangCILintJSON{
		Issues: issues,
		Report: GolangCILintJSONReport{
			Linters: []GolangCILintLinter{
				{Name: LinterName, Enabled: true},
			},
		},
	}
}

func (finding Finding) GolangCILintIssue() GolangCILintIssue {
	return GolangCILintIssue{
		FromLinter:   LinterName,
		Text:         finding.Message,
		Severity:     "",
		SourceLines:  []string{},
		Pos:          GolangCILintPosition{Filename: finding.Path, Offset: finding.Offset, Line: finding.Line, Column: finding.Column},
		ExpectNoLint: false,
	}
}

func (output GolangCILintJSON) Findings() []Finding {
	findings := make([]Finding, 0, len(output.Issues))
	for _, issue := range output.Issues {
		findings = append(findings, issue.Finding())
	}
	return findings
}

func (issue GolangCILintIssue) Finding() Finding {
	return Finding{
		Path:    issue.Pos.Filename,
		Line:    issue.Pos.Line,
		Column:  issue.Pos.Column,
		Offset:  issue.Pos.Offset,
		Message: issue.Text,
	}
}

func SortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		if findings[i].Column != findings[j].Column {
			return findings[i].Column < findings[j].Column
		}
		if findings[i].Complexity != findings[j].Complexity {
			return findings[i].Complexity > findings[j].Complexity
		}
		return findings[i].Message < findings[j].Message
	})
}

func checkFile(filename string, opts Options) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	stats := ComplexityStatsWithDiagnostic(file, fset, nil, opts.IncludeDiagnostics)
	findings := make([]Finding, 0, len(stats))
	for _, stat := range stats {
		if stat.Complexity <= opts.Over {
			continue
		}
		findings = append(findings, Finding{
			Path:        stat.Pos.Filename,
			Line:        stat.Pos.Line,
			Column:      stat.Pos.Column,
			Offset:      stat.Pos.Offset,
			Message:     fmt.Sprintf("cognitive complexity %d of func %s is high (> %d)", stat.Complexity, stat.FuncName, opts.Over),
			PkgName:     stat.PkgName,
			FuncName:    stat.FuncName,
			Complexity:  stat.Complexity,
			Over:        opts.Over,
			Diagnostics: stat.Diagnostics,
		})
	}
	return findings, nil
}
