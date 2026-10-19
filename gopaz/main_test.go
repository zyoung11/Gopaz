package main

import "testing"

func TestGlobalFlagAction(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-h"}, "help"},
		{[]string{"--help"}, "help"},
		{[]string{"upscale", "-i", "a.mp4", "--help"}, "help"},
		{[]string{"interpolate", "-h"}, "help"},
		{[]string{"-v"}, "version"},
		{[]string{"--version"}, "version"},
		{[]string{"-V"}, "version"},
		{[]string{"serve", "--port", "8765", "-v"}, "version"},
		{[]string{"upscale", "-i", "a.mp4"}, ""},
		{[]string{}, ""},
	}
	for _, c := range cases {
		if got := globalFlagAction(c.args); got != c.want {
			t.Errorf("globalFlagAction(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestVersionFormat(t *testing.T) {
	if version == "" || version[0] != 'v' {
		t.Fatalf("version = %q, want v-prefixed", version)
	}
}
