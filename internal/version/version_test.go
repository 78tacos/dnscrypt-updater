package version

import (
	"fmt"
	"testing"
)

func TestParseNormalizesVPrefix(t *testing.T) {
	t.Parallel()
	cases := []string{"2.1.18", "v2.1.18", "V2.1.18", "  v2.1.18  "}
	for _, in := range cases {
		in := in
		t.Run(fmt.Sprintf("%q", in), func(t *testing.T) {
			t.Parallel()
			v, err := Parse(in)
			if err != nil {
				t.Fatal(err)
			}
			if got := v.String(); got != "2.1.18" {
				t.Fatalf("String() = %q, want 2.1.18", got)
			}
			if got := Normalize(in); got != "2.1.18" {
				t.Fatalf("Normalize(%q) = %q", in, got)
			}
		})
	}
}

func TestParsePreReleaseAndBuild(t *testing.T) {
	t.Parallel()
	v, err := Parse("v2.1.18-beta.1+githash")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "2.1.18-beta.1" {
		t.Fatalf("got %q", v.String())
	}
	if len(v.Pre) != 2 || v.Pre[0] != "beta" || v.Pre[1] != "1" {
		t.Fatalf("pre = %#v", v.Pre)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	bads := []string{"", "   ", "v", "abc", "2.", ".1", "2.1.", "2.1.18-", "2.1.18-beta.", "x2.1.18", "2.1.18b"}
	for _, in := range bads {
		in := in
		t.Run(fmt.Sprintf("%q", in), func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(in); err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", in)
			}
		})
	}
}

func TestCompareTable(t *testing.T) {
	t.Parallel()
	type row struct {
		a, b string
		want int
	}
	rows := []row{
		{"2.1.18", "2.1.18", 0},
		{"v2.1.18", "2.1.18", 0},
		{"2.1.18", "v2.1.18", 0},
		{"2.1", "2.1.0", 0},
		{"2.1.0", "2.1", 0},
		{"2.1.18", "2.1.17", 1},
		{"2.1.17", "2.1.18", -1},
		{"2.1.18", "2.0.99", 1},
		{"2.0.99", "2.1.18", -1},
		{"2.1.18", "2.1.18-beta1", 1},
		{"2.1.18-beta1", "2.1.18", -1},
		{"2.1.18-rc.1", "2.1.18", -1},
		{"2.1.18", "2.1.18-rc.1", 1},
		{"2.1.18-alpha", "2.1.18-beta", -1},
		{"2.1.18-beta", "2.1.18-alpha", 1},
		{"2.1.18-beta.2", "2.1.18-beta.11", -1},
		{"2.1.18-beta.11", "2.1.18-beta.2", 1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-beta.2", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-beta.11", "1.0.0-rc.1", -1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"10.0.0", "9.9.9", 1},
		{"2.1.18+build.1", "2.1.18+build.2", 0},
		{"2.1.18+aaa", "2.1.18", 0},
		{"2.2", "2.1.99", 1},
		{"3", "2.9.9", 1},
		{"2.1.18-1", "2.1.18-beta", -1}, // numeric pre-release < non-numeric
		{"  2.1.18  ", "2.1.18", 0},
	}
	for _, r := range rows {
		r := r
		t.Run(fmt.Sprintf("%s_vs_%s", r.a, r.b), func(t *testing.T) {
			t.Parallel()
			got, err := CompareStrings(r.a, r.b)
			if err != nil {
				t.Fatal(err)
			}
			if got != r.want {
				t.Fatalf("Compare(%q, %q) = %d, want %d", r.a, r.b, got, r.want)
			}
			rev, err := CompareStrings(r.b, r.a)
			if err != nil {
				t.Fatal(err)
			}
			if rev != -r.want {
				t.Fatalf("reverse Compare(%q, %q) = %d, want %d", r.b, r.a, rev, -r.want)
			}
		})
	}
}

func TestGreaterRemoteVsLocal(t *testing.T) {
	t.Parallel()
	remote, err := Parse("2.1.18")
	if err != nil {
		t.Fatal(err)
	}
	local, err := Parse("v2.1.14")
	if err != nil {
		t.Fatal(err)
	}
	if !Greater(remote, local) {
		t.Fatal("expected remote > local")
	}
	if Greater(local, remote) {
		t.Fatal("did not expect local > remote")
	}
	same, _ := Parse("v2.1.18")
	if Greater(remote, same) {
		t.Fatal("equal tags must not count as an update")
	}
}

func TestCompareStringsError(t *testing.T) {
	t.Parallel()
	if _, err := CompareStrings("nope", "2.1.18"); err == nil {
		t.Fatal("want error for bad left")
	}
	if _, err := CompareStrings("2.1.18", "nope"); err == nil {
		t.Fatal("want error for bad right")
	}
}

func TestZeroVersion(t *testing.T) {
	t.Parallel()
	var z Version
	if !z.IsZero() {
		t.Fatal("empty Version should be zero")
	}
	if z.String() != "" {
		t.Fatalf("zero String = %q", z.String())
	}
}
