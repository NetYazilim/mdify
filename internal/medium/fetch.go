package medium

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	userAgent          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	maxBodyBytes       = 5 << 20 // 5 MiB
	maxRedirects       = 5
	maxRateLimitWait   = 10 * time.Second
	maxBackoff         = 30 * time.Second
	initialBackoffBase = 2 * time.Second
)

// FetchOptions controls how an article page is downloaded.
type FetchOptions struct {
	Timeout    time.Duration // per-request timeout
	MaxRetries int           // total attempts (>=1)
	Logf       func(format string, args ...any)
}

// HTTPStatusError is returned for non-2xx responses.
type HTTPStatusError struct {
	Status int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("makale alınamadı: %d %s", e.Status, http.StatusText(e.Status))
}

func newClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("çok fazla yönlendirme")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("https olmayan yönlendirme reddedildi: %s", req.URL)
			}
			return nil
		},
	}
}

// FetchHTML downloads the page with retries: 429 honours Retry-After (capped),
// 5xx and network errors use exponential backoff, other 4xx fail immediately.
func FetchHTML(ctx context.Context, rawURL string, opt FetchOptions) (string, error) {
	if opt.MaxRetries < 1 {
		opt.MaxRetries = 3
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 15 * time.Second
	}
	logf := opt.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	client := newClient(opt.Timeout)

	var lastErr error
	for attempt := 0; attempt < opt.MaxRetries; attempt++ {
		last := attempt == opt.MaxRetries-1

		body, status, retryAfter, err := doRequest(ctx, client, rawURL)
		switch {
		case err != nil:
			lastErr = err
		case status == http.StatusTooManyRequests:
			lastErr = fmt.Errorf("Medium istekleri sınırlıyor; %d sn sonra tekrar deneyin", retryAfter)
			if !last {
				wait := min(time.Duration(retryAfter)*time.Second, maxRateLimitWait)
				logf("429 alındı, %s bekleniyor (%d/%d)", wait, attempt+1, opt.MaxRetries)
				if err := sleep(ctx, wait); err != nil {
					return "", err
				}
				continue
			}
			return "", lastErr
		case status >= 400:
			lastErr = &HTTPStatusError{Status: status}
			if status < 500 {
				return "", lastErr // 4xx tekrar denemekle düzelmez
			}
		default:
			return body, nil
		}

		logf("deneme %d/%d başarısız: %v", attempt+1, opt.MaxRetries, lastErr)
		if !last {
			delay := min(initialBackoffBase<<attempt, maxBackoff)
			if err := sleep(ctx, delay); err != nil {
				return "", err
			}
		}
	}
	return "", lastErr
}

func doRequest(ctx context.Context, client *http.Client, rawURL string) (body string, status, retryAfter int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, 0, err
	}
	// Accept-Encoding bilinçli olarak set edilmiyor: Go transport'u gzip'i
	// kendisi istiyor ve şeffaf biçimde açıyor.
	h := req.Header
	h.Set("User-Agent", userAgent)
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	h.Set("Accept-Language", "en-US,en;q=0.9")
	h.Set("Upgrade-Insecure-Requests", "1")
	h.Set("Sec-Fetch-Dest", "document")
	h.Set("Sec-Fetch-Mode", "navigate")
	h.Set("Sec-Fetch-Site", "none")
	h.Set("Sec-Fetch-User", "?1")
	h.Set("Referer", "https://medium.com/")
	h.Set("Cache-Control", "max-age=0")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if ra <= 0 {
			ra = 60
		}
		return "", resp.StatusCode, ra, nil
	}
	if resp.StatusCode >= 400 {
		return "", resp.StatusCode, 0, nil
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return "", 0, 0, err
	}
	if len(b) > maxBodyBytes {
		return "", 0, 0, fmt.Errorf("yanıt çok büyük (> %d MiB)", maxBodyBytes>>20)
	}
	return string(b), resp.StatusCode, 0, nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
