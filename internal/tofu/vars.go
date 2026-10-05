package tofu

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

const secretsDir = "secrets"

// configVars decrypts secrets/<workspace>.tfvars with sops and returns TF_VAR_
// assignments. The plaintext only ever exists in memory.
func configVars(workspace string) ([]string, error) {
	path := filepath.Join(secretsDir, workspace+".tfvars")

	// A workspace without a secrets file has nothing to load.
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if _, err := exec.LookPath("sops"); err != nil {
		return nil, fmt.Errorf("%s needs sops to decrypt: %w", path, err)
	}

	var out bytes.Buffer

	cmd := exec.Command("sops", "-d", path)
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sops could not decrypt %s: %w", filepath.Base(path), err)
	}

	return varsFromTFVars(out.Bytes(), path)
}

// varsFromTFVars turns decrypted tfvars text into environment assignments. Values
// must be literals, and an error names the variable and position but never the value.
func varsFromTFVars(src []byte, name string) ([]string, error) {
	file, diags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s: %s", name, diags[0].Summary)
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("%s: unexpected configuration shape", name)
	}

	attrs := make([]string, 0, len(body.Attributes))
	for attr := range body.Attributes {
		attrs = append(attrs, attr)
	}

	sort.Strings(attrs)

	env := make([]string, 0, len(attrs))

	for _, attr := range attrs {
		value, diags := body.Attributes[attr].Expr.Value(nil)
		if diags.HasErrors() {
			line := body.Attributes[attr].Expr.Range().Start.Line

			return nil, fmt.Errorf("%s:%d: variable %q is not a literal; this expression needs a context tempeh does not have", name, line, attr)
		}

		value, _ = value.Unmark()

		if !value.IsKnown() {
			return nil, fmt.Errorf("%s: variable %q is not a literal", name, attr)
		}

		// A null assigns nothing; the text "null" would reach a string variable as is.
		if value.IsNull() {
			continue
		}

		literal, err := hclLiteral(value)
		if err != nil {
			return nil, fmt.Errorf("%s: variable %q: %w", name, attr, err)
		}

		env = append(env, "TF_VAR_"+attr+"="+literal)
	}

	return env, nil
}

// hclLiteral renders a value the way OpenTofu reads it from the environment: strings
// verbatim, everything else as JSON, which is valid HCL for complex types.
func hclLiteral(value cty.Value) (string, error) {
	if value.Type().Equals(cty.String) {
		return value.AsString(), nil
	}

	literal, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		return "", err
	}

	return string(literal), nil
}
