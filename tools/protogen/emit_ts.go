package main

import (
	"fmt"
	"strings"
)

// emitTS writes the mod's side: the paths, what each endpoint is asked and
// answers, the bounds, and a type per definition. An optional field is marked
// with ?, as the Go side leaves it out when absent.
func emitTS(s schema) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s\n//\n", header)
	writeComment(&b, "// ", s.title+". "+s.description)
	b.WriteString("\n/** The paths of the protocol's endpoints, each a POST. */\nexport const paths = {\n")
	for _, e := range s.endpoints {
		parts := strings.Split(e.path, "/")
		key := parts[0] + upperCamel(parts[1:])
		writeComment(&b, "  // ", key+" "+e.description)
		fmt.Fprintf(&b, "  %s: %q,\n", key, "/v1/"+e.path)
	}
	b.WriteString("} as const\n\n/** What each endpoint is asked and answers. */\nexport interface Endpoints {\n")
	for _, e := range s.endpoints {
		fmt.Fprintf(&b, "  %q: { request: %s; answer: %s }\n", "/v1/"+e.path, e.request, e.answer)
	}
	b.WriteString("}\n\n/** The protocol's bounds. */\nexport const bounds = {\n")
	for _, bound := range s.bounds {
		writeComment(&b, "  // ", bound.name+" "+bound.description)
		fmt.Fprintf(&b, "  %s: %d,\n", bound.name, bound.value)
	}
	b.WriteString("} as const\n")
	for _, d := range s.defs {
		b.WriteString("\n")
		writeComment(&b, "// ", d.name+" "+d.description)
		switch {
		case d.enum != nil:
			quoted := make([]string, len(d.enum))
			for i, value := range d.enum {
				quoted[i] = fmt.Sprintf("%q", value)
			}
			fmt.Fprintf(&b, "export type %s = %s\n", d.name, strings.Join(quoted, " | "))
		case len(d.fields) == 0:
			fmt.Fprintf(&b, "export type %s = Record<string, never>\n", d.name)
		default:
			fmt.Fprintf(&b, "export interface %s {\n", d.name)
			for _, f := range d.fields {
				writeComment(&b, "  // ", f.name+" "+f.description)
				optional := ""
				if !f.required {
					optional = "?"
				}
				fmt.Fprintf(&b, "  %s%s: %s\n", f.name, optional, tsType(f.shape))
			}
			b.WriteString("}\n")
		}
	}
	return []byte(b.String())
}

func tsType(s shape) string {
	base := map[string]string{"string": "string", "integer": "number", "boolean": "boolean"}[s.scalar]
	if s.ref != "" {
		base = s.ref
	}
	if s.array {
		return base + "[]"
	}
	return base
}
