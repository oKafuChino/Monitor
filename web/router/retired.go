package router

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

func rejectRetiredCapabilities(c *gin.Context) {
	p := c.Request.URL.Path
	retired := p == "/api/clients/terminal" || (strings.HasPrefix(p, "/api/admin/client/") && strings.HasSuffix(p, "/terminal"))
	for _, prefix := range []string{"/api/admin/task", "/api/admin/exec", "/api/clients/task", "/api/admin/theme", "/api/admin/plugin", "/api/plugin", "/themes"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			retired = true
		}
	}
	if retired {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"status": "error", "message": "This capability has been removed"})
		return
	}
	c.Next()
}
