// Package style checks and repairs the house HCL style.
package style

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrFailed reports violations, unreadable paths, or an empty selection. The details
// have already been written to the Runner's streams.
var ErrFailed = errors.New("style check failed")

// skipped names directories that never hold configuration to check.
var skipped = map[string]bool{
	".terraform": true,
	".git":       true,
	"scratch":    true,
}

// Runner checks files and reports to its streams.
type Runner struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Run checks every .tf file under paths and, when fix is set, repairs them.
func (r Runner) Run(paths []string, fix bool) error {
	names, failed := r.collect(paths)

	if len(names) == 0 {
		fmt.Fprintln(r.Stderr, "No .tf files found.")

		failed = true
	}

	count, rewritten := 0, 0

	for _, name := range names {
		raw, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintln(r.Stderr, err)

			failed = true

			continue
		}

		src := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		out, issues := src, lint(src, name)

		switch {
		case !fix:
		case !bytes.Equal(raw, src):
			fmt.Fprintf(r.Stdout, "skipped %s: CRLF line endings\n", name)
		default:
			out, issues = format(src, name)
		}

		for _, i := range issues {
			fmt.Fprintf(r.Stdout, "%s:%d: %s: %s\n", name, i.line, i.rule, i.message)
		}

		count += len(issues)

		if !fix || bytes.Equal(out, src) {
			continue
		}

		if err := replace(name, out); err != nil {
			fmt.Fprintln(r.Stderr, err)

			failed = true

			continue
		}

		rewritten++

		fmt.Fprintf(r.Stdout, "formatted %s\n", name)
	}

	if fix {
		fmt.Fprintf(r.Stdout, "Checked %d HCL files; rewrote %d, %d violations left.\n", len(names), rewritten, count)
	} else {
		fmt.Fprintf(r.Stdout, "Checked %d HCL files; %d violations.\n", len(names), count)
	}

	if count > 0 || failed {
		return ErrFailed
	}

	return nil
}

// collect lists the .tf files under paths, sorted and without duplicates.
func (r Runner) collect(paths []string) ([]string, bool) {
	files := map[string]bool{}
	failed := false

	for _, path := range paths {
		err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if skipped[d.Name()] {
					return filepath.SkipDir
				}

				return nil
			}

			if strings.HasSuffix(path, ".tf") {
				files[path] = true
			}

			return nil
		})
		if err != nil {
			fmt.Fprintln(r.Stderr, err)

			failed = true
		}
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	sort.Strings(names)

	return names, failed
}

// replace swaps a file's contents atomically, keeping its mode. A symlink is
// left alone rather than replaced by a regular file.
func replace(name string, out []byte) (err error) {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink; not rewritten", name)
	}

	temp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return err
	}

	// After a successful rename there is nothing left to remove.
	defer func() {
		if err != nil {
			_ = os.Remove(temp.Name())
		}
	}()

	if _, err = temp.Write(out); err != nil {
		_ = temp.Close()

		return err
	}

	if err = temp.Chmod(info.Mode().Perm()); err != nil {
		_ = temp.Close()

		return err
	}

	if err = temp.Close(); err != nil {
		return err
	}

	return os.Rename(temp.Name(), name)
}
