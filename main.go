// mdify — Medium makalelerini Markdown'a çeviren CLI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/NetYazilim/mdify/internal/medium"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mdify:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		outFile  = flag.String("o", "", "çıktı dosyası (varsayılan: stdout)")
		autoName = flag.Bool("O", false, "çıktıyı başlıktan türetilen <slug>.md dosyasına yaz")
		outDir   = flag.String("d", ".", "-O ile kullanılacak klasör")
		htmlFile = flag.String("html", "", "URL yerine kayıtlı HTML dosyasından çevir (\"-\" = stdin)")
		_        = flag.Bool("any-host", false, "artık gerekmiyor, her https adresi kabul ediliyor (eski komutlarla uyumluluk için duruyor)")
		timeout  = flag.Duration("timeout", 15*time.Second, "istek başına zaman aşımı")
		retries  = flag.Int("retries", 3, "toplam deneme sayısı")
		verbose  = flag.Bool("v", false, "ayrıntılı log (stderr)")
		showVer  = flag.Bool("version", false, "sürümü yazdır")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Kullanım:
  mdify [seçenekler] <url>
  mdify [seçenekler] -html sayfa.html [<kaynak-url>]

Örnekler:
  mdify https://medium.com/@user/some-article-123abc > yazi.md
  mdify -O -d notlar https://medium.com/@user/some-article-123abc
  mdify -html kayitli.html https://medium.com/@user/some-article-123abc

Seçenekler:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Println("mdify", version)
		return nil
	}

	logf := func(format string, args ...any) {
		if *verbose {
			fmt.Fprintf(os.Stderr, "mdify: "+format+"\n", args...)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	sourceURL := flag.Arg(0)
	var page string

	switch {
	case *htmlFile != "":
		b, err := readInput(*htmlFile)
		if err != nil {
			return err
		}
		page = string(b)
	case sourceURL == "":
		flag.Usage()
		return errors.New("URL gerekli")
	default:
		if err := medium.ValidateURL(sourceURL); err != nil {
			return err
		}
		logf("indiriliyor: %s", sourceURL)
		var err error
		page, err = medium.FetchHTML(ctx, sourceURL, medium.FetchOptions{
			Timeout:    *timeout,
			MaxRetries: *retries,
			Logf:       logf,
		})
		if err != nil {
			return err
		}
	}

	res, err := medium.Convert(page, sourceURL)
	if err != nil {
		return err
	}
	logf("başlık: %q, yazar: %q", res.Title, res.Author)

	target := *outFile
	if target == "" && *autoName {
		target = filepath.Join(*outDir, slugify(res.Title)+".md")
	}
	if target == "" {
		_, err = io.WriteString(os.Stdout, res.Markdown)
		return err
	}
	if dir := filepath.Dir(target); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(target, []byte(res.Markdown), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "yazıldı:", target)
	return nil
}

func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

var (
	trSlug    = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u", "Ç", "c", "Ğ", "g", "İ", "i", "Ö", "o", "Ş", "s", "Ü", "u")
	nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)
)

func slugify(s string) string {
	s = strings.ToLower(trSlug.Replace(s))
	s = strings.Trim(nonSlugRe.ReplaceAllString(s, "-"), "-")
	if len(s) > 80 {
		s = strings.TrimRight(s[:80], "-")
	}
	if s == "" {
		s = "article"
	}
	return s
}
