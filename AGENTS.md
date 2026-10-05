# AGENTS.md

tempeh wraps OpenTofu and formats HCL to one fixed style. `CLAUDE.md` imports this
file. Read [README.md](README.md) first.

- Go only, built with the project-local toolchain (`make check`). Dependencies: hashicorp/hcl
  and zclconf/go-cty; add nothing else without a reason in the README.
- tempeh invents no flags. Every subcommand but `fmt` is OpenTofu's and its arguments pass
  through untouched; `fmt` accepts only what `tofu fmt` accepts.
- Never write decrypted variables to disk or print a value. Errors name the variable and the
  position.
- A repair must parse and keep the token stream, or it is refused. Add a rule to the checker
  first and a repair only where the fix is measurable from source text.
- Every rule and repair gets a case in the table tests. `testdata/` must stay a fixed point.
- `internal/style` and `internal/tofu` do not import each other; `cmd/tempeh` only dispatches.
- Packages write to the streams they are given and return errors; only `main` exits.
- One-line comments; rationale in README. gofmt is the formatter; run `make fmt`.
- OpenTofu only. No Terraform wording or tooling beyond the names OpenTofu itself requires.
