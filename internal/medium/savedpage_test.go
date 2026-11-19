package medium

import (
	"os"
	"strings"
	"testing"
)

func TestSavedMirrorPage(t *testing.T) {
	b, err := os.ReadFile("testdata/saved_mirror.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"https://uxplanet.org/mcp-is-dead-cf16b667ba6d", ""} {
		res, err := Convert(string(b), src)
		if err != nil {
			t.Fatal(err)
		}
		md := res.Markdown
		if strings.Contains(md, "_files/") {
			t.Errorf("src=%q: yerel resim yolu kaldı", src)
		}
		for _, w := range []string{
			"[Reference](https://uxplanet.org/mcp-is-dead-cf16b667ba6d)",
			"(https://miro.medium.com/",
			"![MCP architecture.](https://miro.medium.com/",
			"[**Claude Code + Figma = 💛**](https://uxplanet.org/claude-code-figma-f647facbe181)",
		} {
			if !strings.Contains(md, w) {
				t.Errorf("src=%q: çıktıda yok: %q", src, w)
			}
		}
		if t.Failed() {
			t.Log(md)
			return
		}
	}
}

func TestSavedFromURL(t *testing.T) {
	page := "<!DOCTYPE html>\n<!-- saved from url=(0073)https://freedium-mirror.cfd/https://uxplanet.org/mcp-is-dead-cf16b667ba6d -->\n<html>"
	if got := savedFromURL(page); got != "https://freedium-mirror.cfd/https://uxplanet.org/mcp-is-dead-cf16b667ba6d" {
		t.Fatalf("got %q", got)
	}
	cases := []struct{ src, want string }{
		{"", "https://freedium-mirror.cfd/https://uxplanet.org/mcp-is-dead-cf16b667ba6d"},
		{"https://uxplanet.org/mcp-is-dead-cf16b667ba6d", "https://freedium-mirror.cfd/https://uxplanet.org/mcp-is-dead-cf16b667ba6d"},
		{"https://medium.com/@x/other", "https://medium.com/@x/other"},
	}
	for _, c := range cases {
		if got := resolveSourceURL(page, c.src); got != c.want {
			t.Errorf("resolveSourceURL(%q) = %q", c.src, got)
		}
	}
}

func TestIsLocalPath(t *testing.T) {
	for src, want := range map[string]bool{
		"./MCP is Dead - Freedium_files/1_a.png": true,
		"MCP_files/1_a.png":                      true,
		"/img/4000/1*a.png":                      false,
		"https://miro.medium.com/x.png":          false,
		"//cdn.example.com/x.png":                false,
		"data:image/png;base64,xx":               false,
		"":                                       false,
	} {
		if got := isLocalPath(src); got != want {
			t.Errorf("isLocalPath(%q) = %v", src, got)
		}
	}
}

func TestReLiteralBold(t *testing.T) {
	for in, want := range map[string]string{
		"**Claude Code + Figma = 💛\n**": "**Claude Code + Figma = 💛**",
		"**a** text **b**":              "**a** text **b**",
		"x **ok **, y":                  "x **ok**, y",
	} {
		if got := reLiteralBold.ReplaceAllString(in, "$1**$2**"); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
