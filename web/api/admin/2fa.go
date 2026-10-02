package admin

import (
    "image/png"
    "net/http"
    "sync"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/komari-monitor/komari/database/accounts"
    "github.com/komari-monitor/komari/utils"
    "github.com/komari-monitor/komari/web/api"
    "github.com/pquerna/otp/totp"
)

type pending2FA struct { user, session, secret string; expires time.Time }
var pending2FAMu sync.Mutex
var pending2FAs = map[string]pending2FA{}

func Generate2FA(c *gin.Context) {
    user, err := accounts.GetUserByUUID(c.GetString("uuid"))
    session, _ := c.Cookie("session_token")
    if err != nil || session == "" || user.TwoFactor != "" { api.RespondError(c, 409, "2FA setup unavailable"); return }
    secret, img, err := accounts.Generate2Fa()
    if err != nil { api.RespondError(c, 500, "Failed to generate 2FA"); return }
    id := utils.GenerateRandomString(43)
    pending2FAMu.Lock()
    for key, pending := range pending2FAs { if time.Now().After(pending.expires) || pending.session == session { delete(pending2FAs, key) } }
    if len(pending2FAs) >= 100 { pending2FAMu.Unlock(); api.RespondError(c, 429, "Too many pending operations"); return }
    pending2FAs[id] = pending2FA{user.UUID, session, secret, time.Now().Add(5*time.Minute)}
    pending2FAMu.Unlock()
    c.SetSameSite(http.SameSiteStrictMode)
    c.SetCookie("2fa_operation", id, 300, "/", "", utils.GetScheme(c) == "https", true)
    c.Header("Content-Type", "image/png")
    _ = png.Encode(c.Writer, img)
}

func Enable2FA(c *gin.Context) {
    id, _ := c.Cookie("2fa_operation")
    session, _ := c.Cookie("session_token")
    var input struct { Code string `json:"code"` }
    if c.ShouldBindJSON(&input) != nil { api.RespondError(c, 400, "Invalid setup request"); return }
    pending2FAMu.Lock()
    pending, ok := pending2FAs[id]
    if ok && pending.user == c.GetString("uuid") && pending.session == session { delete(pending2FAs, id) } else { ok = false }
    pending2FAMu.Unlock()
    if !ok || time.Now().After(pending.expires) || !totp.Validate(input.Code, pending.secret) { api.RespondError(c, 400, "Invalid or expired setup operation; generate a new QR code"); return }
    if err := accounts.Enable2Fa(pending.user, pending.secret); err != nil { api.RespondError(c, 409, "2FA setup failed"); return }
    c.SetCookie("2fa_operation", "", -1, "/", "", utils.GetScheme(c) == "https", true)
    api.RespondSuccess(c, "2FA enabled; sign in again")
}

func Disable2FA(c *gin.Context) {
    if err := accounts.Disable2Fa(c.GetString("uuid")); err != nil { api.RespondError(c, 500, "Failed to disable 2FA"); return }
    api.RespondSuccess(c, "2FA disabled; sign in again")
}
