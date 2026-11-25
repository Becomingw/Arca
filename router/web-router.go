package router

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")
	compress := gzip.Gzip(gzip.DefaultCompression)
	arcaIndex, _ := fs.ReadFile(assets.BuildFS, "web/dist/arca/index.html")

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		func(c *gin.Context) {
			// ServeFrontendFiles sends build files already compressed.
			if !frontendFS.Exists("/", c.Request.URL.Path) {
				compress(c)
			}
		},
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(frontendFS),
		middleware.Cache(),
		func(c *gin.Context) {
			path := c.Request.URL.Path
			if len(arcaIndex) > 0 && (path == "/" || path == "/arca" || path == "/arca/" || path == "/arca/index.html") &&
				(c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
				c.Header("Cache-Control", "no-cache")
				c.Data(http.StatusOK, "text/html; charset=utf-8", arcaIndex)
				c.Abort()
			}
		},
		middleware.ServeFrontendFiles(frontendFS),
		func(c *gin.Context) {
			if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
				controller.RelayNotFound(c)
				return
			}
			c.Header("Cache-Control", "no-cache")
			if len(arcaIndex) > 0 && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
				path := c.Request.URL.Path
				if strings.HasPrefix(path, "/arca/assets/") {
					c.Status(http.StatusNotFound)
					return
				}
				if path == "/arca" || strings.HasPrefix(path, "/arca/") {
					c.Data(http.StatusOK, "text/html; charset=utf-8", arcaIndex)
					return
				}
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.IndexPage)
		},
	)
}
