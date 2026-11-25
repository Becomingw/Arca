package controller

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/arcaopencode"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

const arcaOpenCodeModelLimit = 512

type arcaOpenCodeModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func GetArcaOpenCodeConfig(c *gin.Context) {
	baseURL := arcaOpenCodeBaseURL(system_setting.ServerAddress)
	if baseURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "gateway_address_unavailable"}})
		return
	}
	groups, err := getModelListGroups(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "catalog_unavailable"}})
		return
	}
	names, err := model.ArcaOpenCodeChatModels(c.Request.Context(), groups.ownerGroups)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "catalog_unavailable"}})
		return
	}
	names, err = filterRelayModelNames(c, names)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "catalog_unavailable"}})
		return
	}
	models := make(map[string]arcaOpenCodeModel)
	for _, name := range names {
		// OpenCode expands certain tokens in configuration strings. Catalog
		// names must not opt a client into reading its environment or files.
		if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "{}") ||
			strings.IndexFunc(name, unicode.IsControl) >= 0 {
			continue
		}
		models[name] = arcaOpenCodeModel{ID: name, Name: name}
		if len(models) > arcaOpenCodeModelLimit {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "catalog_unavailable"}})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"$schema": "https://opencode.ai/config.json",
		"provider": gin.H{
			"aether": gin.H{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    "Arca",
				"options": gin.H{"baseURL": baseURL},
				"models":  models,
			},
		},
	})
}

// arcaOpenCodeBaseURL resolves the operator-configured address, independently
// of the caller's Host and forwarded headers.
func arcaOpenCodeBaseURL(address string) string {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || strings.ContainsAny(address, "{}") {
		return ""
	}
	host := parsed.Hostname()
	loopback := strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return ""
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path += "/v1"
	}
	parsed.RawPath = ""
	return parsed.String()
}

// DownloadArcaOpenCodeSync serves the credential-free Linux/macOS setup script.
func DownloadArcaOpenCodeSync(c *gin.Context) {
	c.Header("Content-Disposition", `attachment; filename="opencode-sync.py"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.String(http.StatusOK, arcaopencode.SyncScript())
}
