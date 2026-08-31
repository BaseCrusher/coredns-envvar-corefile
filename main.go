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

// groups parses COREDNS_ env vars into group name -> block, plus the sorted
// list of keys rejected by splitPath.
func groups(environ []string) (map[string]*block, []string) {
	out := map[string]*block{}
	var bad []string
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
			cfg = strings.ReplaceAll(cfg, "_AT_", "@")
			path, ok := splitPath(cfg)
			if !ok {
				bad = append(bad, prefix+k)
				continue
			}
			get(g).root.set(path, v)
			continue
		}
		switch rest { // single '_' -> header field
		case "ZONE":
			get(g).zone = v
		case "PORT":
			get(g).port = v
		}
	}
	sort.Strings(bad)
	return out, bad
}

// splitPath turns "ACME__EMAIL" into ["ACME","EMAIL"]. An empty segment means
// three or more consecutive '_' (COREDNS_Z____a), which used to be dropped: the
// nameless parent vanished and its child was rendered flat, yielding a Corefile
// CoreDNS rejects with an unrelated-looking parse error. Report it instead.
func splitPath(s string) ([]string, bool) {
	out := strings.Split(s, "__")
	for _, seg := range out {
		if seg == "" {
			return nil, false
		}
	}
	return out, true
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

func displayName(k string) string {
	if n := len(k); n >= 2 && k[n-1] == '_' && k[n-2] >= '0' && k[n-2] <= '9' {
		return k[:n-1]
	}
	base, indexed := trimIndex(k)
	if indexed {
		return strings.TrimSuffix(base, "_")
	}
	return base
}

func trimIndex(k string) (string, bool) {
	i := len(k)
	for i > 0 && k[i-1] >= '0' && k[i-1] <= '9' {
		i--
	}
	if i > 0 && i < len(k) && k[i-1] == '_' {
		return k[:i], true
	}
	return k, false
}

// renderChildren writes n's directives, recursing into nested blocks.
func renderChildren(b *strings.Builder, n *node, indent string) {
	for _, k := range sortedKeys(n.children) {
		child := n.children[k]
		name := displayName(k) // directive name is the env segment, case preserved
		// _MULTIPLE on a leaf: split value by comma, one line per value (e.g. many A records).
		if base, ok := strings.CutSuffix(name, "_MULTIPLE"); ok && len(child.children) == 0 {
			for _, v := range strings.Split(child.value, ",") {
				fmt.Fprintf(b, "%s%s %s\n", indent, base, strings.TrimSpace(v))
			}
			continue
		}
		line := name
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
	gs, bad := groups(os.Environ())
	if len(bad) > 0 {
		fmt.Fprintf(os.Stderr, "empty directive name (three or more consecutive '_') in: %s\n", strings.Join(bad, ", "))
		os.Exit(1)
	}
	out := corefile(gs)
	if len(os.Args) < 2 {
		fmt.Print(out)
		return
	}
	if err := os.WriteFile(os.Args[1], []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
