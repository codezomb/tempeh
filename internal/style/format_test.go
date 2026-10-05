package style

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "alignment",
			src:  "locals {\n  longer = 1\n  short = 2\n}\n",
			want: "locals {\n  longer = 1\n  short  = 2\n}\n",
		},
		{
			name: "ordering",
			src:  "locals {\n  a = 1\n  longer = 2\n}\n",
			want: "locals {\n  longer = 2\n  a      = 1\n}\n",
		},
		{
			name: "value tie",
			src:  "locals {\n  aa = 1\n  bb = 222\n}\n",
			want: "locals {\n  bb = 222\n  aa = 1\n}\n",
		},
		{
			name: "comments travel with the item below them",
			src:  "locals {\n  aa = 1\n  # note\n  bb = 222\n}\n",
			want: "locals {\n  # note\n  bb = 222\n  aa = 1\n}\n",
		},
		{
			name: "trailing comment travels",
			src:  "locals {\n  aa = 1 # note\n  value = 22\n}\n",
			want: "locals {\n  value = 22\n  aa    = 1 # note\n}\n",
		},
		{
			name: "group",
			src:  "locals {\n  items = [\n    1,\n    2\n  ]\n  value = 3\n}\n",
			want: "locals {\n  value = 3\n\n  items = [\n    1,\n    2\n  ]\n}\n",
		},
		{
			name: "blank between single lines",
			src:  "locals {\n  longer = 1\n\n  short = 2\n}\n",
			want: "locals {\n  longer = 1\n  short  = 2\n}\n",
		},
		{
			name: "blank before multiline",
			src:  "locals {\n  value = 1\n  items = [\n    1,\n    2\n  ]\n}\n",
			want: "locals {\n  value = 1\n\n  items = [\n    1,\n    2\n  ]\n}\n",
		},
		{
			name: "blank before the first item",
			src:  "locals {\n\n  value = 1\n}\n",
			want: "locals {\n  value = 1\n}\n",
		},
		{
			name: "inline tuple",
			src:  "locals {\n  value = [1, 2]\n}\n",
			want: "locals {\n  value = [\n    1,\n    2\n  ]\n}\n",
		},
		{
			name: "inline object",
			src:  "locals {\n  value = { a = 1, b = 222 }\n}\n",
			want: "locals {\n  value = {\n    b = 222\n    a = 1\n  }\n}\n",
		},
		{
			name: "colon separator",
			src:  "locals {\n  value = {\n    a : 1\n  }\n}\n",
			want: "locals {\n  value = {\n    a = 1\n  }\n}\n",
		},
		{
			name: "dependency list",
			src:  "resource \"x\" \"y\" {\n  depends_on = [x.a, x.long]\n}\n",
			want: "resource \"x\" \"y\" {\n  depends_on = [\n    x.long,\n    x.a\n  ]\n}\n",
		},
		{
			name: "trailing comma",
			src:  "resource \"x\" \"y\" {\n  depends_on = [\n    x.a,\n  ]\n}\n",
			want: "resource \"x\" \"y\" {\n  depends_on = [\n    x.a\n  ]\n}\n",
		},
		{
			name: "dependency last",
			src:  "resource \"x\" \"y\" {\n  depends_on = [\n    x.a\n  ]\n\n  name = \"n\"\n}\n",
			want: "resource \"x\" \"y\" {\n  name = \"n\"\n\n  depends_on = [\n    x.a\n  ]\n}\n",
		},
		{
			name: "dependency after block",
			src:  "resource \"x\" \"y\" {\n  lifecycle { prevent_destroy = true }\n  depends_on = [\n    x.a\n  ]\n}\n",
			want: "resource \"x\" \"y\" {\n  lifecycle { prevent_destroy = true }\n\n  depends_on = [\n    x.a\n  ]\n}\n",
		},
		{
			name: "multiline value keeps its body",
			src:  "locals {\n  value = merge(\n    local.a,\n    local.b\n  )\n}\n",
			want: "locals {\n  value = merge(\n    local.a,\n    local.b\n  )\n}\n",
		},
		{
			name: "heredoc moves whole",
			src:  "locals {\n  items = [\n    1\n  ]\n  text = <<EOT\nline\nEOT\n}\n",
			want: "locals {\n  items = [\n    1\n  ]\n\n  text = <<EOT\nline\nEOT\n}\n",
		},
		{
			name: "variable argument order",
			src:  "variable \"x\" {\n  type = string\n  description = \"d\"\n}\n",
			want: "variable \"x\" {\n  description = \"d\"\n  type        = string\n}\n",
		},
		{
			name: "nested object inside yamlencode",
			src:  "locals {\n  body = yamlencode({\n    spec = { b = 1, a = 2 }\n  })\n}\n",
			want: "locals {\n  body = yamlencode({\n    spec = {\n      b = 1\n      a = 2\n    }\n  })\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, issues := format([]byte(tt.src), "test.tf")
			if string(got) != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}

			if len(issues) > 0 {
				t.Fatalf("violations left after formatting: %+v", issues)
			}

			again, _ := format(got, "test.tf")
			if !bytes.Equal(again, got) {
				t.Fatalf("not idempotent:\n%s", again)
			}
		})
	}
}

// TestFormatDeclines covers rewrites that would move text past something the
// formatter cannot interpret, so the text is reported instead.
func TestFormatDeclines(t *testing.T) {
	tests := map[string]string{
		"comment inside a dependency list": "resource \"x\" \"y\" {\n  depends_on = [ # note\n    x.a,\n  ]\n}\n",
		"comment stranded above a run":     "locals {\n  x = {\n    a = 1\n  }\n\n  # note\n\n  y = {\n    b = 2\n  }\n\n  depends = [\n    x.a\n  ]\n}\n",
		"assignment after a nested block":  "resource \"x\" \"y\" {\n  depends_on = [\n    x.a\n  ]\n\n  lifecycle {\n    ignore_changes = [z]\n  }\n}\n",
		"header comment above a body":      "locals {\n  # section header\n\n  b  = 1\n  aa = 2\n}\n",
		"dangling comment below a run":     "locals {\n  b = 1\n  aa = 2\n  # note\n}\n",
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			got, issues := format([]byte(src), "test.tf")
			if !tokensKept(tokenBag([]byte(src)), tokenBag(got)) {
				t.Fatalf("tokens changed:\n%s", got)
			}

			if len(issues) == 0 {
				t.Fatalf("expected a reported violation, got:\n%s", got)
			}
		})
	}
}

// TestFormatDuplicateKeys holds the line the ordering rule cannot cross: HCL
// applies object entries in source order, so the last duplicate key wins.
func TestFormatDuplicateKeys(t *testing.T) {
	inline := "locals {\n  value = { a = 1, a = 22 }\n}\n"
	got, issues := format([]byte(inline), "test.tf")
	want := "locals {\n  value = {\n    a = 1\n    a = 22\n  }\n}\n"
	if string(got) != want {
		t.Fatalf("duplicate key was reordered:\n%s", got)
	}

	if len(issues) == 0 {
		t.Fatal("expected the ordering rule to be reported for review")
	}

	multiline := want
	if again, _ := format([]byte(multiline), "test.tf"); string(again) != multiline {
		t.Fatalf("duplicate key was reordered:\n%s", again)
	}
}

// TestFormatCommentOwnership checks which comments a reorder may move.
func TestFormatCommentOwnership(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "comment above a block describes the block",
			src:  "resource \"x\" \"y\" {\n  b = 1\n  aa = 2\n\n  # configures lifecycle\n  lifecycle {\n    prevent_destroy = true\n  }\n}\n",
			want: "resource \"x\" \"y\" {\n  aa = 2\n  b  = 1\n\n  # configures lifecycle\n  lifecycle {\n    prevent_destroy = true\n  }\n}\n",
		},
		{
			name: "header comment stays at the top",
			src:  "locals {\n  # section header\n  bb = 2\n  b  = 1\n  c  = 3\n}\n",
			want: "locals {\n  # section header\n  bb = 2\n  b  = 1\n  c  = 3\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, issues := format([]byte(tt.src), "test.tf")
			if string(got) != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}

			if len(issues) > 0 {
				t.Fatalf("violations left: %+v", issues)
			}
		})
	}
}

func TestReplaceRefusesSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.tf")
	link := filepath.Join(dir, "link.tf")
	if err := os.WriteFile(target, []byte("locals {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := replace(link, []byte("locals {\n  a = 1\n}\n")); err == nil {
		t.Error("a symlink was replaced")
	}

	if err := replace(target, []byte("locals {\n  a = 1\n}\n")); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode not preserved: %v", info.Mode())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Errorf("temporary file left behind: %v", entries)
	}
}

func TestFormatRefusesBrokenInput(t *testing.T) {
	tests := map[string]string{
		"syntax error": "locals { value = [ }\n",
		"crlf":         "locals {\r\n  longer = 1\r\n  short = 2\r\n}\r\n",
		"empty":        "",
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			got, _ := format([]byte(src), "test.tf")
			if string(got) != src {
				t.Fatalf("changed %s input:\n%s", name, got)
			}
		})
	}
}

// TestSafetyNet checks the guard that refuses a rewrite which a rule fix should
// never perform.
func TestSafetyNet(t *testing.T) {
	src := []byte("locals {\n  longer = 1\n  short = 2\n}\n")

	if reason := declined([]byte("locals {\n  longer = 1\n"), "t.tf", tokenBag(src)); reason != "rewrite does not parse" {
		t.Errorf("unparsable rewrite: %q", reason)
	}

	if reason := declined([]byte("locals {\n  longer = 1\n  short = 3\n}\n"), "t.tf", tokenBag(src)); reason != "rewrite changes the token stream" {
		t.Errorf("changed value: %q", reason)
	}

	if reason := declined([]byte("locals {\n  longer = 1\n  short  = 2\n}\n"), "t.tf", tokenBag(src)); reason != "" {
		t.Errorf("whitespace only rewrite refused: %q", reason)
	}
}

// TestTreeStable checks that formatted configuration is a fixed point and that a
// mechanical alignment loss is repaired without changing tokens. TEMPEH_TEST_TREE
// points it at a real tree instead of the fixtures.
func TestTreeStable(t *testing.T) {
	root := "testdata"
	if tree := os.Getenv("TEMPEH_TEST_TREE"); tree != "" {
		root = tree
	}

	unalign := regexp.MustCompile(`(?m)^(\s+[A-Za-z_][\w-]*)\s+= `)

	checked := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && skipped[d.Name()] {
			return filepath.SkipDir
		}

		if d.IsDir() || !strings.HasSuffix(path, ".tf") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
		if got, issues := format(src, path); !bytes.Equal(got, src) || len(issues) > 0 {
			t.Errorf("%s is not a fixed point: %d violations, changed=%t", path, len(issues), !bytes.Equal(got, src))
		}

		// Whitespace-only loss is repaired, and only whitespace moves.
		damage := unalign.ReplaceAll(src, []byte("$1 = "))
		fixed, issues := format(damage, path)
		if len(issues) > 0 {
			t.Errorf("%s: alignment loss not repaired: %+v", path, issues)
		}

		if again, _ := format(fixed, path); !bytes.Equal(again, fixed) {
			t.Errorf("%s: repaired text is not a fixed point", path)
		}

		if !tokensKept(tokenBag(damage), tokenBag(fixed)) {
			t.Errorf("%s: formatting changed the token stream", path)
		}

		checked++

		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if checked == 0 {
		t.Fatal("no .tf files checked")
	}
}
