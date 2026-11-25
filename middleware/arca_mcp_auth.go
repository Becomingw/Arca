package middleware

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ArcaMCPAuth validates every MCP HTTP request using the current scoped PAT
// and its live account. Dashboard sessions and legacy tokens cannot use MCP.
func ArcaMCPAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.GetEnvOrDefaultBool("ARCA_MCP_ENABLED", true) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if !validArcaMCPOrigin(c.Request) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Header("WWW-Authenticate", "Bearer")
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !strings.HasPrefix(parts[1], model.AccessTokenPrefix) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ref := model.AccessTokenFingerprint(parts[1])
		token, err := model.FindUserAccessTokenByHash(ref)
		if err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		if token == nil || token.Expired(common.GetTimestamp()) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		user, err := model.GetUserById(token.UserId, false)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		if user.Status != common.UserStatusEnabled || !validUserInfo(user.Username, user.Role) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if user.Role < common.RoleAdminUser {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Writer.Header().Del("WWW-Authenticate")
		c.Set("id", user.Id)
		c.Set("role", user.Role)
		c.Set("username", user.Username)
		c.Set("use_access_token", true)
		base := user.ToBaseUser()
		beginAccessTokenAudit(c, base, ref)
		setAccessTokenContext(c, &accessTokenLookup{ref: ref, user: base, token: token})
		c.Next()
		finishAccessTokenAudit(c)
	}
}

func validArcaMCPOrigin(request *http.Request) bool {
	host := request.Host
	if bare, _, err := net.SplitHostPort(host); err == nil {
		host = bare
	}
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	hosts := common.GetEnvOrDefaultString("ARCA_MCP_ALLOWED_HOSTS", "localhost,127.0.0.1,::1")
	hostAllowed := false
	for candidate := range strings.SplitSeq(hosts, ",") {
		if candidate = strings.TrimSpace(candidate); candidate != "" && strings.EqualFold(host, candidate) {
			hostAllowed = true
			break
		}
	}
	if !hostAllowed {
		return false
	}
	origins := request.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 || origins[0] == "null" || origins[0] == "" {
		return false
	}
	for candidate := range strings.SplitSeq(os.Getenv("ARCA_MCP_ALLOWED_ORIGINS"), ",") {
		if strings.TrimSpace(candidate) == origins[0] {
			return true
		}
	}
	return false
}
