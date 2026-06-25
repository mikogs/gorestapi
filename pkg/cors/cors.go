package cors

import (
	"fmt"
	"net/http"
)

type CORS struct {
	AllowOrigin      string
	AllowHeaders     string
	AllowMethods     string
	AllowCredentials bool
	MaxAge           int
}

func (c *CORS) WriteHeaders(w http.ResponseWriter) {
	if c.AllowOrigin != "" {
		w.Header().Add("Access-Control-Allow-Origin", c.AllowOrigin)
	}

	if c.AllowHeaders != "" {
		w.Header().Add("Access-Control-Allow-Headers", c.AllowHeaders)
	}

	if c.AllowMethods != "" {
		w.Header().Add("Access-Control-Allow-Methods", c.AllowMethods)
	}

	if c.AllowCredentials {
		w.Header().Add("Access-Control-Allow-Credentials", "true")
	}

	if c.MaxAge > 0 {
		w.Header().Add("Access-Control-Max-Age", fmt.Sprintf("%d", c.MaxAge))
	}
}
