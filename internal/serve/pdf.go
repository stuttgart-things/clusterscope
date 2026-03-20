package serve

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// handlePDF renders the cluster detail page headlessly via Chromium and returns
// a PDF with full background colors in landscape A4 format.
//
// Works both locally and inside Kubernetes containers (no-sandbox mode).
//
// GET /pdf/<clusterName>
func (s *Server) handlePDF(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/pdf/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		http.Error(w, "cluster name required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	_, ok := s.cache[name]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	targetURL := fmt.Sprintf("%s://%s/clusters/%s", scheme, r.Host, name)

	// Container-safe Chrome flags:
	//
	//	--no-sandbox             required without user namespaces (k8s root pod)
	//	--disable-dev-shm-usage  use /tmp instead of /dev/shm (avoids 64 MB shm limit)
	//	--disable-gpu            no GPU in headless containers
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Headless,
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(string, ...interface{}) {}),
	)
	defer cancel()

	ctx, cancel2 := context.WithTimeout(ctx, 30*time.Second)
	defer cancel2()

	var pdfBuf []byte
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible(`svg#graph g`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithLandscape(true).
				WithPaperWidth(11.69).
				WithPaperHeight(8.27).
				WithMarginTop(0.4).
				WithMarginBottom(0.4).
				WithMarginLeft(0.4).
				WithMarginRight(0.4).
				WithScale(0.85).
				Do(ctx)
			return err
		}),
	); err != nil {
		http.Error(w, fmt.Sprintf("PDF generation failed: %v", err), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("clusterscope-%s.pdf", name)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBuf)))
	http.ServeContent(w, r, filename, time.Now(), bytes.NewReader(pdfBuf))
}
