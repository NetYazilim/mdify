package medium

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Medium mirror/reader pages are recognised by their
// structure, not by their domain, so a mirror moving to a new host needs no
// code change. The layout handled here:
//
//	<article>
//	  <div data-nosnippet><nav>… <a aria-label="Open original article" href="ORIGINAL"></nav></div>
//	  <img src="/img/medium/700/ID" data-zoom-src="/img/medium/4000/ID" alt="Post cover image">
//	      (newer pages: /img/700/ID, /img/4000/ID)
//	      (or <figure><img …><figcaption>…</figcaption></figure> when the image has a caption)
//	  <header><p>date</p><h1>title</h1><p>subtitle</p><div><img avatar><p>By NAME</p><p>N min read</p></div></header>
//	  <section aria-labelledby="toc-heading">…</section>
//	  <div><div class="prose">… <picture><source media><img src="/img/medium/700/ID" alt="None" data-caption="…"></picture> …
//	        <div class="relative"><button data-code="…"><div class="dark:hidden"><pre class="code-theme-light">…</pre></div>
//	        <div class="hidden dark:block"><pre class="code-theme-dark">…</pre></div></div> …</div></div>
//	  <div data-nosnippet><nav>…</nav></div>
//	</article>
//
// The first image of the Medium article is lifted out of the body and shown
// above the header as the cover; it is not repeated in the body.

const mirrorOriginalSel = "a[aria-label='Open original article']"

func isMirrorPage(doc *goquery.Document) bool {
	if doc.Find("article "+mirrorOriginalSel).Length() > 0 {
		return true
	}
	found := false
	doc.Find("article img[src^='/img/']").EachWithBreak(func(_ int, img *goquery.Selection) bool {
		found = reMirrorImg.MatchString(img.AttrOr("src", ""))
		return !found
	})
	return found
}

// embeddedURL returns the article URL embedded in a mirror URL
// (https://<mirror>/https://medium.com/@user/post → https://medium.com/@user/post),
// or "" if raw has no embedded https URL.
func embeddedURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	inner, ok := strings.CutPrefix(u.EscapedPath(), "/")
	if !ok || !strings.HasPrefix(inner, "https://") {
		return ""
	}
	if u.RawQuery != "" {
		inner += "?" + u.RawQuery
	}
	iu, err := url.Parse(inner)
	if err != nil || iu.Host == "" {
		return ""
	}
	return iu.String()
}

// prepareMirror rewrites a mirror page into the shape the Medium pipeline
// expects: the cover becomes figure.paragraph-image (restructureArticle then
// puts it right after the title, as on Medium) and the mirror's own UI is removed.
// It returns the author name and the original article URL (both may be empty).
func prepareMirror(doc *goquery.Document) (author, original string) {
	art := doc.Find("article").First()

	if href, ok := art.Find(mirrorOriginalSel).First().Attr("href"); ok {
		if u, err := url.Parse(href); err == nil && u.Scheme == "https" && u.Host != "" {
			original = href
		}
	}

	header := art.ChildrenFiltered("header").First()
	header.Find("p").EachWithBreak(func(_ int, p *goquery.Selection) bool {
		if name, ok := strings.CutPrefix(strings.TrimSpace(p.Text()), "By "); ok {
			author = strings.TrimSpace(name)
			return false
		}
		return true
	})
	if author == "" {
		author = defaultAuthor
	}

	// Kapak: article'ın doğrudan çocuğu olan img ya da figure.
	cover := art.ChildrenFiltered("figure, img").First()
	if cover.Length() > 0 {
		if goquery.NodeName(cover) == "img" {
			cover.WrapHtml(`<figure></figure>`)
			cover = cover.Parent()
		}
		cover.AddClass("paragraph-image")
		if h1 := header.Find("h1").First(); h1.Length() > 0 {
			h1.AfterSelection(cover)
		}
	}

	// Header'da sadece başlık, kapak ve alt başlık kalsın: tarih, avatar, yazar, okuma süresi gitsin.
	if h1 := header.Find("h1").First(); h1.Length() > 0 {
		h1.PrevAll().Remove()
	}
	header.Find("img").Not("figure img").Each(func(_ int, img *goquery.Selection) {
		// avatar + "By …" + "N min read" bloğu
		if block := img.ParentsUntil("header").Last(); block.Length() > 0 {
			block.Remove()
		} else {
			img.Remove()
		}
	})

	art.Find("[data-nosnippet], nav, section[aria-labelledby='toc-heading'], button").Remove()
	return author, original
}

// Second mirror layout: plain HTML, images already on
// miro.medium.com, the original URL in a header link outside <article>.
//
//	<div class="bar">… <a class="source" href="ORIGINAL">Original on Medium →</a></div>
//	<article>
//	  <h1>title</h1>
//	  <p class="meta">AUTHOR · Oct 10, 2026 · 8 min read</p>
//	  <p class="print-source">ORIGINAL</p>
//	  <div class="body">… <figure><img src="https://miro.medium.com/…"></figure> … <pre><code class="language-python">…</code></pre> …</div>
//	</article>

func isMetaMirrorPage(doc *goquery.Document) bool {
	art := doc.Find("article").First()
	return art.ChildrenFiltered("p.meta").Length() > 0 &&
		art.ChildrenFiltered("p.print-source, div.body").Length() > 0
}

// prepareMetaMirror removes the meta/print-source lines and returns the author
// (first " · " part of p.meta) and the original article URL (may be empty).
func prepareMetaMirror(doc *goquery.Document) (author, original string) {
	art := doc.Find("article").First()

	meta := art.ChildrenFiltered("p.meta").First()
	if name, _, _ := strings.Cut(strings.TrimSpace(meta.Text()), "·"); !strings.Contains(name, "min read") {
		author = strings.TrimSpace(name)
	}
	if author == "" {
		author = defaultAuthor
	}

	for _, sel := range []string{"a.source", "article > p.print-source"} {
		el := doc.Find(sel).First()
		cand := strings.TrimSpace(el.AttrOr("href", el.Text()))
		if u, err := url.Parse(cand); err == nil && u.Scheme == "https" && u.Host != "" {
			original = cand
			break
		}
	}

	art.ChildrenFiltered("p.meta, p.print-source").Remove()
	return author, original
}
