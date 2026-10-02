package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/internal/securitylimit"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	v2 "github.com/komari-monitor/komari/protocol/v2"
	agent_runtime "github.com/komari-monitor/komari/web/agent"
)

var liveRequests = securitylimit.New(10000)

func GetClients(c *gin.Context) {
	// 升级到ws
	if !IsWebSocketUpgrade(c) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Require WebSocket upgrade"})
		return
	}
	// Upgrade the HTTP connection to a WebSocket connection
	conn, err := UpgradeSafeConn(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Failed to upgrade to WebSocket." + err.Error()})
		return
	}
	defer conn.Close()
	initial := IdentifyPrincipal(c)
	stopGuard := conn.Guard(func() bool {
		p := IdentifyPrincipal(c)
		private, err := config.GetAs[bool](config.PrivateSiteKey, false)
		return err == nil && p.Type == initial.Type && p.UserUUID == initial.UserUUID && (!private || p.PrimaryRole() == RoleAdmin)
	})
	defer stopGuard()

	// 请求
	for {
		var resp struct {
			Online []string             `json:"online"` // 已建立连接的客户端uuid列表
			Data   map[string]v2.Report `json:"data"`   // 最后上报的数据
		}

		resp.Online = []string{}
		resp.Data = map[string]v2.Report{}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		message := string(data)
        if !liveRequests.Allow(c.ClientIP(),300,time.Minute) { return }
        p := IdentifyPrincipal(c)
        isLogin := p.PrimaryRole() == RoleAdmin
        private, err := config.GetAs[bool](config.PrivateSiteKey, false)
        if err != nil || (private && !isLogin) { return }
        var currentClients []models.Client
        if err := dbcore.GetDBInstance().Select("uuid", "hidden").Find(&currentClients).Error; err != nil { return }
        visible := make(map[string]bool, len(currentClients))
        for _, node := range currentClients { if isLogin || !node.Hidden { visible[node.UUID] = true } }

		uuID := ""
		if message != "get" { // 非请求全部内容
			if strings.HasPrefix(message, "get ") {
				uuID = strings.TrimSpace(strings.TrimPrefix(message, "get "))
			} else {
				conn.WriteJSON(gin.H{"status": "error", "error": "Invalid message"})
				continue
			}
		}

		// 在线客户端uuid列表（WebSocket 与非 WebSocket）
		for _, key := range agent_runtime.GetAllOnlineUUIDs() {
			if !visible[key] {
				continue
			}
			if uuID != "" && key != uuID {
				continue
			}
			resp.Online = append(resp.Online, key)
		}

		//过往节点数据信息
		for key, report := range agent_runtime.GetLatestReport() {
			if !visible[key] {
				continue
			}
			if uuID != "" && key != uuID {
				continue
			}

			report.UUID = "" // 不暴露 uuid
			if report.CPU.Usage == 0 {
				report.CPU.Usage = 0.01
			}
			resp.Data[key] = *report
		}

		err = conn.WriteJSON(gin.H{"status": "success", "data": resp})
		if err != nil {
			return
		}
	}
}
