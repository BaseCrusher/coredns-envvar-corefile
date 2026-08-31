package main

import "testing"

// render generates the Corefile and fails the test if any key was rejected.
func render(t *testing.T, env []string) string {
	t.Helper()
	gs, bad := groups(env)
	if len(bad) > 0 {
		t.Fatalf("unexpected rejected keys: %v", bad)
	}
	return corefile(gs)
}

func TestCorefile(t *testing.T) {
	env := []string{
		"COREDNS_MYZONE_ZONE=example.org",
		"COREDNS_MYZONE__acmednschallenge=test",
		"COREDNS_MYZONE__acmednschallenge__email=admin@example.org",
		"COREDNS_MYZONE__acmednschallenge__additionalSans=*.example.org",
		"COREDNS_MYZONE__file=db.example.org",
		"PATH=/bin", // ignored
	}
	got := render(t, env)
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
	got := render(t, env)
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
			got := render(t, env)
			want := "z.example.org:53 {\n" + c.want + "}\n\n"
			if got != want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, want)
			}
		})
	}
}

// An empty path segment (three or more consecutive '_') is rejected by name
// rather than silently flattening the child into the enclosing block.
func TestEmptySegmentRejected(t *testing.T) {
	env := []string{
		"COREDNS_Z_ZONE=z.example.org",
		"COREDNS_Z____a.example.org=1.2.3.4", // empty parent segment
		"COREDNS_Z__dir__=v",                 // empty trailing segment
		"COREDNS_Z__ok=fine",
	}
	gs, bad := groups(env)
	want := []string{"COREDNS_Z____a.example.org", "COREDNS_Z__dir__"}
	if len(bad) != len(want) || bad[0] != want[0] || bad[1] != want[1] {
		t.Fatalf("bad keys: got %v, want %v", bad, want)
	}
	// Valid vars in the same group still render.
	if got, want := corefile(gs), "z.example.org:53 {\n    ok fine\n}\n\n"; got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAtTokenAndRepeats(t *testing.T) {
	env := []string{
		"COREDNS_PROD_ZONE=example.com",
		"COREDNS_PROD__records___AT__1=60 IN SOA ns1.example.com. admin.example.com. 2025073111 3600 300 2419200 300",
		"COREDNS_PROD__records___AT__2=60 IN NS ns1.example.com.",
		"COREDNS_PROD__records___AT__3=60 IN A 192.0.2.1",
		"COREDNS_PROD__records___AT__4=60 IN A 192.0.2.2",
		"COREDNS_PROD__records___AT__5=60 IN A 192.0.2.3",
	}
	got := render(t, env)
	want := "example.com:53 {\n" +
		"    records {\n" +
		"        @ 60 IN SOA ns1.example.com. admin.example.com. 2025073111 3600 300 2419200 300\n" +
		"        @ 60 IN NS ns1.example.com.\n" +
		"        @ 60 IN A 192.0.2.1\n" +
		"        @ 60 IN A 192.0.2.2\n" +
		"        @ 60 IN A 192.0.2.3\n" +
		"    }\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAtNesting(t *testing.T) {
	env := []string{
		"COREDNS_Z_ZONE=z.example.org",
		"COREDNS_Z___AT___child=v",
	}
	got := render(t, env)
	want := "z.example.org:53 {\n" +
		"    @ {\n" +
		"        child v\n" +
		"    }\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRepeatDirective(t *testing.T) {
	env := []string{
		"COREDNS_Z_ZONE=z.example.org",
		"COREDNS_Z__file_1=a.db",
		"COREDNS_Z__file_2=b.db",
	}
	got := render(t, env)
	want := "z.example.org:53 {\n" +
		"    file a.db\n" +
		"    file b.db\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMultiple(t *testing.T) {
	env := []string{
		"COREDNS_Z_ZONE=z.example.org",
		"COREDNS_Z__record__A_MULTIPLE=1.2.3.4, 5.6.7.8,9.9.9.9",
	}
	got := render(t, env)
	want := "z.example.org:53 {\n" +
		"    record {\n" +
		"        A 1.2.3.4\n" +
		"        A 5.6.7.8\n" +
		"        A 9.9.9.9\n" +
		"    }\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEscapedTrailingNumber(t *testing.T) {
	env := []string{
		"COREDNS_Z_ZONE=z.example.org",
		"COREDNS_Z__ip6_2_=fd00::1",
	}
	got := render(t, env)
	want := "z.example.org:53 {\n" +
		"    ip6_2 fd00::1\n" +
		"}\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

// Single '_' is allowed in directive names; only '__' is reserved for nesting.
func TestUnderscoreInDirective(t *testing.T) {
	env := []string{
		"COREDNS_MYZONE_ZONE=example.org",
		"COREDNS_MYZONE__my_directive=some value",
		"COREDNS_MYZONE__parent_block__child_option=x",
	}
	got := render(t, env)
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
