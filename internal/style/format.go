package style

import (
	"bytes"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Repair ranks decide which edit wins when two proposals overlap. Larger wins,
// and a deferred proposal is retried by the next pass over the rewritten text.
const (
	rankAlign = iota + 1
	rankBlank
	rankOrder
	rankColl
	rankSep
)

const formatPasses = 50

// edit is a byte range rewrite offered alongside a diagnostic.
type edit struct {
	text  string
	rank  int
	start int
	end   int
}

func (c *checker) addEdit(start, end int, text string, rank int) {
	if !c.fix || text == string(c.src[start:end]) {
		return
	}

	c.edits = append(c.edits, edit{
		text:  text,
		rank:  rank,
		start: start,
		end:   end,
	})
}

func (c *checker) lineStart(b int) int {
	if i := bytes.LastIndexByte(c.src[:b], '\n'); i >= 0 {
		return i + 1
	}

	return 0
}

func (c *checker) lineEnd(b int) int {
	if i := bytes.IndexByte(c.src[b:], '\n'); i >= 0 {
		return b + i
	}

	return len(c.src)
}

func indentOf(s string) string {
	i := strings.IndexFunc(s, func(r rune) bool { return r != ' ' && r != '\t' })
	if i < 0 {
		i = len(s)
	}

	return s[:i]
}

// reflowGap adds or removes blank lines between two items without touching the
// comments or the indentation of the item that follows the gap.
func reflowGap(gap string, want bool) string {
	lines := strings.Split(gap, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[len(lines)-1]) != "" {
		return gap
	}

	middle := lines[1 : len(lines)-1]
	blank := false
	for _, l := range middle {
		blank = blank || strings.TrimSpace(l) == ""
	}

	out := []string{lines[0]}
	if want && !blank {
		out = append(out, "")
	}

	for _, l := range middle {
		if !want && strings.TrimSpace(l) == "" {
			continue
		}

		out = append(out, l)
	}

	return strings.Join(append(out, lines[len(lines)-1]), "\n")
}

// unitEnd extends an item to the end of its own line so a trailing comment
// travels with the assignment it documents. Multiline values, including heredocs
// and templates, stop at the value: what follows their last line cannot be told
// apart from expression code.
func (c *checker) unitEnd(e entry) int {
	b := e.value.Range().End.Byte
	if e.value.Range().Start.Line != e.value.Range().End.Line {
		return b
	}

	end := c.lineEnd(b)
	for end > b && (c.src[end-1] == ' ' || c.src[end-1] == '\t') {
		end--
	}

	if strings.Contains(string(c.src[b:end]), "/*") && !strings.Contains(string(c.src[b:end]), "*/") {
		return b
	}

	return end
}

func commentLine(s string) bool {
	switch {
	case strings.HasPrefix(s, "#"), strings.HasPrefix(s, "//"):
		return !strings.Contains(s, "*/") || strings.HasSuffix(s, "*/")
	default:
		return strings.HasPrefix(s, "/*") && strings.HasSuffix(s, "*/")
	}
}

// unitStart pulls comment-only lines directly above an item into its unit, so
// reordering keeps a comment attached to the assignment below it. Comments that
// sit at the top of the body are its header rather than an item note, so they end
// the walk and pin the run instead.
func (c *checker) unitStart(line int, pad string, bodyStart int) int {
	from := line
	brace := c.lineStart(bodyStart)
	for from > brace {
		prev := c.lineStart(from - 1)
		if prev <= brace || c.lineStart(prev-1) == brace || indentOf(string(c.src[prev:from-1])) != pad {
			break
		}

		text := strings.TrimSpace(strings.TrimRight(string(c.src[prev:from-1]), " \t"))
		if !commentLine(text) {
			break
		}

		from = prev
	}

	return from
}

const (
	groupSingle = iota
	groupMulti
	groupDep
)

// unit is one assignment plus the comments that move with it.
type unit struct {
	value hclsyntax.Expression
	text  string
	key   string
	pad   string
	from  int
	to    int
	group int
}

// reorder rewrites runs of consecutive assignments into the canonical order:
// single-line items first, then multiline items, then depends_on, each run
// sorted by descending key and value length. Runs broken by an uncertain comment
// or by inconsistent indentation are left alone.
func (c *checker) reorder(es []entry, body bool, start int, end int, blocks []*hclsyntax.Block) {
	if !c.fix || len(es) < 2 {
		return
	}

	units := []unit{}
	for _, e := range es {
		line := c.lineStart(e.kr.Start.Byte)
		pad := string(c.src[line:e.kr.Start.Byte])
		if indentOf(pad) != pad {
			continue
		}

		group := groupSingle
		switch {
		case body && e.key == "depends_on":
			group = groupDep
		case multi(e):
			group = groupMulti
		}

		from := c.unitStart(line, pad, start)
		to := c.unitEnd(e)
		units = append(units, unit{
			value: e.value,
			text:  string(c.src[from:to]),
			key:   e.key,
			pad:   pad,
			from:  from,
			to:    to,
			group: group,
		})
	}

	for i := 0; i < len(units); {
		j := i + 1
		for j < len(units) {
			gap := string(c.src[units[j-1].to:units[j].from])
			if strings.TrimSpace(gap) != "" || units[j].pad != units[i].pad {
				break
			}

			// An item that does not end its own line leaves text that may be
			// expression code rather than a comment.
			if units[j-1].to != c.lineEnd(units[j-1].to) {
				break
			}

			j++
		}

		if j > i+1 && !c.pinned(start, end, blocks, units, i, j) {
			c.reorderRun(units[i:j])
		}

		i = j
	}
}

// pinned reports whether a comment outside a run would be stranded by a reorder.
// Text above the first item may describe that item or the whole body, and a
// comment below the last item that has nothing after it can only describe that
// item. A comment followed by another item or block belongs to what follows.
func (c *checker) pinned(start, end int, blocks []*hclsyntax.Block, us []unit, i, j int) bool {
	from, to := us[i].from, us[j-1].to
	body := end
	for _, b := range blocks {
		if e := b.Range().End.Byte; e < from && e > start {
			start = e
		}

		if st := b.Range().Start.Byte; st > to && st < end {
			end = st
		}
	}

	if c.hasComment(start, from) {
		return true
	}

	return j == len(us) && end == body && c.hasComment(to, end)
}

// hasComment reports whether a region holds a comment token.
func (c *checker) hasComment(from, to int) bool {
	if to <= from {
		return false
	}

	tokens, _ := hclsyntax.LexExpression(c.src[from:to], "", hcl.Pos{Line: 1, Column: 1})
	for _, t := range tokens {
		if t.Type == hclsyntax.TokenComment {
			return true
		}
	}

	return false
}

func (c *checker) precedes(a, b unit) bool {
	if a.group != b.group {
		return a.group < b.group
	}

	if width(a.key) != width(b.key) {
		return width(a.key) > width(b.key)
	}

	return width(strings.TrimSpace(c.raw(a.value.Range()))) > width(strings.TrimSpace(c.raw(b.value.Range())))
}

// duplicateKey reports an object literal that repeats a key. HCL applies object
// entries in source order, so the later one wins and the order is semantic.
// Bodies cannot repeat an attribute, so only object items can.
func duplicateKey(us []unit) bool {
	seen := map[string]bool{}
	for _, u := range us {
		if seen[u.key] {
			return true
		}

		seen[u.key] = true
	}

	return false
}

func (c *checker) reorderRun(us []unit) {
	if duplicateKey(us) {
		return
	}

	want := append([]unit{}, us...)
	sort.SliceStable(want, func(i, j int) bool { return c.precedes(want[i], want[j]) })

	same := true
	for i := range want {
		same = same && want[i].text == us[i].text
	}

	if same {
		return
	}

	var b strings.Builder

	for i, u := range want {
		b.WriteString(u.text)
		if i == len(want)-1 {
			break
		}

		if u.group == groupSingle && want[i+1].group == groupSingle {
			b.WriteString("\n")
			continue
		}

		b.WriteString("\n\n")
	}

	c.addEdit(us[0].from, us[len(us)-1].to, b.String(), rankOrder)
}

// place renders one collection entry at the requested indentation.
func (c *checker) place(r hcl.Range, inner string) (string, bool) {
	text := string(c.src[r.Start.Byte:r.End.Byte])
	cur := ""
	if c.ownLine(r.Start.Byte) {
		cur = string(c.src[c.lineStart(r.Start.Byte):r.Start.Byte])
	}

	if strings.ContainsRune(cur, '\t') {
		return "", false
	}

	if !strings.Contains(text, "\n") {
		return strings.TrimLeft(text, " \t"), true
	}

	if strings.Contains(text, "<<") || strings.Contains(text, "\t") {
		return "", false
	}

	delta := width(inner) - width(cur)
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))

	for i, l := range lines {
		if i == 0 {
			out = append(out, inner+strings.TrimLeft(l, " \t"))
			continue
		}

		if strings.TrimSpace(l) == "" {
			out = append(out, "")
			continue
		}

		if delta < 0 {
			drop := min(-delta, len(indentOf(l)))
			l = l[drop:]
		}

		out = append(out, strings.Repeat(" ", max(delta, 0))+l)
	}

	return strings.Join(out, "\n"), true
}

// entryComments maps collection entries, indexed by position, to the comment
// that trails them on their own line so a rewrite can keep it. It refuses when a
// comment sits anywhere else, because that text has no entry to move with.
func (c *checker) entryComments(r hcl.Range, items []hcl.Range) (map[int]string, bool) {
	tokens, _ := hclsyntax.LexExpression(c.src[r.Start.Byte:r.End.Byte], "", hcl.Pos{Line: 1, Column: 1})
	out := map[int]string{}

	for _, t := range tokens {
		if t.Type != hclsyntax.TokenComment {
			continue
		}

		at := r.Start.Byte + t.Range.Start.Byte
		text := strings.TrimSpace(string(t.Bytes))
		owner := -1

		for i, it := range items {
			if it.End.Byte <= at && at < c.lineEnd(it.End.Byte) && c.lastOnLine(items, it) {
				owner = i
			}
		}

		if owner < 0 || !commentLine(text) {
			return nil, false
		}

		out[owner] += " " + text
	}

	return out, true
}

// lastOnLine reports whether no other entry of the collection starts on r's last
// line. Entries may arrive ordered by content, so compare by position.
func (c *checker) lastOnLine(items []hcl.Range, r hcl.Range) bool {
	end := c.lineEnd(r.End.Byte)
	for _, o := range items {
		if o.Start.Byte > r.End.Byte && o.Start.Byte < end {
			return false
		}
	}

	return true
}

// fixCollection rewrites a collection that the separate-entry-lines rule rejects
// so every entry sits on its own line between delimiters of its own. Comments
// that are not a trailing comment on an entry's own line stop the rewrite.
func (c *checker) fixCollection(r hcl.Range, items []hcl.Range) {
	if len(items) == 0 || r.End.Byte <= r.Start.Byte+1 {
		return
	}

	open := string(c.src[r.Start.Byte])
	shut := string(c.src[r.End.Byte-1])
	tuple := open == "["
	if !tuple && open != "{" {
		return
	}

	if (tuple && shut != "]") || (!tuple && shut != "}") {
		return
	}

	pad := indentOf(string(c.src[c.lineStart(r.Start.Byte):r.Start.Byte]))
	if strings.ContainsRune(pad, '\t') {
		return
	}

	if !tuple {
		for _, it := range items {
			text := string(c.src[it.Start.Byte:it.End.Byte])
			if !strings.Contains(text, "=") && !strings.Contains(text, ":") {
				return
			}
		}
	}

	trails, ok := c.entryComments(r, items)
	if !ok {
		return
	}

	inner := pad + "  "
	texts := make([]string, 0, len(items))
	for i, it := range items {
		// Entries may arrive sorted by content rather than by position, so test
		// for overlap instead of assuming the source order.
		if it.Start.Byte < r.Start.Byte || it.End.Byte > r.End.Byte {
			return
		}

		for j := range i {
			if it.Start.Byte < items[j].End.Byte && items[j].Start.Byte < it.End.Byte {
				return
			}
		}

		text, ok := c.place(it, inner)
		if !ok {
			return
		}

		if tuple && i < len(items)-1 {
			text += ","
		}

		texts = append(texts, text+trails[i])
	}

	c.addEdit(r.Start.Byte, r.End.Byte, open+"\n"+inner+strings.Join(texts, "\n"+inner)+"\n"+pad+shut, rankColl)
}

// clash reports whether two edits cannot both apply. A zero-length edit inserts
// at one point, so any proposal touching that point competes with it.
func clash(a, b edit) bool {
	if a.start == a.end {
		return b.start <= a.start && a.start <= b.end
	}

	if b.start == b.end {
		return a.start <= b.start && b.start <= a.end
	}

	return a.start < b.end && b.start < a.end
}

// applyEdits keeps the highest ranked proposal wherever edits overlap.
func applyEdits(src []byte, edits []edit) []byte {
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].rank != edits[j].rank {
			return edits[i].rank > edits[j].rank
		}

		return edits[i].start < edits[j].start
	})

	kept := []edit{}
	for _, e := range edits {
		ok := true
		for _, k := range kept {
			if clash(e, k) {
				ok = false
				break
			}
		}

		if !ok {
			continue
		}

		kept = append(kept, e)
	}

	sort.Slice(kept, func(i, j int) bool {
		if kept[i].start != kept[j].start {
			return kept[i].start < kept[j].start
		}

		return kept[i].end > kept[j].end
	})

	out := bytes.Buffer{}
	prev := 0
	for _, k := range kept {
		out.Write(src[prev:k.start])
		out.WriteString(k.text)
		prev = k.end
	}

	out.Write(src[prev:])

	return out.Bytes()
}

func syntaxErrors(src []byte, name string) bool {
	_, diags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)

	return diags.HasErrors()
}

// tokenBag summarizes the meaningful tokens of a file. Formatting may drop entry
// separators, turn a colon into an equals sign, and move text around, but it may
// never add, drop, or change a token.
func tokenBag(src []byte) map[string]int {
	tokens, _ := hclsyntax.LexConfig(src, "", hcl.InitialPos)
	bag := map[string]int{}

	for _, t := range tokens {
		if t.Type == hclsyntax.TokenNewline {
			continue
		}

		bag[tokenKey(t)]++
	}

	return bag
}

func tokenKey(t hclsyntax.Token) string {
	if t.Type == hclsyntax.TokenColon {
		return hclsyntax.TokenEqual.String() + "\x00="
	}

	return t.Type.String() + "\x00" + string(t.Bytes)
}

func tokensKept(before, after map[string]int) bool {
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}

	for k := range after {
		keys[k] = true
	}

	for k := range keys {
		comma := strings.HasPrefix(k, hclsyntax.TokenComma.String()+"\x00")
		if comma && after[k] <= before[k] {
			continue
		}

		if before[k] != after[k] {
			return false
		}
	}

	return true
}

// fixDepends rewrites a depends_on tuple onto one line per dependency, longest
// reference first, and without a trailing comma.
func (c *checker) fixDepends(t *hclsyntax.TupleConsExpr, move bool) {
	if len(t.Exprs) == 0 {
		return
	}

	rs := []hcl.Range{}
	for _, x := range t.Exprs {
		rs = append(rs, x.Range())
	}

	if move {
		sort.SliceStable(rs, func(i, j int) bool { return width(c.raw(rs[i])) > width(c.raw(rs[j])) })
	}

	c.fixCollection(t.Range(), rs)
}

// ownLine reports whether the text from the start of b's line to b is only
// indentation, so b sits at its line's indent.
func (c *checker) ownLine(b int) bool {
	pad := string(c.src[c.lineStart(b):b])

	return indentOf(pad) == pad
}

// declined names the reason a rewritten file is refused, or "" when it is fine.
func declined(after []byte, name string, bag map[string]int) string {
	switch {
	case syntaxErrors(after, name):
		return "rewrite does not parse"
	case !tokensKept(bag, tokenBag(after)):
		return "rewrite changes the token stream"
	default:
		return ""
	}
}

// format rewrites src until no repair remains applicable, then reports whatever
// violations the mechanical fixes cannot reach.
func format(src []byte, name string) ([]byte, []issue) {
	if bytes.ContainsRune(src, '\r') {
		return src, lint(src, name)
	}

	before := tokenBag(src)
	out, note := src, issue{}
	issues := lint(src, name)

	for range formatPasses {
		found, edits := analyze(out, name, true)
		issues = found

		if len(found) == 0 || len(edits) == 0 {
			break
		}

		next := applyEdits(out, edits)
		if bytes.Equal(next, out) {
			break
		}

		// A rewrite that changes the token stream or no longer parses is refused:
		// the file keeps the text from the previous pass and says why.
		if reason := declined(next, name, before); reason != "" {
			note = issue{
				message: reason + "; later repairs were skipped",
				rule:    "format",
				line:    found[0].line,
			}

			break
		}

		out = next
	}

	if bytes.Equal(out, src) {
		return out, issues
	}

	issues = lint(out, name)
	if note.line > 0 {
		issues = append(issues, note)
	}

	return out, issues
}
