# tempeh

A wrapper around OpenTofu. It loads encrypted variables, runs `init` when it's needed, and
replaces `tofu fmt` with a formatter for one fixed HCL style. Install it as `tofu` and
everything else behaves as before.

## Use

```sh
tempeh plan                      # any OpenTofu command, in the current directory
tempeh -chdir=stacks/network apply
tempeh init -upgrade

tempeh fmt                       # fix the style in this directory
tempeh fmt -check stacks         # report only; exits 1 if anything is off
TOFU_FMT=real tempeh fmt         # OpenTofu's own formatter
```

Before any command other than `fmt`, tempeh:

1. Decrypts `secrets/<workspace>.tfvars` with `sops`, if the file exists, and passes the
   values to OpenTofu as `TF_VAR_*`, including for automatic and explicit `init` so backend
   configuration can use them. Nothing decrypted is written to disk. Values must be literals.
2. Runs `tofu init` if the `terraform` blocks, a `source` line, or `.terraform.lock.hcl`
   changed since the last init.

It finds OpenTofu at `$TEMPEH_TOFU`, then `/usr/libexec/opentofu`, then the first `tofu` on
`PATH` that isn't tempeh itself.

## Style

`tempeh fmt` enforces these and repairs the ones it can:

- Single-line arguments come first, with no blank lines between them and `=` aligned.
- Multiline arguments follow, each separated by a blank line, with a single space around `=`.
- Within each group, longer keys come first; ties go to the longer value.
- A list or object with more than one value has one entry per line.
- Objects assign with `=`, not `:`.
- `depends_on` is last, after a blank line, one entry per line, longest first, no trailing
  comma.

A repair that would change anything but whitespace and order is refused, and the file is
left as it was.

[docs/hcl-style.md](docs/hcl-style.md) describes each rule with examples.

## Install

Download a build for Linux or macOS from the
[releases](https://github.com/codezomb/tempeh/releases) page and check it against
`checksums.txt`, or build it with Go:

```sh
go install github.com/codezomb/tempeh/cmd/tempeh@latest
```

Pushing a `v*` tag runs the tests and publishes the release.

In a container image, move OpenTofu to `/usr/libexec/opentofu` and link `tofu` to tempeh:

```sh
mv /usr/bin/tofu /usr/libexec/opentofu
install -m 755 tempeh /bin/tempeh
ln -s tempeh /bin/tofu
```

## Build

Go lives inside the project, not on the host:

```sh
make build        # ./bin/go build -o bin/tempeh ./cmd/tempeh
make check        # gofmt check, vet, tests, build
make fmt          # the fixer; check only reports
```

`bin/go` and `bin/gofmt` run the toolchain under `.toolchain/`. To set it up, download a
`go*.tar.gz` for your platform from go.dev, verify its sha256, and unpack it to
`.toolchain/go`.

To run the style tests against a real tree:

```sh
TEMPEH_TEST_TREE=/path/to/stacks make test
```
