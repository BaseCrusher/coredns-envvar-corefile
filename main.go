package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// prefix marks env vars this tool reads. Per <GROUP> (each group is one server
// block; group names hold no '_'):
//
//	COREDNS_<GROUP>_ZONE / _PORT       top-level: the block header <ZONE>:<PORT>
//	COREDNS_<GROUP>__A[__B[__C...]]    a directive inside the block; each extra
//	                                   '__' nests one level deeper into A { B {...} }
const prefix = "COREDNS_"

// node is one directive: its inline value plus any nested directives.
type node struct {
	value    string
	children map[string]*node
}

func newNode() *node { return &node{children: map[string]*node{}} }

// set stores value at the end of path, creating intermediate nodes.
func (n *node) set(path []string, value string) {
	cur := n
	for _, seg := range path {
		if cur.children[seg] == nil {
			cur.children[seg] = newNode()
		}
		cur = cur.children[seg]
	}
	cur.value = value
}

// block is one server block: header fields plus the tree of inside directives.
type block struct {
	zone, port string
	root       *node
}

// groups parses COREDNS_ env vars into group name -> block.
func groups(environ []string) map[string]*block {
	out := map[string]*block{}
	get := func(g string) *block {
		if out[g] == nil {
			out[g] = &block{root: newNode()}
		}
		return out[g]
	}
	for _, e := range environ {
		if !strings.HasPrefix(e, prefix) {
			continue
		}
		k, v, _ := strings.Cut(e, "=")
		k = strings.TrimPrefix(k, prefix)

		g, rest, ok := strings.Cut(k, "_")
		if !ok || g == "" || rest == "" {
			continue
		}
		if cfg, ok := strings.CutPrefix(rest, "_"); ok { // '__' -> inside block
			if path := splitPath(cfg); len(path) > 0 {
				get(g).root.set(path, v)
			}
			continue
		}
		switch rest { // single '_' -> header field
		case "ZONE":
			get(g).zone = v
		case "PORT":
			get(g).port = v
		}
	}
	return out
}

// splitPath turns "ACME__EMAIL" into ["ACME","EMAIL"], dropping empty segments.
func splitPath(s string) []string {
	var out []string
	for _, seg := range strings.Split(s, "__") {
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// corefile renders groups into a Corefile. ZONE is required; PORT defaults to 53.
func corefile(gs map[string]*block) string {
	var b strings.Builder
	for _, g := range sortedKeys(gs) {
		blk := gs[g]
		if blk.zone == "" {
			continue // ponytail: a block with no ZONE has no header; skip it
		}
		port := blk.port
		if port == "" {
			port = "53"
		}
		fmt.Fprintf(&b, "%s:%s {\n", blk.zone, port)
		renderChildren(&b, blk.root, "    ")
		b.WriteString("}\n\n")
	}
	return b.String()
}

// renderChildren writes n's directives, recursing into nested blocks.
func renderChildren(b *strings.Builder, n *node, indent string) {
	for _, k := range sortedKeys(n.children) {
		child := n.children[k]
		line := k // directive name is the env segment, case preserved
		if child.value != "" {
			line += " " + child.value
		}
		if len(child.children) > 0 {
			fmt.Fprintf(b, "%s%s {\n", indent, line)
			renderChildren(b, child, indent+"    ")
			fmt.Fprintf(b, "%s}\n", indent)
		} else {
			fmt.Fprintf(b, "%s%s\n", indent, line)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// main writes the Corefile to the path given as the first argument, or to
// stdout when none is given (distroless images have no shell to redirect with).
func main() {
	out := corefile(groups(os.Environ()))
	if len(os.Args) < 2 {
		fmt.Print(out)
		return
	}
	if err := os.WriteFile(os.Args[1], []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
