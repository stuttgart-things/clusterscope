package serve

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	// Under BASE_PATH the cluster page lives under the prefix too.
	targetURL := fmt.Sprintf("%s://%s%s/clusters/%s", scheme, r.Host, s.opts.BasePath, name)

	// Container-safe Chrome flags:
	//
	//	--no-sandbox             required without user namespaces (k8s root pod)
	//	--disable-dev-shm-usage  use /tmp instead of /dev/shm (avoids 64 MB shm limit)
	//	--disable-gpu            no GPU in headless containers
	//	--disable-crash-reporter prevents crashpad from starting (needs writable --database)
	//	--no-first-run           skip first-run setup that writes to the profile dir
	//	--disable-breakpad       disable Breakpad crash handler as well
	//	--user-data-dir          explicit writable dir for Chrome profile (nobody user)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("disable-crash-reporter", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.UserDataDir("/tmp/chrome-clusterscope"),
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

	err := chromedp.Do(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible(chromedp.CSS(`svg#graph g`)),
	)
	var pdfBuf []byte
	if err == nil {
		pdfBuf, err = chromedp.Run(ctx, chromedp.PrintToPDF(
			chromedp.PDFPrintBackground(),
			chromedp.PDFLandscape(),
			chromedp.PDFPaper(chromedp.PaperSize{Width: 11.69, Height: 8.27}),
			chromedp.PDFMargin(0.4),
			chromedp.PDFScale(0.85),
		))
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("PDF generation failed: %v", err), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("clusterscope-%s.pdf", name)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBuf)))
	http.ServeContent(w, r, filename, time.Now(), bytes.NewReader(pdfBuf))
}
