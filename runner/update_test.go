package main

import "testing"

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"4.18.18", "4.18.18", 0},
		{"4.18.18", "4.18.19", -1},
		{"4.19.0", "4.18.18", 1},
		{"4.18.18", "4.18", 1},
		{"v4.18.18", "4.18.18", 0},
		{"", "4.18", -1}, // unparseable falls back to string compare ("" < "4.18")
	}
	for _, c := range cases {
		if got := versionCompare(c.a, c.b); got != c.want {
			t.Fatalf("versionCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
