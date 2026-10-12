package state

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// The writes the seam stands for (docs/v2/stage3-state.md#what-is-a-writer):
// a call outside this package that makes one of these effects on the
// filesystem passes no guard. A reference counts, not only a call written
// where it is named — a value kept and called later is the same write — and
// so does one through any import name or a dot import.
var bypassEffects = map[string]map[string]bool{
	"os": set("WriteFile", "Create", "CreateTemp", "OpenFile", "Remove", "RemoveAll", "Rename",
		"Link", "Symlink", "Mkdir", "MkdirAll", "MkdirTemp", "Chmod", "Chown", "Lchown", "Chtimes",
		"Truncate", "CopyFS", "OpenRoot", "OpenInRoot"),
	"io/ioutil": set("WriteFile", "TempFile", "TempDir"),
	"syscall": set("Flock", "Mmap", "Unlink", "Unlinkat", "Rename", "Renameat", "Link", "Linkat",
		"Symlink", "Mkdir", "Mkdirat", "Rmdir", "Chmod", "Fchmod", "Fchmodat", "Truncate",
		"Ftruncate", "Open", "Openat", "Creat", "Mknod", "Mknodat", "Mkfifo", "Utimes", "UtimesNano",
		"Utime", "Futimes", "Futimesat", "Chown", "Fchown", "Fchownat", "Lchown", "Setxattr",
		"Removexattr", "Fallocate", "Pwrite"),
	"net": set("Listen", "ListenUnix", "ListenUnixgram", "ListenPacket"),
}

// descriptorEffects are the methods by which a descriptor changes its file
// whatever it was opened for: fchmod and fchown work on a read-only one. A
// content write needs a writable descriptor, which only an open counted here
// or a guarded one gives. Without type information every method of these
// names counts, whatever its receiver — a package's function aside — the safe
// side again.
var descriptorEffects = set("Chmod", "Chown")

// readFlags are the open flags, in os's spelling and syscall's, a read is
// spelled from: an open whose flag argument names only these writes nothing.
var readFlags = set("O_RDONLY", "O_NOFOLLOW", "O_NONBLOCK", "O_CLOEXEC", "O_DIRECTORY", "O_NOCTTY", "O_PATH")

// readProtection is the one protection a mapping that writes nothing asks.
var readProtection = set("PROT_READ")

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, name := range names {
		m[name] = true
	}
	return m
}

type bypassRef struct {
	effect string
	line   int
}

// bypassRefs lists the writes one file refers to. Without type information
// a dot-imported file's local of the same name counts too; erring that way
// is the safe side. Two calls are read by their arguments: os.OpenFile is a
// write only with a write flag, and a socket only on a path — an abstract
// address, "@" first, makes no file.
func bypassRefs(t *testing.T, path string) []bypassRef {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	aliases := map[string]string{}
	dotted := map[string]bool{}
	imported := map[string]bool{}
	for _, spec := range file.Imports {
		value, _ := strconv.Unquote(spec.Path.Value)
		if spec.Name != nil {
			imported[spec.Name.Name] = true
		} else {
			imported[value[strings.LastIndex(value, "/")+1:]] = true
		}
		if bypassEffects[value] == nil {
			continue
		}
		switch {
		case spec.Name == nil:
			aliases[value[strings.LastIndex(value, "/")+1:]] = value
		case spec.Name.Name == ".":
			dotted[value] = true
		case spec.Name.Name != "_":
			aliases[spec.Name.Name] = value
		}
	}
	effect := func(expr ast.Expr) string {
		switch expr := expr.(type) {
		case *ast.SelectorExpr:
			if x, ok := expr.X.(*ast.Ident); ok {
				if pkg := aliases[x.Name]; pkg != "" && bypassEffects[pkg][expr.Sel.Name] {
					return pkg + "." + expr.Sel.Name
				}
			}
		case *ast.Ident:
			for pkg := range dotted {
				if bypassEffects[pkg][expr.Name] {
					return pkg + "." + expr.Name
				}
			}
		}
		return ""
	}
	var found []bypassRef
	reads := map[ast.Expr]bool{}
	var visit func(ast.Node) bool
	visit = func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.CallExpr:
			if name := effect(node.Fun); name != "" && readOnly(name, node.Args) {
				reads[node.Fun] = true
			}
		case *ast.SelectorExpr:
			if name := effect(node); name != "" && !reads[node] {
				found = append(found, bypassRef{name, fset.Position(node.Pos()).Line})
			} else if x, ok := node.X.(*ast.Ident); descriptorEffects[node.Sel.Name] && (!ok || !imported[x.Name]) {
				found = append(found, bypassRef{"descriptor." + node.Sel.Name, fset.Position(node.Pos()).Line})
			}
			// What is selected from anything but a package is a field or a
			// method; the expression it is selected from is looked through.
			ast.Inspect(node.X, visit)
			return false
		case *ast.Ident:
			if name := effect(node); name != "" && !reads[node] {
				found = append(found, bypassRef{name, fset.Position(node.Pos()).Line})
			}
		}
		return true
	}
	ast.Inspect(file, visit)
	return found
}

// onlyNames answers whether an argument is spelled from the names allowed
// alone, combined as it may be: a variable, a number or any other name may
// be anything, so the call is taken for a write.
func onlyNames(arg ast.Expr, allowed map[string]bool) bool {
	only := true
	ast.Inspect(arg, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			only = only && allowed[node.Sel.Name]
			return false
		case *ast.Ident:
			only = only && allowed[node.Name]
		case *ast.BasicLit, *ast.CallExpr, *ast.IndexExpr, *ast.StarExpr:
			only = false
		}
		return true
	})
	return only
}

// readOnly answers for a call whose arguments show it writes nothing.
func readOnly(effect string, args []ast.Expr) bool {
	switch effect {
	case "os.OpenFile", "syscall.Open":
		return len(args) >= 2 && onlyNames(args[1], readFlags)
	case "syscall.Openat":
		return len(args) >= 3 && onlyNames(args[2], readFlags)
	case "syscall.Mmap":
		return len(args) >= 4 && onlyNames(args[3], readProtection)
	case "net.Listen", "net.ListenUnix", "net.ListenUnixgram", "net.ListenPacket":
		return len(args) >= 2 && (!unixNetwork(args[0]) || abstractAddress(args[1]))
	}
	return false
}

// unixNetwork answers whether a network argument may name a Unix socket: any
// expression but a literal naming another network.
func unixNetwork(arg ast.Expr) bool {
	lit, ok := arg.(*ast.BasicLit)
	if !ok {
		return true
	}
	network, _ := strconv.Unquote(lit.Value)
	return strings.HasPrefix(network, "unix")
}

// abstractAddress answers whether the address of a listen call is proven, by
// how it is spelled where the call is written, to begin with "@": a string
// literal that does, a concatenation whose first operand does, a UnixAddr
// whose Name does. Anything else — a variable, a call, whatever literal it
// holds inside — may be a path.
func abstractAddress(address ast.Expr) bool {
	switch address := address.(type) {
	case *ast.BasicLit:
		value, err := strconv.Unquote(address.Value)
		return address.Kind == token.STRING && err == nil && strings.HasPrefix(value, "@")
	case *ast.ParenExpr:
		return abstractAddress(address.X)
	case *ast.BinaryExpr:
		return address.Op == token.ADD && abstractAddress(address.X)
	case *ast.UnaryExpr:
		return address.Op == token.AND && abstractAddress(address.X)
	case *ast.CompositeLit:
		for index, element := range address.Elts {
			if field, ok := element.(*ast.KeyValueExpr); ok {
				if key, ok := field.Key.(*ast.Ident); ok && key.Name == "Name" {
					return abstractAddress(field.Value)
				}
			} else if index == 0 {
				return abstractAddress(element)
			}
		}
	}
	return false
}
