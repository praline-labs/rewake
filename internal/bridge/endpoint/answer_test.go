package endpoint

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The bound test of a transport's answers (answer.go), rebuilt from
// bridge/server's encoder and bound tests: an answer leaves whole and as it
// is when it fits, is replaced whole when it does not, and a request is
// bounded before it is parsed.

// An answer goes out as it is when it fits, and is replaced whole, as an
// error, when it does not.
func TestTheEncoderReplacesAnAnswerThatDoesNotFit(t *testing.T) {
	stdout := "line \"quoted\" — and more\n"
	fitting := childAnswer{stdout: stdout, stderr: "a warning\n", code: 3}.encoded()
	want := []string{stdout, "a warning\n", "exit status 3"}
	if !fitting.IsError || len(fitting.Texts) != 3 || fitting.Texts[0] != want[0] || fitting.Texts[1] != want[1] || fitting.Texts[2] != want[2] {
		t.Fatalf("a fitting answer: %+v", fitting)
	}
	for _, size := range []int{bridge.ResultCap, bridge.ResultCap - 20} {
		answer := childAnswer{stdout: strings.Repeat("\"", size/2)}.encoded()
		encoded, _ := json.Marshal(answer)
		if len(encoded) > bridge.ResultCap || !answer.IsError || len(answer.Texts) != 1 || answer.Texts[0] != overBound {
			t.Fatalf("an answer of %d bytes: %d bytes out: %.200s", size, len(encoded), encoded)
		}
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

// Only the encoder makes a transport's answer: a ToolAnswer that carries
// anything is built only by result, result is called only by encoded, and every answer runCall returns
// is one that encoded made. A path that wrote an answer of its own would leave
// unchecked against the bound.
func TestOnlyTheEncoderAnswers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := gotoken.NewFileSet()
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
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.CompositeLit:
					if ident, ok := node.Type.(*ast.Ident); ok && ident.Name == "ToolAnswer" && len(node.Elts) > 0 && fn.Name.Name != "result" {
						t.Errorf("%s: %s builds a ToolAnswer", fset.Position(node.Pos()), fn.Name.Name)
					}
				case *ast.SelectorExpr:
					if node.Sel.Name == "result" && fn.Name.Name != "encoded" {
						t.Errorf("%s: %s calls result outside the encoder", fset.Position(node.Pos()), fn.Name.Name)
					}
				case *ast.ReturnStmt:
					if fn.Name.Name != "runCall" {
						break
					}
					for _, value := range node.Results {
						var selector *ast.SelectorExpr
						if call, ok := value.(*ast.CallExpr); ok {
							selector, _ = call.Fun.(*ast.SelectorExpr)
						}
						if selector == nil || selector.Sel.Name != "encoded" {
							t.Errorf("%s: runCall returns an answer the encoder did not make", fset.Position(value.Pos()))
						}
					}
				}
				return true
			})
		}
	}
}

func FuzzEveryAnswerFits(f *testing.F) {
	dense := strings.Repeat("\x01", 900)
	for _, seed := range []struct {
		stdout, stderr string
		code           int
		substituted    bool
	}{
		{"plain ascii\n", "", 0, false},
		{strings.Repeat("кириллица ", 300), "предупреждение\n", 1, false},
		{strings.Repeat("🙂", 900), strings.Repeat("🙂", 100), 2, false},
		{strings.Repeat(`"\`, 1500), "", 0, false},
		{dense, dense, 3, false},
		{strings.Repeat("x", bridge.ResultCap-60), "", 0, false},
		{strings.Repeat("x", bridge.ResultCap), "", 0, false},
		{strings.Repeat("<>&", 800), "  ", 255, false},
		{"Rewake: the endpoint's own line\n", "", 0, true},
	} {
		f.Add(seed.stdout, seed.stderr, seed.code, seed.substituted)
	}
	f.Fuzz(func(t *testing.T, stdout, stderr string, code int, substituted bool) {
		child := childAnswer{stdout: stdout, stderr: stderr, code: code, substituted: substituted}
		answer := child.encoded()
		encoded, err := json.Marshal(answer)
		if err != nil || len(encoded) > bridge.ResultCap {
			t.Fatalf("an answer of %d bytes: %v", len(encoded), err)
		}
		// The whole response line the transport reads carries it too.
		line, _ := json.Marshal(response{ID: 1, Answer: &answer})
		var read response
		if err := json.Unmarshal(line, &read); err != nil || read.Answer == nil {
			t.Fatalf("a response that does not parse: %v", err)
		}
		// An answer is the child's byte for byte, or the replacement, an
		// error.
		if len(answer.Texts) == 1 && answer.Texts[0] == overBound && (stdout != overBound || stderr != "" || code != 0) {
			if !answer.IsError {
				t.Fatal("the replacement is not an error")
			}
			return
		}
		whole := child.result()
		if len(answer.Texts) != len(whole.Texts) || answer.IsError != (substituted || code != 0) {
			t.Fatalf("the answer changed on the way: %+v", answer)
		}
		for i := range whole.Texts {
			if answer.Texts[i] != whole.Texts[i] {
				t.Fatalf("text %d changed on the way: %q", i, answer.Texts[i])
			}
		}
	})
}

// counting is a reader that counts what was taken from it.
type counting struct {
	r io.Reader
	n int
}

func (c *counting) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func FuzzRequestsAreBounded(f *testing.F) {
	for _, seed := range []struct {
		size int
		fill string
	}{
		{1, "x"},
		{maxCall, "x"},
		{maxCall + 1, "x"},
		{maxCall - 1, "ж"},
		{maxCall + 3, "🙂"},
		{3 * maxCall, "\r"},
		// A line that ends in a carriage return, just past the bound with it.
		{maxCall + 1, "\r0"},
		{maxCall - 97, "\r0"},
	} {
		f.Add(seed.size, seed.fill)
	}
	f.Fuzz(func(t *testing.T, size int, fill string) {
		if fill == "" || strings.ContainsAny(fill, "\n") || size < 1 || size > 4*maxCall {
			return
		}
		long := strings.Repeat(fill, size/len(fill)+1)[:size]
		source := &counting{r: io.MultiReader(strings.NewReader(long+"\n"), strings.NewReader("after\n"))}
		const buffer = 4096
		line, err := readBounded(bufio.NewReaderSize(source, buffer), maxCall)
		// A carriage return that ends the line goes with its newline.
		content := strings.TrimSuffix(long, "\r")
		switch {
		case len(content) > maxCall:
			if !errors.Is(err, errOverBound) {
				t.Fatalf("a request of %d bytes: %v, %d bytes", size, err, len(line))
			}
			// Reading stops past the bound: no more than one buffer beyond
			// it is ever taken.
			if source.n > maxCall+2*buffer {
				t.Fatalf("a request of %d bytes: %d bytes read", size, source.n)
			}
		default:
			if err != nil || string(line) != content {
				t.Fatalf("a request of %d bytes: %v, %d bytes", size, err, len(line))
			}
		}
	})
}
