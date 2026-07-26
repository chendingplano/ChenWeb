package rendering

import "testing"

func TestEscapeTypst_MarkupCharactersNeutralized(t *testing.T) {
	got := escapeTypst("#set page(width: 1pt)")
	if got == "#set page(width: 1pt)" {
		t.Fatalf("expected # to be escaped, got unescaped: %q", got)
	}
	want := `\#set page(width: 1pt)`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEscapeTypst_BackslashEscapedFirstNoDoubleEscape(t *testing.T) {
	got := escapeTypst(`\#`)
	want := `\\\#`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEscapeTypst_AllMetacharacters(t *testing.T) {
	cases := map[string]string{
		`\`: `\\`,
		"#": `\#`,
		"$": `\$`,
		"*": `\*`,
		"_": `\_`,
		"`": "\\`",
		"@": `\@`,
		"<": `\<`,
		">": `\>`,
		"[": `\[`,
		"]": `\]`,
	}
	for in, want := range cases {
		if got := escapeTypst(in); got != want {
			t.Errorf("escapeTypst(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeTypst_PlainTextUnchanged(t *testing.T) {
	s := "Jaro-Winkler similarity is designed for short strings."
	if got := escapeTypst(s); got != s {
		t.Fatalf("expected plain text unchanged, got %q", got)
	}
}
