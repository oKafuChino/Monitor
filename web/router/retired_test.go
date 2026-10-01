package router

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/cmd/flags"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/web/api"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetiredRoutesAndUIAuthorization(t *testing.T) {
	t.Chdir(t.TempDir())
	old := flags.DatabaseFile
	flags.DatabaseFile = filepath.Join(t.TempDir(), "test.db")
	t.Cleanup(func() { _ = dbcore.Close(); flags.DatabaseFile = old })
	if err := dbcore.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := config.Set(config.ApiKeyKey, "isolated-test-admin-key"); err != nil {
		t.Fatal(err)
	}
	if err := dbcore.GetDBInstance().Create(&models.Client{UUID: "test-node", Token: "isolated-test-agent-token", Name: "Test"}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(api.IdentityMiddleware())
	Register(r)
	request := func(method, path, body, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	// Prove credentials are valid before checking retired endpoints.
	if w := request("GET", "/api/admin/ui/settings", "", "isolated-test-admin-key"); w.Code != 200 {
		t.Fatalf("admin fixture: %d %s", w.Code, w.Body)
	}
	for _, path := range []string{"/api/admin/task/exec", "/api/admin/task/all", "/api/admin/task/test/result", "/api/admin/exec", "/api/admin/theme/list", "/api/admin/theme/settings", "/api/admin/plugin/list", "/api/plugin/old/index.html", "/themes/old/dist/index.html", "/themes/default/komari-theme.json", "/api/admin/client/test-node/terminal"} {
		for _, method := range []string{"GET", "POST"} {
			w := request(method, path, "{}", "isolated-test-admin-key")
			if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("retired endpoint %s %s: %d %s", method, path, w.Code, w.Body)
			}
		}
	}
	agentReq := httptest.NewRequest("GET", "/api/clients/terminal?token=isolated-test-agent-token", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = agentReq
	if p := api.IdentifyPrincipal(c); p.ClientUUID != "test-node" {
		t.Fatalf("agent fixture invalid: %#v", p)
	}
	agentReq.Header.Set("Connection", "Upgrade")
	agentReq.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, agentReq)
	if w.Code != 404 {
		t.Fatalf("agent terminal endpoint: %d", w.Code)
	}

	for _, method := range []string{"exec", "getTasks", "getTaskById", "getTasksByClientId", "getSpecificTaskResult", "getTaskResultsByTaskId"} {
		body := `{"jsonrpc":"2.0","id":1,"method":"admin:` + method + `","params":{"command":"echo isolated","clients":["test-node"]}}`
		result := request("POST", "/api/rpc2", body, "isolated-test-admin-key")
		var reply struct {
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(result.Body.Bytes(), &reply); err != nil || reply.Error.Code != -32601 {
			t.Fatalf("retired command RPC %s: %s", method, result.Body)
		}
	}
	result := request("POST", "/api/clients/v2/rpc?token=isolated-test-agent-token", `{"jsonrpc":"2.0","id":1,"method":"agent.taskResult","params":{"task_id":"old","result":"ignored","exit_code":0}}`, "")
	var reply struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &reply); err != nil || reply.Error.Code != -32601 {
		t.Fatalf("retired agent result accepted: %s", result.Body)
	}
	if dbcore.GetDBInstance().Migrator().HasTable("tasks") || dbcore.GetDBInstance().Migrator().HasTable("task_results") {
		t.Fatal("retired task tables created on new installation")
	}
	for _, path := range []string{"/api/admin/ui/settings", "/api/admin/ui/settings?token=isolated-test-agent-token"} {
		w := request("PATCH", path, `{"mainContentWidth":30}`, "")
		if w.Code != 401 {
			t.Fatalf("unauthorized UI write: %d %s", w.Code, w.Body)
		}
	}
	w = request("PATCH", "/api/admin/ui/settings", `{"showIpTagsInCard":false,"mainContentWidth":0,"backgroundImageUrlDesktop":""}`, "isolated-test-admin-key")
	if w.Code != 200 {
		t.Fatalf("UI patch: %d %s", w.Code, w.Body)
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &payload)
	if payload.Data["showIpTagsInCard"] != false || payload.Data["mainContentWidth"] != float64(0) {
		t.Fatalf("lost false/zero: %s", w.Body)
	}
	if w := request("PATCH", "/api/admin/ui/settings?theme=custom", `{}`, "isolated-test-admin-key"); w.Code != 400 {
		t.Fatal("theme selector accepted")
	}
	if w := request("POST", "/api/admin/settings/", `{"theme":"custom"}`, "isolated-test-admin-key"); w.Code == 200 {
		t.Fatal("general settings enabled custom theme")
	}
	for _, purpose := range []string{"theme", "plugin"} {
		if w := request("POST", "/api/admin/upload/init", `{"purpose":"`+purpose+`","filename":"old.zip","size":1}`, "isolated-test-admin-key"); w.Code != 400 {
			t.Fatalf("upload %s accepted: %d", purpose, w.Code)
		}
	}
	if w := request("GET", "/api/admin/client/list", "", "isolated-test-admin-key"); w.Code != 200 {
		t.Fatalf("retained client listing failed: %d %s", w.Code, w.Body)
	}
}
