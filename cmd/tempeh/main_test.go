package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkDir(t *testing.T) {
	tests := []struct {
		args []string
		dir  string
		rest []string
	}{
		{nil, ".", []string{}},
		{[]string{"version"}, ".", []string{"version"}},
		{[]string{"plan", "-json"}, ".", []string{"plan", "-json"}},
		{[]string{"-chdir=stacks/network", "plan"}, "stacks/network", []string{"plan"}},
		{[]string{"apply", "-chdir=x"}, "x", []string{"apply"}},
	}

	for _, tt := range tests {
		dir, rest := workDir(tt.args)
		if dir != tt.dir || !reflect.DeepEqual(rest, tt.rest) {
			t.Errorf("workDir(%v) = %q %v, want %q %v", tt.args, dir, rest, tt.dir, tt.rest)
		}
	}
}

func TestRunFmtMapsTofuFlags(t *testing.T) {
	dir := t.TempDir()
	src := "locals {\n  longer = 1\n  short = 2\n}\n"

	write := func() {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write()

	if code := runFmt(io.Discard, io.Discard, dir, []string{"-check", "-recursive"}); code != 1 {
		t.Errorf("check mode exit %d, want 1", code)
	}

	if data, err := os.ReadFile(filepath.Join(dir, "main.tf")); err != nil || string(data) != src {
		t.Error("check mode rewrote the file")
	}

	if code := runFmt(io.Discard, io.Discard, dir, nil); code != 0 {
		t.Errorf("repair mode exit %d, want 0", code)
	}

	if data, err := os.ReadFile(filepath.Join(dir, "main.tf")); err != nil || string(data) == src {
		t.Error("repair mode left the violation in place")
	}

	write()

	if code := runFmt(io.Discard, io.Discard, dir, []string{"-write=false"}); code != 1 {
		t.Errorf("-write=false exit %d, want 1", code)
	}

	if data, err := os.ReadFile(filepath.Join(dir, "main.tf")); err != nil || string(data) != src {
		t.Error("-write=false rewrote the file")
	}

	// An unknown option is refused, not guessed at.
	if code := runFmt(io.Discard, io.Discard, dir, []string{"-wreck=false"}); code != 1 {
		t.Errorf("unknown option exit %d, want 1", code)
	}

	if data, err := os.ReadFile(filepath.Join(dir, "main.tf")); err != nil || string(data) != src {
		t.Error("an unknown option rewrote the file")
	}
}
