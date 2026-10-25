package gorestapi

import (
	"net/http"
	"strconv"
)

// writeSecurityHeaders writes the headers that keep browsers from using the responses in other ways than as data of a
// client: as a page, in a frame, as a script of another type, or from a cache, see Options.DisableSecurityHeaders.
func (h *Handler) writeSecurityHeaders(w http.ResponseWriter) {
	if h.options.DisableSecurityHeaders {
		return
	}
	header := w.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	cacheControl := h.options.CacheControl
	if cacheControl == "" {
		cacheControl = "no-store"
	}
	header.Set("Cache-Control", cacheControl)
	if seconds := int64(h.options.HSTSMaxAge.Seconds()); seconds > 0 {
		header.Set("Strict-Transport-Security", "max-age="+strconv.FormatInt(seconds, 10)+"; includeSubDomains")
	}
}
