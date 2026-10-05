// tempeh wraps OpenTofu: it prepares a run and hands off to tofu, and it owns `fmt`
// as the house HCL style tool.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/codezomb/tempeh/internal/tofu"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches one invocation and returns the process exit status.
func run(args []string) int {
	// -chdir is resolved first: `fmt` has to be found behind it.
	dir, args := workDir(args)

	// TOFU_FMT=real hands fmt back to OpenTofu's own formatter.
	if len(args) > 0 && args[0] == "fmt" && os.Getenv("TOFU_FMT") != "real" {
		return runFmt(os.Stdout, os.Stderr, dir, args[1:])
	}

	code, err := tofu.Run(dir, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempeh:", err)
	}

	return code
}

// workDir pulls -chdir out of the arguments, as OpenTofu does before anything else.
func workDir(args []string) (string, []string) {
	dir := "."
	rest := make([]string, 0, len(args))

	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, "-chdir="); ok {
			dir = value

			continue
		}

		rest = append(rest, arg)
	}

	return dir, rest
}
