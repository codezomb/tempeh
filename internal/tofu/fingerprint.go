package tofu

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

const lockFile = ".terraform.lock.hcl"

// sourceLine matches the source declarations that decide what init resolves.
var sourceLine = regexp.MustCompile(`(^|[^A-Za-z0-9_])source[ \t]*=`)

// fingerprintPath is where the fingerprint of the last successful init is kept.
var fingerprintPath = filepath.Join(stateDir, ".fingerprint")

// fingerprint hashes what init depends on: terraform blocks (backend and required
// providers), module and provider source lines, and the provider lock.
func fingerprint() (string, error) {
	files, err := tfFiles(".")
	if err != nil {
		return "", err
	}

	sum := sha256.New()

	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			return "", err
		}

		sum.Write(settings(src, name))

		for line := range strings.SplitSeq(string(src), "\n") {
			if sourceLine.MatchString(line) {
				sum.Write([]byte(line + "\n"))
			}
		}
	}

	if lock, err := os.ReadFile(lockFile); err == nil {
		sum.Write(lock)
	}

	return hex.EncodeToString(sum.Sum(nil)), nil
}

// settings returns the source of a file's terraform blocks. A file that does not
// parse counts whole, so a change to it is never missed.
func settings(src []byte, name string) []byte {
	file, diags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
	if diags.HasErrors() {
		return src
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return src
	}

	out := []byte{}

	for _, block := range body.Blocks {
		if block.Type == "terraform" {
			out = append(out, block.Range().SliceBytes(src)...)
			out = append(out, '\n')
		}
	}

	return out
}

// needsInit reports whether the stored fingerprint is missing or stale.
func needsInit() bool {
	stored, err := os.ReadFile(fingerprintPath)
	if err != nil {
		return true
	}

	current, err := fingerprint()
	if err != nil {
		return true
	}

	return current != strings.TrimSpace(string(stored))
}

// storeFingerprint records the current fingerprint after a successful init.
func storeFingerprint() error {
	sum, err := fingerprint()
	if err != nil {
		return err
	}

	// A configuration with no providers initializes without creating .terraform.
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(fingerprintPath, []byte(sum+"\n"), 0o644)
}
