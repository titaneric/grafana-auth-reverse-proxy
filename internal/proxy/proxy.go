package proxy

import (
	"github.com/labstack/echo/v4"
	"gitlab.pnet.ch/observability/grafana/grafana-auth-reverse-proxy/internal/config"
	"gitlab.pnet.ch/observability/grafana/grafana-auth-reverse-proxy/internal/jwks"
	"go.uber.org/zap"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
)

func Setup(e *echo.Echo, cfg *config.Config, l *zap.SugaredLogger) {
	targetURL, err := url.Parse(cfg.ProxyTarget)
	if err != nil {
		l.Errorw("Failed to parse proxy target URL", "error", err)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	e.Any(path.Join(cfg.BasePath, "/*"), func(c echo.Context) error {
		l.Debugw("Proxying request", "method", c.Request().Method, "uri", c.Request().RequestURI)

		req := c.Request()
		res := c.Response()

		req.URL.Host = targetURL.Host
		req.URL.Scheme = targetURL.Scheme
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		req.Host = targetURL.Host

		cookie, err := req.Cookie(cfg.AccessTokenCookieName)
		if err != nil {
			l.Debugw("Failed to get cookie", "error", err)
			return c.Redirect(http.StatusFound, "/auth")
		}

		claims, err := jwks.ParseJWTToken(cookie.Value, cfg.JwksUrl)
		if err != nil {
			l.Error("Error parsing token:", err)
		}

		loginOrEmail, err := jwks.ExtractClaimValue(claims, cfg.SyncLoginOrEmailClaimAttribute)
		if err != nil {
			l.Errorw("Failed to extract loginOrEmail from token", "error", err)
			return echo.NewHTTPError(http.StatusForbidden, "Access denied")
		}

		email, err := jwks.ExtractClaimValue(claims, cfg.SyncEmailClaimAttribute)
		if err != nil {
			l.Warnw("Failed to extract email from token", "error", err)
		}

		name, err := jwks.ExtractClaimValue(claims, cfg.SyncNameClaimAttribute)
		if err != nil {
			l.Warnw("Failed to extract name from token", "error", err)
		}

		if loginOrEmail != "" {
			l.Debugw("Extracted loginOrEmail", "loginOrEmail", loginOrEmail)
			req.Header.Set("X-WEBAUTH-USER", loginOrEmail)
		}

		if email != "" {
			l.Debugw("Extracted email", "email", email)
			req.Header.Set("X-WEBAUTH-EMAIL", email)
		}

		if name != "" {
			l.Debugw("Extracted name", "name", name)
			req.Header.Set("X-WEBAUTH-NAME", name)
		}

		proxy.ServeHTTP(res, req)
		return nil
	})

}
