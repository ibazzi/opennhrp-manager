package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ServeSPA serves embedded frontend dist files and falls back to index.html for SPA routing
func ServeSPA(r *gin.Engine, webAssets embed.FS) {
	distFS, err := fs.Sub(webAssets, "frontend/dist")
	if err != nil {
		return
	}

	httpFS := http.FS(distFS)

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Never fallback API paths to index.html
		if strings.HasPrefix(path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API route not found"})
			return
		}

		// Try opening static file
		f, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			_ = f.Close()
			c.Header("Cache-Control", staticCacheControl(path))
			c.FileFromFS(path, httpFS)
			return
		}

		// Fallback to index.html
		c.Header("Cache-Control", "no-cache")
		c.FileFromFS("/", httpFS)
	})
}

func staticCacheControl(path string) string {
	if strings.HasPrefix(path, "/assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}
