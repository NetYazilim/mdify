package medium

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func convertFixture(t *testing.T, name, src string) *Result {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(string(b), src)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func checkMarkdown(t *testing.T, md string, want, bad []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(md, w) {
			t.Errorf("çıktıda yok: %q", w)
		}
	}
	for _, b := range bad {
		if strings.Contains(md, b) {
			t.Errorf("çıktıda olmamalı: %q", b)
		}
	}
	if t.Failed() {
		t.Log(md)
	}
}

func TestMirrorImages(t *testing.T) {
	res := convertFixture(t, "mirror.html",
		"https://mirror.invalid/https://medium.com/@user/redis-complete-guide-b36706fa7a98")
	if res.Title != "Redis: The Complete Guide from Zero to Production" {
		t.Errorf("title = %q", res.Title)
	}
	if res.Author != "Alok Ranjan Singh" {
		t.Errorf("author = %q", res.Author)
	}
	md := res.Markdown
	checkMarkdown(t, md, []string{
		// kapak başlığın hemen altında, Medium'daki gibi
		"# Redis: The Complete Guide from Zero to Production\n\n![captionless image](https://miro.medium.com/v2/resize:fit:1400/1*yI4tM_afUhInKTlJnO6Rmg.png)\n\n[Reference](https://medium.com/@user/redis-complete-guide-b36706fa7a98)\n\nby Alok Ranjan Singh",
		"How to think like a Redis engineer.",
		"Our pizza menu is a perfect candidate for RAM.\n\n![captionless image](https://miro.medium.com/v2/resize:fit:1400/1*BT9jchDU6wny4XY7S1yfgQ.png)\n\n### What is a Cache?",
		"![Cache diagram](https://miro.medium.com/v2/resize:fit:1400/1*SphE9pB5_LzxnDiezguiyg.png)",
		"Final words.\n\n![captionless image](https://miro.medium.com/v2/resize:fit:1400/1*yI4tM_afUhInKTlJnO6Rmg.png)",
	}, []string{
		"/img/medium/", "medium.com/img", "mirror.invalid", "](None", "[None]", "Post cover image",
		"8zXBu5arNA8ys5F5vBb2JA", // avatar
		"July 10, 2026", "22 min read", "Download article", "Contents", "Show all", "By Alok",
	})
	if n := strings.Count(md, "!["); n != 4 {
		t.Errorf("resim sayısı = %d, beklenen 4", n)
	}
}

func TestMirrorCaptionedCover(t *testing.T) {
	res := convertFixture(t, "mirror_caption.html", "https://mirror.invalid/https://medium.com/@x/y-96a0ef94c151")
	if res.Author != "KanishkSingh" {
		t.Errorf("author = %q", res.Author)
	}
	checkMarkdown(t, res.Markdown, []string{
		"# AWS Just Put DuckDB Inside PostgreSQL.\n\n![Image by Mehmet Ali Peker](https://miro.medium.com/v2/resize:fit:1400/1*YJO0ciwGIv-6XO46mlgkEw.avif)\n\n[Reference](https://medium.com/@singhkanishk098/aws-just-put-duckdb-inside-postgresql-data-engineers-have-some-rethinking-to-do-96a0ef94c151)",
		"### The pipeline you've been maintaining",
	}, []string{"/img/medium/", "GjSCfJMgiKSLheWWAG2n2w", "4 min read", "Post cover image"})
	if n := strings.Count(res.Markdown, "!["); n != 1 {
		t.Errorf("resim sayısı = %d, beklenen 1", n)
	}
}

func TestImageURL(t *testing.T) {
	const medium, mirror = "https://medium.com", "https://mirror.invalid"
	cases := []struct{ src, base, want string }{
		{"/img/medium/700/1*abc.png", mirror, "https://miro.medium.com/v2/resize:fit:1400/1*abc.png"},
		{"https://mirror.invalid/img/medium/4000/1*abc.jpeg", mirror, "https://miro.medium.com/v2/resize:fit:1400/1*abc.jpeg"},
		{"https://miro.medium.com/v2/resize:fit:700/1*abc.png", medium, "https://miro.medium.com/v2/resize:fit:700/1*abc.png"},
		// Medium dışı kaynak: mirror'ın kendi adresine göre çözülür
		{"/img/source/width=1424,quality=80,format=auto/content-assets/images/x.jpg", mirror,
			"https://mirror.invalid/img/source/width=1424,quality=80,format=auto/content-assets/images/x.jpg"},
		{"/img/700/0*Fbkg1VCzlwpEsFnd.jpg", mirror, "https://miro.medium.com/v2/resize:fit:1400/0*Fbkg1VCzlwpEsFnd.jpg"},
		{"https://mirror.invalid/img/4000/1*abc.png", mirror, "https://miro.medium.com/v2/resize:fit:1400/1*abc.png"},
		{"/plain.png", medium, "https://medium.com/plain.png"},
		{"//miro.medium.com/a.png", medium, "https://miro.medium.com/a.png"},
		{"https://miro.medium.com/v2/resize:fill:64:64/1*x-avatar.png", mirror, "https://miro.medium.com/v2/resize:fill:64:64/1*x-avatar.png"},
	}
	for _, c := range cases {
		if got := imageURL(c.src, c.base); got != c.want {
			t.Errorf("imageURL(%q, %q) = %q, beklenen %q", c.src, c.base, got, c.want)
		}
	}
}

// Medium dışı bir kaynağın göreli resimleri, mirror sayfasında mirror'ın
// adresine göre mutlak yapılmalı (medium.com'a değil).
func TestMirrorNonMediumImage(t *testing.T) {
	page := `<html><body><article>
<div data-nosnippet=""><nav><a aria-label="Open original article" href="https://medium.com/@news/story-1a2b3c">o</a></nav></div>
<header><p>November 19, 2026</p><h1>Story</h1><div><div><img src="https://miro.medium.com/v2/resize:fill:64:64/1*news-avatar.png" alt=""></div><div><p>By Example News</p></div></div></header>
<div><div class="prose"><p>Text.</p><p><img src="/img/source/width=1424/images/a.jpg" alt="A photo"></p></div></div>
</article></body></html>`
	res, err := Convert(page, "https://mirror.invalid/https://medium.com/@news/story-1a2b3c")
	if err != nil {
		t.Fatal(err)
	}
	checkMarkdown(t, res.Markdown, []string{
		"![A photo](https://mirror.invalid/img/source/width=1424/images/a.jpg)",
		"[Reference](https://medium.com/@news/story-1a2b3c)",
	}, []string{"medium.com/img", "news-avatar"})
}

// Mirror başka bir domaine taşınsa ve "Open original article" linki olmasa da
// sayfa yapıdan tanınmalı, Reference gömülü URL'den gelmeli.
func TestMirrorOtherDomainNoOriginalLink(t *testing.T) {
	b, err := os.ReadFile("testdata/mirror_caption.html")
	if err != nil {
		t.Fatal(err)
	}
	page := strings.Replace(string(b), ` aria-label="Open original article"`, "", 1)
	page = strings.Replace(page, "</title>", "</title>", 1)
	res, err := Convert(page, "https://mirror.invalid/https://medium.com/@x/y-96a0ef94c151")
	if err != nil {
		t.Fatal(err)
	}
	checkMarkdown(t, res.Markdown, []string{
		"![Image by Mehmet Ali Peker](https://miro.medium.com/v2/resize:fit:1400/1*YJO0ciwGIv-6XO46mlgkEw.avif)",
		"[Reference](https://medium.com/@x/y-96a0ef94c151)",
		"by KanishkSingh",
	}, []string{"/img/medium/", "mirror.invalid", "GjSCfJMgiKSLheWWAG2n2w"})
}

func TestEmbeddedURL(t *testing.T) {
	cases := map[string]string{
		"https://mirror.invalid/https://medium.com/@a/b-1":              "https://medium.com/@a/b-1",
		"https://mirror.invalid/https://levent.medium.com/b?source=rss": "https://levent.medium.com/b?source=rss",
		"https://medium.com/@a/b-1":                                     "",
		"https://mirror.invalid/http://medium.com/@a/b":                 "",
		"https://mirror.invalid/":                                       "",
	}
	for in, want := range cases {
		if got := embeddedURL(in); got != want {
			t.Errorf("embeddedURL(%q) = %q, beklenen %q", in, got, want)
		}
	}
}

func TestMetaMirror(t *testing.T) {
	res := convertFixture(t, "meta_mirror.html",
		"https://mirror.invalid/https://medium.com/python-in-plain-english/the-python-trick-behind-decorators-what-really-does-4eedb26fe77e")
	if res.Title != "The Python Trick Behind Decorators — What @ Really Does" {
		t.Errorf("title = %q", res.Title)
	}
	if res.Author != "Zayd Islam" {
		t.Errorf("author = %q", res.Author)
	}
	md := res.Markdown
	checkMarkdown(t, md, []string{
		"# The Python Trick Behind Decorators — What @ Really Does\n\n[Reference](https://medium.com/python-in-plain-english/the-python-trick-behind-decorators-what-really-does-4eedb26fe77e)\n\nby Zayd Islam\n\nPython Tricks That Make Senior Engineers",
		"[Read part 1](https://medium.com/@zayd/part-1)\n\n![captionless image](https://miro.medium.com/v2/resize:fit:1400/1*TdpN_DW0lBu8a91uoPG6Xg.png)\n\nHave you ever",
		"```python\n@app.route(\"/users\")\ndef get_users():\n    return [\"Alice\", \"Bob\"]\n```",
		"```python\n@property\ndef full_name(self):\n    return f\"{self.first} {self.last}\"\n```",
		"![How a decorator wraps a function](https://miro.medium.com/v2/resize:fit:1400/1*second.png)",
	}, []string{"Oct 10, 2026", "8 min read", "mirror.invalid", "Unknown Author", "Download", "Original on Medium"})
	if n := strings.Count(md, "medium.com/python-in-plain-english/the-python-trick"); n != 1 {
		t.Errorf("orijinal URL %d kez geçiyor, beklenen 1 (sadece Reference)", n)
	}
}

func TestCodeLang(t *testing.T) {
	cases := map[string]string{
		`<pre><span data-code-block-lang="go">x</span></pre>`:    "go",
		`<pre><code class="language-python hljs">x</code></pre>`: "python",
		`<pre class="lang-js"><code>x</code></pre>`:              "js",
		`<pre><code class="hljs">x</code></pre>`:                 "",
	}
	for in, want := range cases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(in))
		if err != nil {
			t.Fatal(err)
		}
		if got := codeLang(doc.Find("pre").First()); got != want {
			t.Errorf("codeLang(%s) = %q, beklenen %q", in, got, want)
		}
	}
}

// Mirror sayfasındaki lazy-load video iframe'leri (src boş, adres data-src'de)
// çalışan bir iframe'e çevrilmeli; adressiz iframe'ler atılmalı.
func TestLazyIframe(t *testing.T) {
	page := `<html><body><article>
<div data-nosnippet=""><nav><a aria-label="Open original article" href="https://medium.com/@news/story-1a2b3c">o</a></nav></div>
<header><h1>Story</h1></header>
<div><div class="prose"><p>Before video.</p>
<div><div><div style="position: relative; width: 100%; height: 315px;"><div class="animate-pulse bg-zinc-200 absolute inset-0"></div><iframe frameborder="0" style="opacity: 0;" sandbox="allow-same-origin" data-lazy-initialized="true" data-src="https://medium.com/media/?autoplay=false&amp;id=42"></iframe></div></div></div>
<p>Between.</p>
<iframe style="x" data-foo="1"></iframe>
<p>After video.</p></div></div>
</article></body></html>`
	res, err := Convert(page, "https://mirror.invalid/https://medium.com/@news/story-1a2b3c")
	if err != nil {
		t.Fatal(err)
	}
	checkMarkdown(t, res.Markdown, []string{
		`<iframe src="https://medium.com/media/?autoplay=false&amp;id=42" frameborder="0" width="100%" height="315"></iframe>`,
		"Before video.", "Between.", "After video.",
	}, []string{"data-src", "sandbox", "style=", "opacity", "data-foo", "animate-pulse"})
	if n := strings.Count(res.Markdown, "<iframe"); n != 1 {
		t.Errorf("iframe sayısı = %d, beklenen 1", n)
	}
}

// Poster'lı video iframe'inin altına tıklanabilir kapak resmi eklenmeli
// (iframe göstermeyen Markdown görüntüleyiciler için); poster yoksa eklenmemeli.
func TestVideoPosterFallback(t *testing.T) {
	page := `<html><body><article>
<div data-nosnippet=""><nav><a aria-label="Open original article" href="https://medium.com/@news/story-1a2b3c">o</a></nav></div>
<header><h1>Story</h1></header>
<div><div class="prose"><p>Before.</p>
<div><div><iframe sandbox="allow-same-origin" data-src="https://medium.com/media/?autoplay=false&amp;mid=781541&amp;poster=https://miro.medium.com/v2/resize:fit:1400/1*poster.jpg"></iframe></div></div>
<p>Middle.</p>
<figure><iframe src="https://medium.com/media/abc123?src=yt" width="640"></iframe><figcaption>Clip</figcaption></figure>
<p>After.</p></div></div>
</article></body></html>`
	res, err := Convert(page, "https://mirror.invalid/https://medium.com/@news/story-1a2b3c")
	if err != nil {
		t.Fatal(err)
	}
	md := res.Markdown
	checkMarkdown(t, md, []string{
		`<iframe src="https://medium.com/media/?autoplay=false&amp;mid=781541&amp;poster=https://miro.medium.com/v2/resize:fit:1400/1*poster.jpg" width="100%" height="315"></iframe>` +
			"\n\n[![Video](https://miro.medium.com/v2/resize:fit:1400/1*poster.jpg)](https://medium.com/media/?autoplay=false&mid=781541&poster=https%3A%2F%2Fmiro.medium.com%2Fv2%2Fresize%3Afit%3A1400%2F1%2Aposter.jpg)\n\nMiddle.",
		// sadece eksik olan (height) tamamlanır, mevcut width korunur
		`<iframe src="https://medium.com/media/abc123?src=yt" width="640" height="315"></iframe>` + "\n\n*Clip*",
	}, nil)
	if n := strings.Count(md, "![Video]"); n != 1 {
		t.Errorf("poster sayısı = %d, beklenen 1", n)
	}
}

// Yeni mirror düzeni: /img/<w>/<id> resim yolları, data-caption altyazıları ve
// tema başına iki kez basılan (açık/koyu) kod blokları.
func TestMirrorThemedCodeAndNewImagePaths(t *testing.T) {
	res := convertFixture(t, "mirror_code.html",
		"https://mirror.invalid/https://mvineetsharma.medium.com/11-kafka-design-patterns-for-every-backend-engineer-f053bdadd99e")
	if res.Author != "Vineet Sharma" {
		t.Errorf("author = %q", res.Author)
	}
	md := res.Markdown
	checkMarkdown(t, md, []string{
		"# 11 Kafka Design Patterns for Every Backend Engineer\n\n![11 Kafka Design Patterns for Every Backend Engineer](https://miro.medium.com/v2/resize:fit:1400/0*nsKaZzd3eHByK17E.jpg)\n\n[Reference](https://mvineetsharma.medium.com/11-kafka-design-patterns-for-every-backend-engineer-f053bdadd99e)",
		"![Event Sourcing](https://miro.medium.com/v2/resize:fit:1400/0*Fbkg1VCzlwpEsFnd.jpg)",
		"![Event Sourcing](https://miro.medium.com/v2/resize:fit:1400/0*poal1v-dcvJzybo2.png)",
		"```\n# Producer - append only\nevent = {\"event_type\": \"OrderCreated\"}\n```",
	}, []string{"/img/", "mirror.invalid", "u0kG3U0iiFtJZ4gYQPH4xQ", "captionless", "View Source", "None", "code-copy"})
	if n := strings.Count(md, "# Producer - append only"); n != 1 {
		t.Errorf("ilk kod bloğu %d kez geçiyor, beklenen 1", n)
	}
	// Yazarın bilerek tekrarladığı blok (ayrı kapsayıcılar) korunmalı: 2 kez.
	if n := strings.Count(md, "read_model.get(id)"); n != 2 {
		t.Errorf("read_model bloğu %d kez geçiyor, beklenen 2", n)
	}
}

func TestCaptionText(t *testing.T) {
	cases := map[string]string{
		"Event Sourcing": "Event Sourcing",
		`Event Sourcing [<a href="https://medium.com/@user/diagram-01">View Source<span><svg><path/></svg></span></a>]`: "Event Sourcing",
		"Photo by <a href=\"https://medium.com/@someone\">Someone</a> on Medium":                                        "Photo by Someone on Medium",
		"":     "",
		"None": "",
	}
	for in, want := range cases {
		if got := captionText(in); got != want {
			t.Errorf("captionText(%q) = %q, beklenen %q", in, got, want)
		}
	}
}
