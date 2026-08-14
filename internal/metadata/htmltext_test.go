package metadata

import "testing"

func TestHTMLToText(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"plain":                         "plain",
		"<p>one</p><p>two</p>":          "one\n\ntwo",
		"a<br>b":                        "a\n\nb",
		"<i>x</i> &amp; <b>y</b>":       "x & y",
		"&quot;q&quot; &#39;s&#39;":     `"q" 's'`,
		"<p>trailing space </p>":        "trailing space",
		"<ul><li>a</li><li>b</li></ul>": "a\n\nb",
		"a b":                           "a b", // non-breaking space
	}
	for in, want := range cases {
		if got := HTMLToText(in); got != want {
			t.Errorf("HTMLToText(%q) = %q, want %q", in, got, want)
		}
	}
}
