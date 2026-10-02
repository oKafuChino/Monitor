package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const JSONBodyLimit int64 = 1 << 20
var ErrBodyTooLarge = errors.New("request exceeds size limit")

// ReadBounded checks the extra byte before returning any data to the decoder.
func ReadBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil { return nil, err }
	if int64(len(b)) > limit { return nil, ErrBodyTooLarge }
	return b, nil
}

// RequestLimits must precede identity and all handlers, including unknown routes.
func RequestLimits() gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := JSONBodyLimit
		path := c.Request.URL.Path
		switch {
		case path == "/api/login": limit = 16 << 10
		case path == "/api/admin/upload/chunk" || path == "/api/install/upload/chunk": limit = 6 << 20 // 5 MiB + multipart overhead
		case path == "/api/admin/ui/settings": limit = 2 << 20
		case path == "/api/admin/update/favicon": limit = 5 << 20
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request exceeds size limit"})
			return
		}
		if c.Request.Body != nil { c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit) }
		c.Next()
	}
}
