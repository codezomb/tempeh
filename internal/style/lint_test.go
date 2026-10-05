package style

import "testing"

func TestRules(t *testing.T) {
	tests := []struct{ name, src, rule string }{
		{"valid", "locals {\n  longer = 1\n  short  = 2\n\n  items = [\n    1,\n    2\n  ]\n}\n", ""},
		{"nested", "locals {\n  value = yamlencode({\n    longer = 1\n    short  = 2\n  })\n}\n", ""},
		{"ordering", "locals {\n  a      = 1\n  longer = 2\n}\n", "order"},
		{"alignment", "locals {\n  longer = 1\n  short = 2\n}\n", "spacing"},
		{"inline tuple", "locals {\n  value = [1, 2]\n}\n", "collection"},
		{"inline object", "locals {\n  value = { a = 1, b = 2 }\n}\n", "collection"},
		{"nested ordering", "locals {\n  value = yamlencode({\n    a      = 1\n    longer = 2\n  })\n}\n", "order"},
		{"comment spacing", "locals {\n  longer = 1 # explanation\n  # another explanation\n  short  = 2\n}\n", ""},
		{"value tie", "locals {\n  aa = 1\n  bb = 222\n}\n", "order"},
		{"group", "locals {\n  items = [\n    1,\n    2\n  ]\n  value = 3\n}\n", "group"},
		{"blank", "locals {\n  value = 1\n  items = [\n    1,\n    2\n  ]\n}\n", "blank"},
		{"scalar blank", "locals {\n  longer = 1\n\n  short  = 2\n}\n", "blank"},
		{"first blank", "locals {\n\n  value = 1\n}\n", "blank"},
		{"nested first blank", "locals {\n  value = {\n\n    key = 1\n  }\n}\n", "blank"},
		{"comment brace", "locals {\n  aa = 1\n  # { is only a comment\n  bb = 222\n}\n", "order"},
		{"single dependency inline", "resource \"x\" \"y\" {\n  depends_on = [x.a]\n}\n", "depends"},
		{"dependency after block no gap", "resource \"x\" \"y\" {\n  lifecycle { prevent_destroy = true }\n  depends_on = [\n    x.a\n  ]\n}\n", "blank"},
		{"variable", "variable \"x\" {\n  description = \"test\"\n  sensitive   = true\n  default     = 1\n  type        = number\n}\n", ""},
		{"dependency order", "resource \"x\" \"y\" {\n  name = \"test\"\n\n  depends_on = [\n    x.a,\n    x.long\n  ]\n}\n", "depends"},
		{"dependency comma", "resource \"x\" \"y\" {\n  depends_on = [\n    x.a,\n  ]\n}\n", "depends"},
		{"dependency block last", "resource \"x\" \"y\" {\n  depends_on = [x.a]\n  lifecycle { prevent_destroy = true }\n}\n", "depends"},
		{"valid dependency", "resource \"x\" \"y\" {\n  lifecycle { prevent_destroy = true }\n\n  depends_on = [\n    x.long,\n    x.a\n  ]\n}\n", ""},
		{"syntax", "locals { value = [ }", "syntax"},
		{"heredoc", "locals {\n  text = <<EOT\na = { fake = [1, 2] }\nEOT\n}\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := lint([]byte(tt.src), "test.tf")
			if tt.rule == "" {
				if len(issues) > 0 {
					t.Fatalf("unexpected: %+v", issues)
				}
				return
			}
			for _, i := range issues {
				if i.rule == tt.rule {
					return
				}
			}
			t.Fatalf("missing %s in %+v", tt.rule, issues)
		})
	}
}
