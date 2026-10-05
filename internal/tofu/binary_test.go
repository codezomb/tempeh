package tofu

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBinaryPrefersEnvironment(t *testing.T) {
	t.Setenv(binaryEnv, "/opt/tofu/bin/tofu")

	got, err := binary()
	if err != nil || got != "/opt/tofu/bin/tofu" {
		t.Errorf("binary() = %q, %v", got, err)
	}
}

func TestBinarySearchesPath(t *testing.T) {
	if _, err := os.Stat(libexecBinary); err == nil {
		t.Skip("the libexec binary is present and wins")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "tofu")

	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(binaryEnv, "")
	t.Setenv("PATH", dir)

	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}

	if got, err := binary(); err != nil || got != want {
		t.Errorf("binary() = %q, %v; want %q", got, err, want)
	}

	t.Setenv("PATH", t.TempDir())

	if got, err := binary(); err == nil {
		t.Errorf("binary() = %q with no tofu on PATH; want an error", got)
	}
}
