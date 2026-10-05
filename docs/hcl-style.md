# tempeh HCL style

The style `tempeh fmt` enforces, rule by rule. There is one style: no configuration file, no
per-rule switches, and no comment that turns a rule off.

## Running the check

```sh
tempeh fmt                  # repair the files under the current directory
tempeh fmt -check stacks    # report only
```

Every `.tf` file under the given paths is checked. Directories named `.terraform`, `.git`
and `scratch` are skipped. A violation is printed as `file:line: rule: message`:

```
stacks/web/main.tf:14: order: keys must follow descending length/value order
```

The exit status is 1 when any violation is left, a path cannot be read, or no `.tf` file is
found.

## Rules

| Rule                        | Enforces                                              | Repaired |
| --------------------------- | ----------------------------------------------------- | -------- |
| [`spacing`](#spacing)       | `=` alignment, and `=` rather than `:` in objects     | yes      |
| [`group`](#group)           | single-line assignments before multiline ones         | yes      |
| [`blank`](#blank)           | where blank lines go between assignments              | yes      |
| [`order`](#order)           | longer keys first                                     | yes      |
| [`collection`](#collection) | one entry per line in a list or object of two or more | yes      |
| [`depends`](#depends)       | the position and shape of `depends_on`                | yes      |
| [`syntax`](#syntax)         | the file parses                                       | no       |

"Repaired" means `tempeh fmt` rewrites the file. Some repairs are refused; see
[Refused repairs](#refused-repairs).

The rules apply to the arguments of a block and to the entries of an object, at any depth.
An assignment is single-line when its value starts and ends on the same line, and multiline
otherwise.

### spacing

Single-line assignments in the same block or object align their `=` in one column, set by
the longest key, even when a nested block sits between them. A multiline assignment and
`depends_on` take exactly one space before `=`. Every assignment has exactly one space
after `=`.

```hcl
# bad
variable "replicas" {
  description = "Number of web replicas."
  default = 2
  type = number
}

# good
variable "replicas" {
  description = "Number of web replicas."
  default     = 2
  type        = number
}
```

```hcl
# bad: a multiline assignment does not join the alignment
locals {
  namespace = "web"

  ranges    = [
    "198.51.100.0/24",
    "192.0.2.0/24"
  ]
}

# good
locals {
  namespace = "web"

  ranges = [
    "198.51.100.0/24",
    "192.0.2.0/24"
  ]
}
```

Object entries assign with `=`, not `:`.

```hcl
# bad
locals {
  ports = {
    https: 443
    http: 80
  }
}

# good
locals {
  ports = {
    https = 443
    http  = 80
  }
}
```

Messages: `assignment spacing/alignment does not match its group`,
`use = for object assignments`.

### group

Single-line assignments come before multiline assignments.

```hcl
# bad
locals {
  ranges = [
    "198.51.100.0/24",
    "192.0.2.0/24"
  ]

  namespace = "web"
}

# good
locals {
  namespace = "web"

  ranges = [
    "198.51.100.0/24",
    "192.0.2.0/24"
  ]
}
```

Message: `single-line assignments must precede multiline assignments`.

### blank

- No blank line before the first item in a block or object.
- No blank line between two single-line assignments.
- A blank line before every multiline assignment and before `depends_on`, unless it is the
  first item.

```hcl
# bad
locals {

  namespace = "web"

  domain    = "example.test"
  ranges = [
    "198.51.100.0/24",
    "192.0.2.0/24"
  ]
}

# good
locals {
  namespace = "web"
  domain    = "example.test"

  ranges = [
    "198.51.100.0/24",
    "192.0.2.0/24"
  ]
}
```

A single-line assignment that follows a nested block may have a blank line before it or
not.

Messages: `no blank line before the first item`,
`no blank lines between single-line assignments`,
`multiline assignments require a preceding blank line`.

### order

Within the single-line group, and separately within the multiline group, longer keys come
first.

```hcl
# bad
variable "api_token" {
  type        = string
  sensitive   = true
  description = "Token for the DNS provider."
}

# good
variable "api_token" {
  description = "Token for the DNS provider."
  sensitive   = true
  type        = string
}
```

When two keys are the same length, the one with the longer value comes first.

```hcl
# bad
locals {
  ips = {
    gateway = "192.0.2.1"
    storage = "192.0.2.20"
    proxy   = "192.0.2.10"
  }
}

# good
locals {
  ips = {
    storage = "192.0.2.20"
    gateway = "192.0.2.1"
    proxy   = "192.0.2.10"
  }
}
```

- Length is counted in characters. The quotes around an object key are not counted; a value
  is measured as it is written in the file, quotes included.
- Two entries with equal key and value lengths may go in either order.
- A nested block starts the ordering over: the assignments after it are ordered among
  themselves.
- `depends_on` is not ordered by this rule; [`depends`](#depends) places it.

Message: `keys must follow descending length/value order`.

### collection

A list or object with two or more entries puts each entry on its own line, with the opening
and closing delimiters on lines of their own. A collection with one entry may stay on one
line.

```hcl
# bad
locals {
  single = ["192.0.2.1"]

  ranges = ["198.51.100.0/24", "192.0.2.0/24"]

  labels = { tier = "web", app = "shop" }
}

# good
locals {
  single = ["192.0.2.1"]

  ranges = [
    "198.51.100.0/24",
    "192.0.2.0/24"
  ]

  labels = {
    tier = "web"
    app  = "shop"
  }
}
```

The entries of a list are not reordered; only object keys and `depends_on` are.

Message:
`multivalue collections require separate entry lines and separate opening/closing delimiters`.

### depends

`depends_on` is the last thing in its block, after a blank line. Its list has one dependency
per line even when there is only one, the longest reference first, and no trailing comma.

```hcl
# bad
resource "kubectl_manifest" "web" {
  depends_on = [kubectl_manifest.settings, kubectl_manifest.namespace,]
  yaml_body  = local.web
}

# good
resource "kubectl_manifest" "web" {
  yaml_body = local.web

  depends_on = [
    kubectl_manifest.namespace,
    kubectl_manifest.settings
  ]
}
```

```hcl
# bad
resource "kubectl_manifest" "upstream" {
  yaml_body = each.value

  depends_on = [kubectl_manifest.namespace]
}

# good
resource "kubectl_manifest" "upstream" {
  yaml_body = each.value

  depends_on = [
    kubectl_manifest.namespace
  ]
}
```

Messages: `depends_on must be last in its block`, `dependencies require their own lines`,
`dependencies must be ordered by descending source length`,
`dependencies must not have a trailing comma`.

### syntax

A file that does not parse is reported with the parser's message and is not checked
further or rewritten.

## Refused repairs

`tempeh fmt` only moves text and changes whitespace. When it cannot be sure a repair does
only that, it leaves the text alone and the violation stays in the report.

- **A comment with no clear owner.** A comment directly above an assignment, or trailing it
  on the same line, moves with that assignment. A comment at the top of a block may
  describe the whole block, so the assignments under it are not reordered. A comment inside
  a list or object that does not trail an entry stops that collection from being expanded.
- **A repeated object key.** The later entry wins in HCL, so the order carries meaning and
  the object is not reordered.
- **Inconsistent indentation** within a run of assignments.
- **A changed token stream.** If a rewritten file would not parse, or would hold different
  tokens than the original, the rewrite is dropped and a `format` line says why.
- **CRLF line endings.** The file is checked but not rewritten.
- **A symlink.** It is not replaced by a regular file.

## Outside the style

tempeh does not check indentation, comment wording, the order of blocks in a file, the
order of nested blocks, or the arguments of a function call. `TOFU_FMT=real tempeh fmt`
runs OpenTofu's own formatter.

[internal/style/testdata/stack](../internal/style/testdata/stack) is a complete example that
passes every rule.
