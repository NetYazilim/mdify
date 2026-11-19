package medium

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	nethtml "golang.org/x/net/html"
)

// Pages saved from a browser ("Save page as… / Web page, HTML only" or
// "complete") differ from the live page in two ways handled here:
//
//   - The browser writes a "<!-- saved from url=(NNNN)https://… -->" comment
//     near the top that names the page's real URL.
//   - <img src> is rewritten to a local file next to the HTML
//     ("./Title_files/1_abc.png"), while the original path often survives in
//     data-zoom-src / data-src.

var reSavedFrom = regexp.MustCompile(`<!--\s*saved from url=\(\d+\)(\S+?)\s*-->`)

// savedFromURL returns the URL from the browser's "saved from" comment, or "".
func savedFromURL(page string) string {
	head := page
	if len(head) > 4096 {
		head = head[:4096]
	}
	m := reSavedFrom.FindStringSubmatch(head)
	if m == nil || !strings.HasPrefix(m[1], "http") {
		return ""
	}
	return m[1]
}

// resolveSourceURL picks the URL used for mirror detection and image
// resolution. A saved page's own URL wins when the caller gave none, or when
// the caller gave the original article that the saved (mirror) URL embeds.
func resolveSourceURL(page, sourceURL string) string {
	saved := savedFromURL(page)
	if saved == "" {
		return sourceURL
	}
	if sourceURL == "" || sameURL(embeddedURL(saved), sourceURL) {
		return saved
	}
	return sourceURL
}

func sameURL(a, b string) bool {
	norm := func(s string) string { return strings.TrimRight(strings.TrimSpace(s), "/") }
	return a != "" && norm(a) == norm(b)
}

// isLocalPath reports whether src points to a file next to a saved page
// rather than to a URL (relative path without scheme or leading slash).
func isLocalPath(src string) bool {
	src = strings.TrimSpace(src)
	if src == "" || strings.HasPrefix(src, "/") || strings.HasPrefix(src, "#") {
		return false
	}
	if i := strings.Index(src, ":"); i > 0 && !strings.ContainsAny(src[:i], "/.") {
		return false // has a scheme: https:, data:, blob:, …
	}
	return true
}

// savedImgAttrs are attributes that keep the image's original path on saved pages.
var savedImgAttrs = []string{"data-zoom-src", "data-src", "data-original", "data-lazy-src"}

// restoreSavedImages swaps local file paths in <img src> for the original
// path kept in a data-* attribute. Local <source srcset> entries, which a
// renderer cannot load either, are dropped so the <img> is used instead.
func restoreSavedImages(doc *goquery.Document) {
	doc.Find("img").Each(func(_ int, img *goquery.Selection) {
		if !isLocalPath(img.AttrOr("src", "")) {
			return
		}
		for _, a := range savedImgAttrs {
			if v := strings.TrimSpace(img.AttrOr(a, "")); v != "" && !isLocalPath(v) {
				img.SetAttr("src", v)
				return
			}
		}
	})
	doc.Find("source[srcset]").Each(func(_ int, s *goquery.Selection) {
		if isLocalPath(lastSrcsetURL(s.AttrOr("srcset", ""))) {
			s.RemoveAttr("srcset")
		}
	})
}

// trimEmphasisSpace moves whitespace at the start/end of <strong>/<b>/<em>/<i>
// outside the element: "**💛 **" is not bold in CommonMark, "**💛** " is.
func trimEmphasisSpace(doc *goquery.Document) {
	doc.Find("strong, b, em, i").Each(func(_ int, s *goquery.Selection) {
		n := s.Get(0)
		if n.Parent == nil {
			return
		}
		if c := n.LastChild; c != nil && c.Type == nethtml.TextNode {
			if t := strings.TrimRightFunc(c.Data, unicode.IsSpace); t != c.Data {
				c.Data = t
				n.Parent.InsertBefore(&nethtml.Node{Type: nethtml.TextNode, Data: " "}, n.NextSibling)
			}
		}
		if c := n.FirstChild; c != nil && c.Type == nethtml.TextNode {
			if t := strings.TrimLeftFunc(c.Data, unicode.IsSpace); t != c.Data {
				c.Data = t
				n.Parent.InsertBefore(&nethtml.Node{Type: nethtml.TextNode, Data: " "}, n)
			}
		}
	})
}

// reLiteralBold matches Markdown bold written as literal text whose closing
// "**" is preceded by whitespace ("**Title\n**", seen on mirror link cards).
var reLiteralBold = regexp.MustCompile(`(^|\s)\*\*(\S[^*]*?)\s+\*\*`)

// fixLiteralBold rewrites such text nodes to "**Title**" (outside pre/code),
// since CommonMark does not treat "**Title **" as bold.
func fixLiteralBold(doc *goquery.Document) {
	var walk func(n *nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && (n.Data == "pre" || n.Data == "code") {
			return
		}
		if n.Type == nethtml.TextNode && strings.Contains(n.Data, "**") {
			n.Data = reLiteralBold.ReplaceAllString(n.Data, "$1**$2**")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range doc.Find("article").Nodes {
		walk(n)
	}
}
