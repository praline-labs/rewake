package server

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
)

// An answer goes out byte for byte when the whole message fits, and is
// replaced whole, as an error, when it does not.
func TestTheEncoderReplacesAnAnswerThatDoesNotFit(t *testing.T) {
	var out bytes.Buffer
	encoder := newEncoder(&out)
	stdout := "line \"quoted\" — and more\n"
	encoder.reply(json.RawMessage("7"), answer{stdout: stdout, stderr: "a warning\n", code: 3})
	var fitting struct {
		ID     int    `json:"id"`
		Result result `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &fitting); err != nil {
		t.Fatal(err)
	}
	want := []item{{"text", stdout}, {"text", "a warning\n"}, {"text", "exit status 3"}}
	if fitting.ID != 7 || !fitting.Result.IsError || len(fitting.Result.Content) != 3 || fitting.Result.Content[0] != want[0] || fitting.Result.Content[2] != want[2] {
		t.Fatalf("a fitting answer: %s", out.Bytes())
	}

	for _, size := range []int{bridge.ResultCap, bridge.ResultCap - 60} {
		out.Reset()
		encoder.reply(json.RawMessage(`"`+strings.Repeat("i", 126)+`"`), answer{stdout: strings.Repeat("\"", size/2)})
		if out.Len() > bridge.ResultCap+1 || !strings.Contains(out.String(), "did not fit one tool result") || !strings.Contains(out.String(), `"isError":true`) {
			t.Fatalf("an answer of %d bytes: %d bytes out: %.200s", size, out.Len(), out.String())
		}
	}

	out.Reset()
	encoder.fail(nil, codeParse, "the request is not JSON")
	if !strings.Contains(out.String(), `"id":null`) {
		t.Fatalf("an error with no id: %s", out.String())
	}
}

// A child's output is kept to its cap, and one that broke it is known.
func TestAChildsOutputIsCapped(t *testing.T) {
	var kept capped
	_, _ = kept.Write(bytes.Repeat([]byte("x"), outputCap-1))
	if kept.over {
		t.Fatal("over before the cap")
	}
	_, _ = kept.Write([]byte("yz"))
	if !kept.over || kept.Len() != outputCap {
		t.Fatalf("over %v, %d bytes", kept.over, kept.Len())
	}
}

// Only the encoder writes to the server's stdout: Run hands its writer to
// newEncoder and to nothing else, and no code of the package prints.
func TestOnlyTheEncoderWritesToStdout(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok {
				if pkg, ok := selector.X.(*ast.Ident); ok && (pkg.Name == "os" && selector.Sel.Name == "Stdout" || pkg.Name == "fmt" && strings.HasPrefix(selector.Sel.Name, "Print")) {
					t.Errorf("%s: %s.%s", fset.Position(node.Pos()), pkg.Name, selector.Sel.Name)
				}
			}
			run, ok := node.(*ast.FuncDecl)
			if !ok || run.Name.Name != "Run" {
				return true
			}
			ast.Inspect(run.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if ok {
					if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "newEncoder" {
						return false
					}
				}
				if ident, ok := inner.(*ast.Ident); ok && ident.Name == "stdout" {
					t.Errorf("%s: Run uses its stdout outside newEncoder", fset.Position(ident.Pos()))
				}
				return true
			})
			return false
		})
	}
}
