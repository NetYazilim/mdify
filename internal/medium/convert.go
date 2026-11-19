// Package medium converts Medium article pages to Markdown.
package medium

import (
	"bytes"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/PuerkitoBio/goquery"
	nethtml "golang.org/x/net/html"
)

const (
	mediumURLPrefix = "https://medium.com"
	defaultTitle    = "Untitled Article"
	defaultAuthor   = "Unknown Author"

	linesToCheckForRemoval = 40
)

var (
	ErrInvalidURL = errors.New("geçersiz URL: https://… biçiminde tam bir adres verin")
	ErrNoArticle  = errors.New("sayfada makale (<article>) bulunamadı")
)

var undesiredLines = map[string]bool{
	"·": true, "Published in": true, "--": true, "1": true, "Listen": true, "Share": true,
}

// Result is the outcome of a conversion.
type Result struct {
	Title    string
	Author   string
	Markdown string
}

// ValidateURL checks that raw is an absolute https URL. Any host is accepted:
// medium.com, custom-domain Medium blogs, mirror/reader URLs
// (https://<mirror>/https://<site>/...) and other sites. Pages that cannot be
// converted are reported by Convert (e.g. ErrNoArticle).
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return ErrInvalidURL
	}
	if u.Scheme != "https" {
		return errors.New("yalnızca https adresleri destekleniyor")
	}
	return nil
}

// Convert turns the HTML of a Medium article page into Markdown.
// sourceURL is used for the "Reference" link; it may be empty.
func Convert(pageHTML, sourceURL string) (*Result, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return nil, err
	}
	if doc.Find("article").Length() == 0 {
		return nil, ErrNoArticle
	}

	title := extractTitle(doc)
	var authorName, authorLink, original string
	mirror := true
	switch {
	case isMirrorPage(doc):
		authorName, original = prepareMirror(doc)
	case isMetaMirrorPage(doc):
		authorName, original = prepareMetaMirror(doc)
	default:
		mirror = embeddedURL(sourceURL) != ""
		authorName, authorLink = extractAuthor(doc)
	}
	// Göreli resim yolları: Medium sayfasında medium.com'a, mirror sayfasında
	// mirror'ın kendi adresine göre çözülür (bilinmeyen /img/<kaynak>/… yolları).
	imgBase := mediumURLPrefix
	if mirror {
		if o := origin(sourceURL); o != "" {
			imgBase = o
		}
	}
	if original == "" {
		original = embeddedURL(sourceURL)
	}
	if original != "" {
		sourceURL = original // Reference linki mirror yerine orijinal makaleyi göstersin
	}

	removeMediumUI(doc)
	removeEmptyElements(doc)
	restructureArticle(doc, sourceURL, authorName, authorLink)
	normalizeIframes(doc)
	normalizeFigures(doc, imgBase)
	addVideoFallbacks(doc)
	normalizeImages(doc, imgBase)
	normalizeCodeBlocks(doc)

	articleHTML, err := goquery.OuterHtml(doc.Find("article").First())
	if err != nil {
		return nil, err
	}

	md, err := newConverter().ConvertString(articleHTML, converter.WithDomain(mediumURLPrefix))
	if err != nil {
		return nil, err
	}

	md = cleanMarkdown(md)
	md = filterUndesiredLines(md)
	return &Result{Title: title, Author: authorName, Markdown: strings.TrimSpace(md) + "\n"}, nil
}

// ---------------------------------------------------------------- extract

func extractTitle(doc *goquery.Document) string {
	if t := strings.TrimSpace(doc.Find("article h1").First().Text()); t != "" {
		return t
	}
	return defaultTitle
}

func extractAuthor(doc *goquery.Document) (name, link string) {
	el := doc.Find("[data-testid='authorName']").First()
	name = strings.TrimSpace(el.Text())
	if name == "" {
		name = defaultAuthor
	}
	link, _ = el.Attr("href")
	if link == "" {
		// Medium bazen data-testid'yi <a> içindeki bir elemana koyuyor.
		link, _ = el.Closest("a").Attr("href")
	}
	return name, absMedium(link)
}

// ---------------------------------------------------------------- clean DOM

func removeMediumUI(doc *goquery.Document) {
	if sp := doc.Find("div.speechify-ignore.ab.cp").First(); sp.Children().Length() >= 3 {
		sp.Children().Eq(2).Remove()
	}
	doc.Find("img[data-testid='authorPhoto']").Closest("a").Remove()
	doc.Find("img[data-testid='publicationPhoto']").Closest("a").Remove()
	doc.Find("a[href*='/m/signin']").Remove()
	doc.Find("[data-testid='publicationName']").Closest("div").Remove()
	doc.Find("[data-testid='storyReadTime']").Closest("span").Remove()
	doc.Find("[data-testid='storyPublishDate']").Closest("span").Remove()
	doc.Find("[data-testid='authorName']").Remove()
	doc.Find("[data-testid='headerClapButton']").Remove()
}

func removeEmptyElements(doc *goquery.Document) {
	doc.Find("p").Each(func(_ int, s *goquery.Selection) {
		t := strings.TrimSpace(s.Text())
		if (t == "" || t == "·") && s.Find("img, picture, iframe, video").Length() == 0 {
			s.Remove()
		}
	})
}

func restructureArticle(doc *goquery.Document, sourceURL, authorName, authorLink string) {
	mainFigure := doc.Find("figure.paragraph-image").First()
	titleEl := doc.Find("article h1").First()

	if mainFigure.Length() > 0 && titleEl.Length() > 0 {
		titleEl.AfterSelection(mainFigure)
	}

	anchor := titleEl
	if mainFigure.Length() > 0 {
		anchor = mainFigure
	}
	if anchor.Length() == 0 {
		return
	}

	var b strings.Builder
	if sourceURL != "" {
		b.WriteString(`<p><a href="` + html.EscapeString(sourceURL) + `">Reference</a></p>`)
	}
	if authorLink != "" {
		b.WriteString(`<p>by <a href="` + html.EscapeString(authorLink) + `">` + html.EscapeString(authorName) + `</a></p>`)
	} else {
		b.WriteString(`<p>by ` + html.EscapeString(authorName) + `</p>`)
	}
	anchor.AfterHtml(b.String())
}

// iframeAttrs are the attributes kept on embedded iframes; everything else
// (style, sandbox, lazy-loading markers, classes) is dropped so the embed works
// when the Markdown is rendered.
var iframeAttrs = map[string]bool{
	"src": true, "width": true, "height": true, "title": true,
	"allow": true, "allowfullscreen": true, "frameborder": true,
}

// Default size for iframes without width/height attributes.
const (
	defaultIframeWidth  = "100%"
	defaultIframeHeight = "315"
)

// normalizeIframes fixes lazily loaded embeds (src empty, real URL in
// data-src — e.g. video players on mirror pages) and removes iframes that have
// no URL at all. Iframes without width/height get defaultIframeWidth/Height.
func normalizeIframes(doc *goquery.Document) {
	doc.Find("iframe").Each(func(_ int, f *goquery.Selection) {
		src := strings.TrimSpace(f.AttrOr("src", ""))
		if src == "" || src == "about:blank" {
			src = strings.TrimSpace(f.AttrOr("data-src", f.AttrOr("data-lazy-src", "")))
		}
		if strings.HasPrefix(src, "//") {
			src = "https:" + src
		}
		if !strings.HasPrefix(src, "https://") && !strings.HasPrefix(src, "http://") {
			f.Remove()
			return
		}
		n := f.Get(0)
		kept := []nethtml.Attribute{{Key: "src", Val: src}}
		hasW, hasH := false, false
		for _, a := range n.Attr {
			if !iframeAttrs[a.Key] || a.Key == "src" {
				continue
			}
			if (a.Key == "width" || a.Key == "height") && strings.TrimSpace(a.Val) == "" {
				continue
			}
			hasW = hasW || a.Key == "width"
			hasH = hasH || a.Key == "height"
			kept = append(kept, a)
		}
		// Boyutu olmayan embed'ler tarayıcıda 300×150 görünür; makul bir varsayılan ver.
		if !hasW {
			kept = append(kept, nethtml.Attribute{Key: "width", Val: defaultIframeWidth})
		}
		if !hasH {
			kept = append(kept, nethtml.Attribute{Key: "height", Val: defaultIframeHeight})
		}
		n.Attr = kept
	})
}

// posterParams are query parameters video players use for the poster image.
var posterParams = []string{"poster", "posterUrl", "poster_url", "thumbnail"}

// addVideoFallbacks puts a clickable poster image right after each iframe
// whose URL carries a poster (…?poster=https://…), so the video is reachable
// in Markdown viewers that strip iframes.
func addVideoFallbacks(doc *goquery.Document) {
	doc.Find("iframe").Each(func(_ int, f *goquery.Selection) {
		src := f.AttrOr("src", "")
		poster := videoPoster(src)
		if poster == "" {
			return
		}
		f.AfterHtml(`<p><a href="` + html.EscapeString(src) + `"><img src="` + html.EscapeString(poster) + `" alt="Video"></a></p>`)
	})
}

func videoPoster(src string) string {
	u, err := url.Parse(src)
	if err != nil {
		return ""
	}
	q := u.Query()
	for _, k := range posterParams {
		p := strings.TrimSpace(q.Get(k))
		if pu, err := url.Parse(p); err == nil && (pu.Scheme == "https" || pu.Scheme == "http") && pu.Host != "" {
			return p
		}
	}
	return ""
}

// normalizeFigures replaces each <figure> with a plain <img> (largest srcset
// candidate, caption as alt) or keeps embedded iframes with their caption.
func normalizeFigures(doc *goquery.Document, imgBase string) {
	doc.Find("figure").Each(func(_ int, fig *goquery.Selection) {
		caption := strings.TrimSpace(fig.Find("figcaption").Text())

		if iframe := fig.Find("iframe").First(); iframe.Length() > 0 {
			out, _ := goquery.OuterHtml(iframe)
			if caption != "" {
				out += "<p><em>" + html.EscapeString(caption) + "</em></p>"
			}
			fig.ReplaceWithHtml(out)
			return
		}

		img := fig.Find("img").First()
		src := ""
		if srcset, ok := fig.Find("source[srcset]").First().Attr("srcset"); ok {
			src = lastSrcsetURL(srcset)
		}
		if src == "" {
			src, _ = img.Attr("src")
		}
		src = imageURL(src, imgBase)
		if src == "" {
			if caption == "" {
				fig.Remove()
			} else {
				fig.ReplaceWithHtml("<p><em>" + html.EscapeString(caption) + "</em></p>")
			}
			return
		}

		alt := caption
		if alt == "" {
			alt = imageAlt(img)
		}
		if alt == "" {
			alt = "captionless image"
		}
		fig.ReplaceWithHtml(`<p><img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(alt) + `"></p>`)
	})
}

// normalizeImages handles images outside <figure> (e.g. the mirror's
// <picture><source><img></picture> in the article body).
func normalizeImages(doc *goquery.Document, imgBase string) {
	doc.Find("picture").Each(func(_ int, pic *goquery.Selection) {
		img := pic.Find("img").First()
		if img.Length() == 0 {
			pic.Remove()
			return
		}
		src := ""
		if srcset, ok := pic.Find("source[srcset]").First().Attr("srcset"); ok {
			src = lastSrcsetURL(srcset)
		}
		if src == "" {
			src, _ = img.Attr("src")
		}
		if src == "" {
			pic.Remove()
			return
		}
		alt := imageAlt(img)
		if alt == "" {
			alt = "captionless image"
		}
		pic.ReplaceWithHtml(`<p><img src="` + html.EscapeString(imageURL(src, imgBase)) + `" alt="` + html.EscapeString(alt) + `"></p>`)
	})
	doc.Find("img").Each(func(_ int, img *goquery.Selection) {
		if src, ok := img.Attr("src"); ok && src != "" {
			img.SetAttr("src", imageURL(src, imgBase))
			img.SetAttr("alt", imageAlt(img))
		} else {
			img.Remove()
		}
	})
}

// normalizeCodeBlocks rewrites Medium's <pre><span data-code-block-lang>…<br>…</span></pre>
// into <pre><code class="language-x">…</code></pre> so the converter emits a fenced block.
func normalizeCodeBlocks(doc *goquery.Document) {
	dedupeThemedCode(doc)
	doc.Find("pre").Each(func(_ int, pre *goquery.Selection) {
		lang := codeLang(pre)
		var buf bytes.Buffer
		for _, n := range pre.Nodes {
			textWithBreaks(&buf, n)
		}
		code := strings.Trim(buf.String(), "\n")

		cls := ""
		if lang != "" {
			cls = ` class="language-` + html.EscapeString(lang) + `"`
		}
		pre.ReplaceWithHtml("<pre><code" + cls + ">" + html.EscapeString(code) + "</code></pre>")
	})
}

// dedupeThemedCode removes the second copy of code blocks rendered once per
// colour theme (syntax highlighters on mirror pages: <div class="dark:hidden"><pre
// class="code-theme-light">…</pre></div><div class="hidden dark:block"><pre
// class="code-theme-dark">…</pre></div>). A <pre> is dropped when the
// previous <pre> has the same parent's parent and the same text.
func dedupeThemedCode(doc *goquery.Document) {
	var prev *goquery.Selection
	doc.Find("pre").Each(func(_ int, pre *goquery.Selection) {
		if prev != nil && sameNode(prev.Parent().Parent(), pre.Parent().Parent()) &&
			prev.Text() == pre.Text() {
			wrap := pre.Parent()
			pre.Remove()
			if strings.TrimSpace(wrap.Text()) == "" && wrap.Children().Length() == 0 {
				wrap.Remove()
			}
			return
		}
		prev = pre
	})
}

func sameNode(a, b *goquery.Selection) bool {
	return a.Length() > 0 && b.Length() > 0 && a.Get(0) == b.Get(0)
}

// codeLang returns the language of a code block: Medium's data-code-block-lang,
// or a "language-x" / "lang-x" class on <pre> or its <code> (syntax
// highlighters, mirror pages).
func codeLang(pre *goquery.Selection) string {
	if lang, _ := pre.Find("[data-code-block-lang]").First().Attr("data-code-block-lang"); lang != "" {
		return lang
	}
	if lang, _ := pre.Attr("data-code-block-lang"); lang != "" {
		return lang
	}
	for _, s := range []*goquery.Selection{pre.Find("code").First(), pre} {
		for _, c := range strings.Fields(s.AttrOr("class", "")) {
			for _, prefix := range []string{"language-", "lang-"} {
				if l, ok := strings.CutPrefix(c, prefix); ok && l != "" {
					return l
				}
			}
		}
	}
	return ""
}

func textWithBreaks(buf *bytes.Buffer, n *nethtml.Node) {
	switch n.Type {
	case nethtml.TextNode:
		buf.WriteString(n.Data)
	case nethtml.ElementNode:
		if n.Data == "br" {
			buf.WriteByte('\n')
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		textWithBreaks(buf, c)
	}
}

// ---------------------------------------------------------------- convert

func newConverter() *converter.Converter {
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(
				commonmark.WithHeadingStyle(commonmark.HeadingStyleATX),
				commonmark.WithBulletListMarker("-"),
				commonmark.WithCodeBlockFence("```"),
				commonmark.WithLinkEmptyContentBehavior(commonmark.LinkBehaviorRender),
			),
			strikethrough.NewStrikethroughPlugin(),
			table.NewTablePlugin(),
		),
	)
	// iframe'ler Markdown'a çevrilmez, HTML olarak kalır.
	conv.Register.RendererFor("iframe", converter.TagTypeBlock, func(_ converter.Context, w converter.Writer, n *nethtml.Node) converter.RenderStatus {
		var b bytes.Buffer
		if err := nethtml.Render(&b, n); err != nil {
			return converter.RenderTryNext
		}
		w.WriteString("\n\n")
		w.Write(b.Bytes())
		w.WriteString("\n\n")
		return converter.RenderSuccess
	}, converter.PriorityEarly)
	return conv
}

// ---------------------------------------------------------------- clean markdown

var (
	reLinkOpenNL  = regexp.MustCompile(`\[\n+`)
	reLinkCloseNL = regexp.MustCompile(`\n+\]\(`)
)

func cleanMarkdown(md string) string {
	md = reLinkOpenNL.ReplaceAllString(md, "[")
	md = reLinkCloseNL.ReplaceAllString(md, "](")
	md = strings.ReplaceAll(md, "[](", "[nameless link](")
	return md
}

func filterUndesiredLines(md string) string {
	lines := strings.Split(md, "\n")
	out := lines[:0]
	for i, l := range lines {
		if i < linesToCheckForRemoval && undesiredLines[strings.TrimSpace(l)] {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------- helpers

const (
	miroPrefix    = "https://miro.medium.com/v2/resize:fit:"
	imageFitWidth = "1400" // Medium'un srcset'teki en büyük adayı
)

// reMirrorImg matches the mirror's Medium image proxy path, relative or
// absolute on any host, in both forms seen so far:
// /img/medium/700/1*abc.png, /img/700/0*abc.jpg, https://<mirror>/img/4000/1*abc.png
// Proxies for other sites (/img/<source>/…) do not match: the segment after
// /img/ (or /img/medium/) must be a width.
var reMirrorImg = regexp.MustCompile(`^(?:https?://[^/]+)?/img/(?:medium/)?\d+/(.+)$`)

// imageURL makes an image src absolute. Medium proxy paths on a mirror are
// rewritten to the miro.medium.com URL they redirect to, so Medium images do
// not depend on the (frequently changing) mirror domain. Other root-relative
// paths are resolved against base: medium.com on Medium pages, the mirror's own
// origin on mirror pages (e.g. /img/<source>/… proxies for non-Medium sites).
func imageURL(src, base string) string {
	src = strings.TrimSpace(src)
	if m := reMirrorImg.FindStringSubmatch(src); m != nil {
		return miroPrefix + imageFitWidth + "/" + m[1]
	}
	switch {
	case strings.HasPrefix(src, "//"):
		return "https:" + src
	case strings.HasPrefix(src, "/"):
		return strings.TrimRight(base, "/") + src
	}
	return src
}

// origin returns scheme://host of an absolute http(s) URL, or "".
func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// placeholderAlts are alt texts that do not describe the image (the mirror
// fills "None" — a stringified Python None — and "Post cover image").
var placeholderAlts = map[string]bool{"none": true, "null": true, "post cover image": true}

// imageAlt returns a meaningful alt text: the img's alt, or — when that is
// empty or a placeholder — its data-caption (mirror pages keep body image
// captions there instead of a <figcaption>).
func imageAlt(img *goquery.Selection) string {
	alt := strings.TrimSpace(img.AttrOr("alt", ""))
	if alt != "" && !placeholderAlts[strings.ToLower(alt)] {
		return alt
	}
	return captionText(img.AttrOr("data-caption", ""))
}

var reBracketed = regexp.MustCompile(`\s*\[[^\[\]]*\]`)

// captionText turns an HTML caption into plain alt text: tags are dropped
// (link text is kept) and bracketed parts are removed, e.g.
// "Event Sourcing [<a>View Source</a>]" → "Event Sourcing" — brackets would
// clash with Markdown image syntax.
func captionText(caption string) string {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return ""
	}
	frag, err := goquery.NewDocumentFromReader(strings.NewReader("<div>" + caption + "</div>"))
	if err != nil {
		return ""
	}
	frag.Find("svg, script, style").Remove()
	t := reBracketed.ReplaceAllString(frag.Text(), "")
	t = strings.Join(strings.Fields(t), " ")
	if placeholderAlts[strings.ToLower(t)] {
		return ""
	}
	return t
}

func absMedium(u string) string {
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") {
		return mediumURLPrefix + u
	}
	return u
}

func lastSrcsetURL(srcset string) string {
	parts := strings.Split(srcset, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if f := strings.Fields(parts[i]); len(f) > 0 {
			return f[0]
		}
	}
	return ""
}
