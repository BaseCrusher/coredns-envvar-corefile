package main

import "testing"

func TestCorefile(t *testing.T) {
	env := []string{
		"COREDNS_MYZONE_ZONE=example.org",
		"COREDNS_MYZONE__acmednschallenge=test",
		"COREDNS_MYZONE__acmednschallenge__email=admin@example.org",
		"COREDNS_MYZONE__acmednschallenge__additionalSans=*.example.org",
		"COREDNS_MYZONE__file=db.example.org",
		"PATH=/bin", // ignored
	}
	got := corefile(groups(env))
	want := "example.org:53 {\n" +
		"    acmednschallenge test {\n" +
		"        additionalSans *.example.org\n" +
		"        email admin@example.org\n" +
		"    }\n" +
		"    file db.example.org\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

// Custom PORT, value-less directive, multiple groups sorted, a ZONE-less group
// that is skipped, and malformed vars that are ignored.
func TestHeaderAndSkips(t *testing.T) {
	env := []string{
		"COREDNS_BETA_ZONE=b.example.org", // second alphabetically -> after ALPHA
		"COREDNS_ALPHA_ZONE=a.example.org",
		"COREDNS_ALPHA_PORT=1053", // custom port
		"COREDNS_ALPHA__log=",     // value-less directive
		"COREDNS_NOZONE_PORT=53",  // no ZONE -> whole block skipped
		"COREDNS_",                // malformed: no field -> ignored
		"COREDNS_X_ZONE",          // malformed: no '=' key only -> ignored
	}
	got := corefile(groups(env))
	want := "a.example.org:1053 {\n" +
		"    log\n" +
		"}\n\n" +
		"b.example.org:53 {\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

// A directive's inline value and its nested block are independent: value shows
// only when non-empty, "{ ... }" shows only when it has children. All four
// constellations, plus an implicit parent (child defined, parent line skipped).
func TestDirectiveConstellations(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		want string // directive lines inside the block (4-space indented)
	}{
		{
			name: "value + children",
			env: []string{
				"COREDNS_Z__dir=arg",
				"COREDNS_Z__dir__opt=v",
			},
			want: "    dir arg {\n        opt v\n    }\n",
		},
		{
			name: "empty value + children",
			env: []string{
				"COREDNS_Z__dir=",
				"COREDNS_Z__dir__opt=v",
			},
			want: "    dir {\n        opt v\n    }\n",
		},
		{
			name: "value + no children",
			env:  []string{"COREDNS_Z__dir=arg"},
			want: "    dir arg\n",
		},
		{
			name: "empty value + no children",
			env:  []string{"COREDNS_Z__dir="},
			want: "    dir\n",
		},
		{
			name: "implicit parent (parent line skipped)",
			env:  []string{"COREDNS_Z__dir__opt=v"},
			want: "    dir {\n        opt v\n    }\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := append([]string{"COREDNS_Z_ZONE=z.example.org"}, c.env...)
			got := corefile(groups(env))
			want := "z.example.org:53 {\n" + c.want + "}\n\n"
			if got != want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, want)
			}
		})
	}
}

// Single '_' is allowed in directive names; only '__' is reserved for nesting.
func TestUnderscoreInDirective(t *testing.T) {
	env := []string{
		"COREDNS_MYZONE_ZONE=example.org",
		"COREDNS_MYZONE__my_directive=some value",
		"COREDNS_MYZONE__parent_block__child_option=x",
	}
	got := corefile(groups(env))
	want := "example.org:53 {\n" +
		"    my_directive some value\n" +
		"    parent_block {\n" +
		"        child_option x\n" +
		"    }\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}
