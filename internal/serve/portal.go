package serve

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
)

// Behind sthings-portal (stuttgart-things/sthings-portal#23) the dashboard
// runs under a prefix and inside the portal's frame. writePage adds what that
// needs to every page the server hands out, so the templates — which the CLI
// also renders into standalone files — only use relative URLs:
//
//   - <base href="<BasePath>/">: relative URLs (fetch, iframe src, links,
//     pushState) resolve under the prefix wherever the page is shown;
//   - the hr-embedded class, set before the first paint when the page is
//     framed; the shell turns its header into one compact line then;
//   - the portal's design tokens when the request came through the portal
//     (X-Forwarded-Prefix); the shell builds its colours on them.

// ParseBasePath checks BASE_PATH: "" (or "/") means no prefix, otherwise it
// is "/" plus path segments, without a trailing slash.
func ParseBasePath(v string) (string, error) {
	v = strings.TrimSuffix(strings.TrimSpace(v), "/")
	if v == "" {
		return "", nil
	}
	if !strings.HasPrefix(v, "/") {
		return "", fmt.Errorf("base path %q must start with /", v)
	}
	for _, seg := range strings.Split(v[1:], "/") {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, "?#%{}\\ \t\"'<>") {
			return "", fmt.Errorf("base path %q is not a clean path", v)
		}
	}
	return v, nil
}

// pageHead is inserted right after <head>.
func (s *Server) pageHead(r *http.Request) string {
	head := `<base href="` + html.EscapeString(s.opts.BasePath) + `/">` +
		`<script>if (window.self !== window.top) document.documentElement.classList.add('hr-embedded')</script>`
	if r.Header.Get("X-Forwarded-Prefix") != "" {
		head += `<link rel="stylesheet" href="/static/homerun2-tokens.css">`
	}
	return head
}

// writePage renders a page into a buffer, adds pageHead, and writes it; a
// render error is a clean 500 rather than half a page.
func (s *Server) writePage(w http.ResponseWriter, r *http.Request, write func(io.Writer) error) {
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := buf.String()
	if i := strings.Index(page, "<head>"); i >= 0 {
		i += len("<head>")
		page = page[:i] + s.pageHead(r) + page[i:]
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, page)
}
