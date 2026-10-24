package admin

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/internal/themes"
	"github.com/komari-monitor/komari/web/api"
	"github.com/komari-monitor/komari/web/public"
	"github.com/komari-monitor/komari/web/upload"
)

func ListThemes(c *gin.Context) {
	installed, err := themes.List()
	if err != nil { api.RespondError(c, http.StatusInternalServerError, err.Error()); return }
	var builtin themes.Manifest
	raw, err := public.PublicFS.ReadFile("defaultTheme/komari-theme.json")
	if err != nil { api.RespondError(c, http.StatusInternalServerError, err.Error()); return }
	if err := json.Unmarshal(raw, &builtin); err != nil { api.RespondError(c, http.StatusInternalServerError, err.Error()); return }
	api.RespondSuccess(c, gin.H{"active":themes.Active(), "themes":append([]themes.Manifest{builtin}, installed...)})
}

func SetTheme(c *gin.Context) {
	var request struct { Short string `json:"short" binding:"required"` }
	if err := c.ShouldBindJSON(&request); err != nil { api.RespondError(c, http.StatusBadRequest, "expected a theme short identifier"); return }
	if err := themes.Select(request.Short); err != nil { api.RespondError(c, http.StatusBadRequest, err.Error()); return }
	auditlog.Log(c.ClientIP(), c.GetString("uuid"), "select theme: "+request.Short, "info")
	api.RespondSuccess(c, gin.H{"active":request.Short})
}

func DeleteTheme(c *gin.Context) {
	var request struct { Short string `json:"short" binding:"required"` }
	if err := c.ShouldBindJSON(&request); err != nil { api.RespondError(c, http.StatusBadRequest, "expected a theme short identifier"); return }
	if err := themes.Delete(dbcore.GetDBInstance(), request.Short); err != nil { api.RespondError(c, http.StatusBadRequest, err.Error()); return }
	auditlog.Log(c.ClientIP(), c.GetString("uuid"), "delete theme: "+request.Short, "info")
	api.RespondSuccess(c, nil)
}

func GetThemeSettings(c *gin.Context) {
	values, err := themes.Settings(dbcore.GetDBInstance(), c.Query("theme"))
	if err != nil { api.RespondError(c, http.StatusBadRequest, err.Error()); return }
	api.RespondSuccess(c, values)
}

func SaveThemeSettings(c *gin.Context) {
	var values map[string]any
	if err := c.ShouldBindJSON(&values); err != nil || values == nil { api.RespondError(c, http.StatusBadRequest, "expected a settings object"); return }
	short := c.Query("theme")
	if err := themes.SaveSettings(dbcore.GetDBInstance(), short, values); err != nil { api.RespondError(c, http.StatusBadRequest, err.Error()); return }
	auditlog.Log(c.ClientIP(), c.GetString("uuid"), "update theme settings: "+short, "info")
	api.RespondSuccess(c, values)
}

func finalizeThemeUpload(session upload.Session) (upload.Result, error) {
	m, err := themes.Install(session.ArchivePath)
	if err != nil { return upload.Result{}, err }
	return upload.Result{Message:"Theme installed. Select it explicitly to activate.", Data:m}, nil
}
