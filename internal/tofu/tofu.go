// Package tofu prepares a directory and hands the command to OpenTofu.
package tofu

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// binaryEnv names the OpenTofu binary when it is not in a usual place.
	binaryEnv = "TEMPEH_TOFU"

	// libexecBinary is where an image keeps OpenTofu when tempeh is installed as tofu.
	libexecBinary = "/usr/libexec/opentofu"

	stateDir = ".terraform"
)

// Run prepares dir and replaces this process with OpenTofu, so on success it does
// not return. It returns an exit status when OpenTofu ran as a child (an explicit
// init) or when preparation failed. Arguments pass through untouched.
func Run(dir string, args []string) (int, error) {
	bin, err := binary()
	if err != nil {
		return 1, err
	}

	// OpenTofu applies -chdir by changing directory; the preparation has to match.
	if err := os.Chdir(dir); err != nil {
		return 1, err
	}

	env := os.Environ()

	if isConfigDir(".") {
		vars, err := configVars(workspace(bin))
		if err != nil {
			return 1, err
		}

		env = append(env, vars...)

		// An explicit init runs as a child so the fingerprint is recorded after it.
		if len(args) > 0 && args[0] == "init" {
			return initExplicit(bin, args, env)
		}

		if needsInit() {
			fmt.Println("Providers, backend and modules changed -- initializing.")

			if code, err := forward(bin, []string{"init"}, env); err != nil || code != 0 {
				return 1, fmt.Errorf("tofu init failed: %w", errors.Join(err, exitError(code)))
			}

			if err := storeFingerprint(); err != nil {
				return 1, fmt.Errorf("cannot record the init fingerprint: %w", err)
			}
		}
	}

	// On success exec does not return.
	return 1, syscall.Exec(bin, append([]string{"tofu"}, args...), env)
}

// initExplicit runs a requested init and records the fingerprint when it succeeds.
func initExplicit(bin string, args, env []string) (int, error) {
	code, err := forward(bin, args, env)
	if err != nil {
		return 1, err
	}

	if code != 0 {
		return code, nil
	}

	if err := storeFingerprint(); err != nil {
		return 1, fmt.Errorf("cannot record the init fingerprint: %w", err)
	}

	return 0, nil
}

// exitError describes a non-zero exit status, or nothing for zero.
func exitError(code int) error {
	if code == 0 {
		return nil
	}

	return fmt.Errorf("exit status %d", code)
}

// binary finds OpenTofu: TEMPEH_TOFU, then the libexec path, then a tofu on PATH that
// is not this program.
func binary() (string, error) {
	if path := os.Getenv(binaryEnv); path != "" {
		return path, nil
	}

	if _, err := os.Stat(libexecBinary); err == nil {
		return libexecBinary, nil
	}

	self, err := os.Executable()
	if err == nil {
		self, _ = filepath.EvalSymlinks(self)
	}

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		path, err := filepath.EvalSymlinks(filepath.Join(dir, "tofu"))
		if err != nil || path == self {
			continue
		}

		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}

	return "", fmt.Errorf("OpenTofu not found; set %s to its path", binaryEnv)
}

// workspace asks OpenTofu which workspace is selected. A directory that has never
// been initialized has no answer yet, and default is what it will use.
func workspace(bin string) string {
	out, err := exec.Command(bin, "workspace", "show").Output()
	if err != nil {
		return "default"
	}

	return strings.TrimSpace(string(out))
}

// forward runs OpenTofu as a child, wired to the terminal, and reports its exit status.
func forward(bin string, args, env []string) (int, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return exit.ExitCode(), nil
	}

	return 1, err
}

// isConfigDir keeps preparation away from directories that hold no configuration, so
// `tempeh version` at a repository root neither decrypts nor initializes.
func isConfigDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if e.IsDir() && (e.Name() == secretsDir || e.Name() == stateDir) {
			return true
		}

		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tf") {
			return true
		}
	}

	return false
}

// tfFiles lists .tf files under dir, sorted, skipping .terraform and .git.
func tfFiles(dir string) ([]string, error) {
	files := []string{}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == stateDir || d.Name() == ".git" {
				return filepath.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(path, ".tf") {
			files = append(files, path)
		}

		return nil
	})

	// WalkDir visits in lexical order, so files is already sorted.
	return files, err
}
