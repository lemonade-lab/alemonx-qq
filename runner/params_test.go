package main

import "testing"

func TestLinesParam(t *testing.T) {
	for _, value := range []string{"0", "5001", "abc"} {
		if _, err := linesParam(map[string]string{"lines": value}); err == nil {
			t.Fatalf("lines %q must be rejected", value)
		}
	}
	if got, err := linesParam(map[string]string{"lines": "100"}); err != nil || got != 100 {
		t.Fatalf("lines = %d, %v", got, err)
	}
	if got, err := linesParam(map[string]string{}); err != nil || got != 200 {
		t.Fatalf("default lines = %d, %v", got, err)
	}
}

func TestStatusLineNotInstalled(t *testing.T) {
	line := statusLine(State{})
	if line == "" || line[0] != '?' {
		t.Fatalf("uninstalled status should start with '?', got %q", line)
	}
}
