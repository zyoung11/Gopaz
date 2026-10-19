package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDedupePath(t *testing.T) {
	dir := t.TempDir()
	base := "11月18日 (3)"
	suffix := "_apo8"
	got := dedupePath(dir, base, suffix, "mp4")
	want := filepath.Join(dir, "11月18日 (3)_apo8.mp4")
	if got != want {
		t.Fatalf("dedupePath = %q, want %q", got, want)
	}
	if err := os.WriteFile(got, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got = dedupePath(dir, base, suffix, "mp4")
	want = filepath.Join(dir, "11月18日 (3)(1)_apo8.mp4")
	if got != want {
		t.Fatalf("dedupePath first collision = %q, want %q", got, want)
	}
	if err := os.WriteFile(got, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got = dedupePath(dir, base, suffix, "mp4")
	want = filepath.Join(dir, "11月18日 (3)(2)_apo8.mp4")
	if got != want {
		t.Fatalf("dedupePath second collision = %q, want %q", got, want)
	}
	got = dedupePath(dir, "clip", "", "mkv")
	want = filepath.Join(dir, "clip.mkv")
	if got != want {
		t.Fatalf("dedupePath custom = %q, want %q", got, want)
	}
}

func TestOutputTags(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"apollo", "apo8"},
		{"aion", "aion1"},
		{"proteus", "prob4"},
		{"gaia", "ghq5"},
		{"gaia-cg", "gcg5"},
	}
	for _, c := range cases {
		m := ModelRegistry()[c.key]
		if m == nil {
			t.Fatalf("model %s missing", c.key)
		}
		if got := m.Short + m.Version; got != c.want {
			t.Errorf("%s tag = %q, want %q", c.key, got, c.want)
		}
	}
}
