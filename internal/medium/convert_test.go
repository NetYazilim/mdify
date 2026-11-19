package medium

import (
	"os"
	"strings"
	"testing"
)

func TestConvertFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/article.html")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(string(b), "https://medium.com/@levent/go-ornek-123")
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Go ile Örnek Yazı" {
		t.Errorf("title = %q", res.Title)
	}
	if res.Author != "Levent" {
		t.Errorf("author = %q", res.Author)
	}

	md := res.Markdown
	want := []string{
		"# Go ile Örnek Yazı\n\n![Kapak görseli](https://miro.medium.com/v2/resize:fit:1400/format:webp/1*a.png)",
		"[Reference](https://medium.com/@levent/go-ornek-123)",
		"by [Levent](https://medium.com/@levent?source=post_page)",
		"[göreli link](https://medium.com/tag/go)",
		"```go\nfunc main() {\n    fmt.Println(\"a < b\")\n}\n```",
		"<iframe src=\"https://medium.com/media/abc123?src=yt\"",
		"![alt metni](https://medium.com/plain.png)",
		"[nameless link](https://medium.com/about)",
	}
	for _, w := range want {
		if !strings.Contains(md, w) {
			t.Errorf("çıktıda yok: %q", w)
		}
	}
	for _, bad := range []string{"5 min read", "Follow", "Listen", "Share", "footer", "Sign in"} {
		if strings.Contains(md, bad) {
			t.Errorf("çıktıda olmamalı: %q", bad)
		}
	}
	if t.Failed() {
		t.Log(md)
	}
}

func TestNoArticle(t *testing.T) {
	if _, err := Convert("<html><body><p>x</p></body></html>", ""); err != ErrNoArticle {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateURL(t *testing.T) {
	cases := []struct {
		url string
		ok  bool
	}{
		{"https://medium.com/@a/b-1", true},
		{"https://levent.medium.com/b-1", true},
		{"https://medium.com/m/global-identity?redirectUrl=x", true},
		{"https://mirror.invalid/https://medium.com/@a/b-1", true},
		{"https://mirror.invalid/https://levent.medium.com/b-1", true},
		{"http://medium.com/@a/b", false},
		{"medium.com/@a/b", false},
		{"/https://medium.com/@a/b", false},
		{"not a url", false},
		{"", false},
	}
	for _, c := range cases {
		if err := ValidateURL(c.url); (err == nil) != c.ok {
			t.Errorf("%q: err=%v", c.url, err)
		}
	}
}

func TestUnescapeOutsideCode(t *testing.T) {
	in := "yaklaşık \\~20k token, a\\_b `x\\_y`\n```\nC:\\\\path \\~\n```\n"
	want := "yaklaşık ~20k token, a_b `x\\_y`\n```\nC:\\\\path \\~\n```\n"
	if got := unescapeOutsideCode(in); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
