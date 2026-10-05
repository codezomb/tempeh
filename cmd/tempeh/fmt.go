package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/codezomb/tempeh/internal/style"
)

const usage = `Usage: tempeh fmt [-check | -diff | -write=false] [file.tf | directory ...]

Repairs the mechanical style rules: equals-sign alignment, blank-line separation,
descending key and value order, single-line groups before multiline ones, one
entry per line in collections, depends_on shape. -check and -diff report without
writing; the default target is the directory the command was run in. Exits 1 for
violations, read errors, or no HCL files.

Every other subcommand is OpenTofu's, run in the current workspace after its
tfvars are loaded into TF_VAR_ variables and providers are initialized when the
fingerprint is stale. TOFU_FMT=real sends fmt to OpenTofu instead.
`

// runFmt maps OpenTofu's fmt flags onto the style tool: -check, -diff and
// -write=false report, anything else repairs.
func runFmt(stdout, stderr io.Writer, dir string, args []string) int {
	fix := true
	targets := []string{}

	for _, arg := range args {
		switch {
		case arg == "-h" || arg == "-help" || arg == "--help":
			fmt.Fprint(stdout, usage)

			return 0
		case arg == "-check", arg == "-diff", arg == "-write=false":
			fix = false
		case arg == "-write=true":
			fix = true
		case arg == "-recursive":
			// Directories are always walked.
		case strings.HasPrefix(arg, "-"):
			// A typo must not turn a report into a rewrite.
			fmt.Fprintf(stderr, "tempeh fmt: unknown option %s\n", arg)
			fmt.Fprint(stderr, usage)

			return 1
		default:
			targets = append(targets, arg)
		}
	}

	if len(targets) == 0 {
		targets = []string{dir}
	}

	verb := "repairing"
	if !fix {
		verb = "checking"
	}

	fmt.Fprintf(stderr, "tempeh fmt: %s %s\n", verb, strings.Join(targets, " "))

	runner := style.Runner{
		Stdout: stdout,
		Stderr: stderr,
	}

	if err := runner.Run(targets, fix); err != nil {
		return 1
	}

	return 0
}
