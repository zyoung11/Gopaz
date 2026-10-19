package main

import "testing"

// TestApplyCUDNNAlgoFromEnv checks the cuDNN algorithm override values.
func TestApplyCUDNNAlgoFromEnv(t *testing.T) {
	cases := []struct {
		value string
		want  int
		bad   bool
	}{
		{"", 0, false},
		{"exhaustive", 0, false},
		{"heuristic", 1, false},
		{"DEFAULT", 2, false},
		{"invalid", 0, true},
	}
	for _, c := range cases {
		t.Setenv("GOPAZ_CUDNN_ALGO", c.value)
		so := DefaultSessionOptions()
		err := applyCUDNNAlgoFromEnv(&so)
		if c.bad {
			if err == nil {
				t.Fatalf("%q: expected an error", c.value)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", c.value, err)
		}
		if so.CUDNNAlgo != c.want {
			t.Fatalf("%q: CUDNNAlgo = %d, want %d", c.value, so.CUDNNAlgo, c.want)
		}
	}
}
