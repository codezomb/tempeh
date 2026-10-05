package style

// The style engine: it parses HCL, reports the rules a file breaks, and offers byte
// rewrites for the ones that are mechanically repairable. Expressions are never
// evaluated, so a value is only ever read as source text.

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type issue struct {
	message string
	rule    string
	line    int
}

type checker struct {
	edits  []edit
	issues []issue
	src    []byte
	fix    bool
}

type entry struct {
	value hclsyntax.Expression
	key   string
	kr    hcl.Range
	eq    hcl.Pos
}

func (c *checker) report(r hcl.Range, rule, message string) {
	c.issues = append(c.issues, issue{
		message: message,
		rule:    rule,
		line:    r.Start.Line,
	})
}

func (c *checker) raw(r hcl.Range) string { return string(c.src[r.Start.Byte:r.End.Byte]) }

func width(s string) int { return utf8.RuneCountInString(s) }

func multi(e entry) bool { return e.value.Range().Start.Line != e.value.Range().End.Line }

func (c *checker) blank(a, b int) bool {
	lines := strings.Split(string(c.src[a:b]), "\n")
	if len(lines) < 3 {
		return false
	}

	for _, line := range lines[1 : len(lines)-1] {
		if strings.TrimSpace(line) == "" {
			return true
		}
	}

	return false
}

func (c *checker) ordered(a, b entry) bool {
	if width(a.key) != width(b.key) {
		return width(a.key) > width(b.key)
	}

	return width(strings.TrimSpace(c.raw(a.value.Range()))) >= width(strings.TrimSpace(c.raw(b.value.Range())))
}

func (c *checker) entries(es []entry, start hcl.Pos, end hcl.Pos, body bool, blocks []*hclsyntax.Block) {
	sort.Slice(es, func(i, j int) bool { return es[i].kr.Start.Byte < es[j].kr.Start.Byte })
	maxcol := 0
	for _, e := range es {
		if !multi(e) && (!body || e.key != "depends_on") && e.kr.End.Column > maxcol {
			maxcol = e.kr.End.Column
		}
	}

	seenMulti := false
	var prev *entry

	for i, e := range es {
		dep := body && e.key == "depends_on"
		r := e.value.Range()
		if multi(e) {
			seenMulti = true
		} else if seenMulti && !dep {
			c.report(e.kr, "group", "single-line assignments must precede multiline assignments")
		}

		before := string(c.src[e.kr.End.Byte:e.eq.Byte])
		after := string(c.src[e.eq.Byte+1 : r.Start.Byte])

		want := 1
		if !multi(e) && !dep {
			want = maxcol - e.kr.End.Column + 1
		}

		if before != strings.Repeat(" ", want) || after != " " {
			c.report(e.kr, "spacing", "assignment spacing/alignment does not match its group")

			// Alignment columns only mean something for an assignment that starts
			// its own line; an inline collection is expanded before it is aligned.
			if before != strings.Repeat(" ", want) && (want == 1 || c.ownLine(e.kr.Start.Byte)) {
				c.addEdit(e.kr.End.Byte, e.eq.Byte, strings.Repeat(" ", want), rankAlign)
			}

			if after != " " {
				c.addEdit(e.eq.Byte+1, r.Start.Byte, " ", rankAlign)
			}
		}

		anchor := start.Byte
		if i > 0 {
			anchor = es[i-1].value.Range().End.Byte
		}

		// Use parsed block boundaries, never brace characters in comments or strings.
		blockBefore := false
		for _, b := range blocks {
			if b.Range().End.Byte <= e.kr.Start.Byte && b.Range().End.Byte > anchor {
				anchor = b.Range().End.Byte
				blockBefore = true
			}
		}

		if blockBefore {
			prev = nil
		}

		blank := c.blank(anchor, e.kr.Start.Byte)

		// reflowGap only touches whole lines between the two items, so a comment
		// that trails the previous item or block stays where it is.
		fixGap := func(want bool) {
			gap := string(c.src[anchor:e.kr.Start.Byte])
			c.addEdit(anchor, e.kr.Start.Byte, reflowGap(gap, want), rankBlank)
		}

		if i == 0 && !blockBefore && blank {
			c.report(e.kr, "blank", "no blank line before the first item")
			fixGap(false)
		}

		if (i > 0 || blockBefore) && (multi(e) || dep) && !blank {
			c.report(e.kr, "blank", "multiline assignments require a preceding blank line")
			fixGap(true)
		}

		if i > 0 && !blockBefore && !multi(e) && !dep && blank {
			c.report(e.kr, "blank", "no blank lines between single-line assignments")
			fixGap(false)
		}

		if prev != nil && !dep && prev.key != "depends_on" && multi(*prev) == multi(e) && !c.ordered(*prev, e) {
			c.report(e.kr, "order", "keys must follow descending length/value order")
		}

		if dep {
			fixes := false

			// Nothing except whitespace/comments may follow depends_on in its body.
			tokens, _ := hclsyntax.LexConfig(c.src[r.End.Byte:end.Byte], "", hcl.InitialPos)
			for _, t := range tokens {
				if t.Type != hclsyntax.TokenEOF && t.Type != hclsyntax.TokenNewline && t.Type != hclsyntax.TokenComment {
					c.report(e.kr, "depends", "depends_on must be last in its block")
					break
				}
			}

			if t, ok := e.value.(*hclsyntax.TupleConsExpr); ok {
				move := false
				for j, x := range t.Exprs {
					if x.Range().Start.Line == t.Range().Start.Line || x.Range().End.Line == t.Range().End.Line {
						c.report(x.Range(), "depends", "dependencies require their own lines")
						fixes = true
					}

					if j > 0 && width(c.raw(t.Exprs[j-1].Range())) < width(c.raw(x.Range())) {
						c.report(x.Range(), "depends", "dependencies must be ordered by descending source length")
						fixes = true
						move = true
					}
				}

				if len(t.Exprs) > 0 {
					last := t.Exprs[len(t.Exprs)-1].Range()
					toks, _ := hclsyntax.LexExpression(c.src[last.End.Byte:t.Range().End.Byte-1], "", hcl.InitialPos)
					for _, tok := range toks {
						if tok.Type == hclsyntax.TokenComma {
							c.report(last, "depends", "dependencies must not have a trailing comma")
							fixes = true
						}
					}
				}

				if fixes {
					c.fixDepends(t, move)
				}
			}
		}

		prev = &es[i]
	}

	c.reorder(es, body, start.Byte, end.Byte, blocks)
}

func (c *checker) collection(r hcl.Range, items []hcl.Range) {
	if len(items) < 2 {
		return
	}

	broken := false
	for i, item := range items {
		if item.Start.Line == r.Start.Line || (i > 0 && item.Start.Line <= items[i-1].End.Line) || item.End.Line == r.End.Line {
			c.report(item, "collection", "multivalue collections require separate entry lines and separate opening/closing delimiters")
			broken = true
		}
	}

	if broken {
		c.fixCollection(r, items)
	}
}

func lint(src []byte, name string) []issue {
	issues, _ := analyze(src, name, false)
	return issues
}

// analyze reports style violations and, when fix is set, byte rewrites that
// repair those diagnostics. Not every diagnostic has a repair.
func analyze(src []byte, name string, fix bool) ([]issue, []edit) {
	c := checker{src: src, fix: fix}
	file, diags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
	if diags.HasErrors() {
		for _, d := range diags {
			if d.Severity == hcl.DiagError {
				line := 1
				if d.Subject != nil {
					line = d.Subject.Start.Line
				}

				c.issues = append(c.issues, issue{
					message: d.Summary,
					rule:    "syntax",
					line:    line,
				})
			}
		}

		return c.issues, nil
	}

	root, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	bounds := map[*hclsyntax.Body]hcl.Range{}

	// The visitors never return diagnostics.
	_ = hclsyntax.VisitAll(root, func(n hclsyntax.Node) hcl.Diagnostics {
		if b, ok := n.(*hclsyntax.Block); ok {
			bounds[b.Body] = hcl.Range{
				Start: b.OpenBraceRange.End,
				End:   b.CloseBraceRange.Start,
			}
		}

		return nil
	})

	_ = hclsyntax.VisitAll(root, func(n hclsyntax.Node) hcl.Diagnostics {
		switch v := n.(type) {
		case *hclsyntax.Body:
			es := []entry{}
			for _, a := range v.Attributes {
				es = append(es, entry{
					value: a.Expr,
					key:   a.Name,
					kr:    a.NameRange,
					eq:    a.EqualsRange.Start,
				})
			}

			bound, ok := bounds[v]
			if !ok {
				bound = v.SrcRange
			}

			c.entries(es, bound.Start, bound.End, true, v.Blocks)

		case *hclsyntax.ObjectConsExpr:
			es := []entry{}
			rs := []hcl.Range{}
			for _, x := range v.Items {
				kr := x.KeyExpr.Range()
				key := strings.Trim(c.raw(kr), "\"")
				tokens, _ := hclsyntax.LexExpression(src[kr.End.Byte:x.ValueExpr.Range().Start.Byte], name, kr.End)
				for _, t := range tokens {
					if t.Type == hclsyntax.TokenEqual || t.Type == hclsyntax.TokenColon {
						es = append(es, entry{
							value: x.ValueExpr,
							key:   key,
							kr:    kr,
							eq:    t.Range.Start,
						})

						if t.Type == hclsyntax.TokenColon {
							c.report(kr, "spacing", "use = for object assignments")
							c.addEdit(t.Range.Start.Byte, t.Range.End.Byte, "=", rankSep)
						}

						break
					}
				}

				rs = append(rs, hcl.Range{
					Start: kr.Start,
					End:   x.ValueExpr.Range().End,
				})
			}

			c.collection(v.Range(), rs)
			c.entries(es, v.Range().Start, v.Range().End, false, nil)

		case *hclsyntax.TupleConsExpr:
			rs := []hcl.Range{}
			for _, x := range v.Exprs {
				rs = append(rs, x.Range())
			}

			c.collection(v.Range(), rs)
		}

		return nil
	})

	sort.SliceStable(c.issues, func(i, j int) bool { return c.issues[i].line < c.issues[j].line })
	return c.issues, c.edits
}
