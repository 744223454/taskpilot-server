package auth

import (
	"net/http"
	"time"

	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/gin-gonic/gin"
)

const refreshCookiePath = "/api/v1/auth"

func setSessionCookies(c *gin.Context, svcCtx *svc.ServiceContext, session *authlogic.AuthSession) error {
	csrfToken, err := jwtauth.GenerateCSRFToken()
	if err != nil {
		return err
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		jwtauth.AccessCookieName,
		session.Response.AccessToken,
		int(svcCtx.Config.Auth.AccessExpire),
		"/",
		"",
		svcCtx.Config.Auth.CookieSecure,
		true,
	)
	c.SetCookie(
		jwtauth.RefreshCookieName,
		session.RefreshToken,
		refreshCookieMaxAge(session.RefreshExpiresAt),
		refreshCookiePath,
		"",
		svcCtx.Config.Auth.CookieSecure,
		true,
	)
	c.SetCookie(
		jwtauth.CSRFCookieName,
		csrfToken,
		refreshCookieMaxAge(session.RefreshExpiresAt),
		"/",
		"",
		svcCtx.Config.Auth.CookieSecure,
		false,
	)
	return nil
}

func clearSessionCookies(c *gin.Context, svcCtx *svc.ServiceContext) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := svcCtx.Config.Auth.CookieSecure
	c.SetCookie(jwtauth.AccessCookieName, "", -1, "/", "", secure, true)
	c.SetCookie(jwtauth.RefreshCookieName, "", -1, refreshCookiePath, "", secure, true)
	c.SetCookie(jwtauth.CSRFCookieName, "", -1, "/", "", secure, false)
}

func refreshCookieMaxAge(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
