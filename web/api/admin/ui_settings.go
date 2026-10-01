package admin

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/internal/uisettings"
	"github.com/komari-monitor/komari/web/api"
	"net/http"
)

func GetUISettings(c *gin.Context) {
	values, err := uisettings.Read(dbcore.GetDBInstance())
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondSuccess(c, values)
}

func PatchUISettings(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if len(c.Request.URL.Query()) != 0 {
		api.RespondError(c, http.StatusBadRequest, "UI settings do not accept a theme or path target")
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil || patch == nil {
		api.RespondError(c, http.StatusBadRequest, "expected a settings object")
		return
	}
	values, err := uisettings.Patch(dbcore.GetDBInstance(), patch)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, uisettings.ErrInvalidSetting) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, uisettings.ErrCorruptSettings) {
			status = http.StatusConflict
		}
		api.RespondError(c, status, err.Error())
		return
	}
	auditlog.Log(c.ClientIP(), c.GetString("uuid"), "update built-in UI settings", "info")
	api.RespondSuccess(c, values)
}
