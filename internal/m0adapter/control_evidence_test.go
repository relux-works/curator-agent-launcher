package m0adapter

import (
	"go/ast"
	"go/build"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type controlInstance struct {
	name string
	run  func(*testing.T)
}

func runControlInstances(t *testing.T, cases []controlInstance) {
	t.Helper()
	if len(cases) == 0 {
		t.Fatal("no executable instances")
	}
	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}

// listedInstanceCount derives the fixture's semicolon-separated instance count,
// rather than treating an entire multi-instance row as one exercised case.
func listedInstanceCount(change string) int {
	if i := strings.Index(change, "in turn:"); i >= 0 {
		return len(strings.Split(change[i+len("in turn:"):], ";"))
	}
	return 1
}

func activeTopLevelTests(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	context := build.Default
	if raceEnabled {
		context.BuildTags = append(append([]string(nil), context.BuildTags...), "race")
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		active, err := context.MatchFile(".", name)
		if err != nil {
			t.Fatal(err)
		}
		if !active {
			continue
		}
		f, err := goparser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				out[fn.Name.Name] = true
			}
		}
	}
	return out
}

// argumentInput reports selectors of os.Args (including aliased imports) and
// argument-bearing Config fields. It examines production syntax, not comments.
func argumentInput(f *ast.File) bool {
	osName := ""
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if p == "os" {
			osName = "os"
			if imp.Name != nil {
				osName = imp.Name.Name
			}
		}
	}
	bad := osName == "."
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "Args" {
			if x, ok := s.X.(*ast.Ident); ok && x.Name == osName {
				bad = true
			}
		}
		if typ, ok := n.(*ast.TypeSpec); ok && typ.Name.Name == "Config" {
			if st, ok := typ.Type.(*ast.StructType); ok {
				for _, field := range st.Fields.List {
					for _, name := range field.Names {
						if strings.Contains(strings.ToLower(name.Name), "arg") {
							bad = true
						}
					}
				}
			}
		}
		return true
	})
	return bad
}

func closedRunnerSelection(f *ast.File) bool {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "runnerArgv" {
			continue
		}
		for _, stmt := range fn.Body.List {
			sw, ok := stmt.(*ast.SwitchStmt)
			if !ok {
				continue
			}
			tag, ok := sw.Tag.(*ast.SelectorExpr)
			if !ok || tag.Sel.Name != "RunnerCommand" {
				continue
			}
			cases := map[string]bool{}
			for _, item := range sw.Body.List {
				c := item.(*ast.CaseClause)
				if c.List == nil {
					ast.Inspect(c, func(n ast.Node) bool {
						if call, ok := n.(*ast.CallExpr); ok {
							if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "refuse" && len(call.Args) == 1 {
								if reason, ok := call.Args[0].(*ast.Ident); ok && reason.Name == "ReasonRunnerVariantPending" {
									cases["default"] = true
								}
							}
						}
						return true
					})
				}
				for _, value := range c.List {
					switch v := value.(type) {
					case *ast.Ident:
						cases[v.Name] = true
					case *ast.BasicLit:
						s, _ := strconv.Unquote(v.Value)
						cases[s] = true
					}
				}
			}
			return len(cases) == 3 && cases["codexRunnerCommand"] && cases["run-claude"] && cases["default"]
		}
	}
	return false
}

// TestRunnerSelectionClosed proves unknown literals have their own closed default, independent of the proposed variant seam.
func TestRunnerSelectionClosed(t *testing.T) {
	f, err := goparser.ParseFile(token.NewFileSet(), "mapping.go", nil, 0)
	if err != nil || !closedRunnerSelection(f) {
		t.Fatal("runner_selection_open")
	}
	bad, err := goparser.ParseFile(token.NewFileSet(), "control.go", `package adapter; func runnerArgv() { if lit.RunnerCommand == codexRunnerCommand { return }; claudeRunnerArgv() }`, 0)
	if err != nil || closedRunnerSelection(bad) {
		t.Fatal("open selection accepted by syntax control")
	}
}

func runnerArgumentsInstance(t *testing.T) {
	t.Helper()
	for _, dir := range []string{".", "../../cmd/curator-run-m0-adapter"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			f, err := goparser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			if argumentInput(f) {
				t.Fatal("runner_argument_input")
			}
		}
	}
	for _, c := range []struct {
		source string
		want   bool
	}{
		{`package adapter; type Config struct { Exec func() }`, false},
		{`package adapter; import process "os"; var selected = process.Args`, true},
		{`package adapter; type Config struct { Arguments []string }`, true},
	} {
		f, err := goparser.ParseFile(token.NewFileSet(), "control.go", c.source, 0)
		if err != nil || argumentInput(f) != c.want {
			t.Fatal("argument-input syntax control failed")
		}
	}
}
