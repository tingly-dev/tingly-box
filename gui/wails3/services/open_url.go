package services

import (
	"crypto/subtle"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// openURLRequest is the body of POST /api/v1/gui/open-url.
type openURLRequest struct {
	URL string `json:"url"`
}

// openURLHandler serves POST /api/v1/gui/open-url: the desktop bridge's
// openExternal (frontend/src/host/desktop.ts), which every external link in
// the window goes through (host/externalLinks.ts). A WebView has nowhere to
// put a new tab — target="_blank" is swallowed, and a plain external link
// would navigate the app window away — so the shell hands the URL to the
// OS's default browser.
//
// Only http(s) and mailto are opened: the OS opener runs whatever handler a
// scheme maps to, so file:, custom app schemes and the like are refused.
func openURLHandler(userToken func() string, open func(url string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		want := "Bearer " + userToken()
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte(want)) != 1 {
			c.Status(http.StatusForbidden)
			return
		}

		var req openURLRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
		if !isExternalURL(req.URL) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "only http, https and mailto URLs can be opened"})
			return
		}
		if err := open(req.URL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

func isExternalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Host != ""
	case "mailto":
		return u.Opaque != ""
	}
	return false
}
