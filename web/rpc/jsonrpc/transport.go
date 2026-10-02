package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database/accounts"
	"github.com/komari-monitor/komari/internal/securitylimit"
	"github.com/komari-monitor/komari/pkg/rpc"
	"github.com/komari-monitor/komari/web/api"
)

var rpcAttempts = securitylimit.New(10000)

// OnRpcRequest 是 /api/rpc2 的统一入口：GET 升级为 WebSocket，POST 处理单条/批量 JSON-RPC。
func OnRpcRequest(c *gin.Context) {
	if !rpcAttempts.Allow("ip:"+c.ClientIP(), 300, time.Minute) { c.AbortWithStatus(http.StatusTooManyRequests); return }
	// GET -> WebSocket
	if c.Request.Method == http.MethodGet {
		serveWebSocket(c)
		return
	}

	if c.Request.Method != http.MethodPost {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
		return
	}
	servePost(c)
}

// CallFromGin 供传统 gin handler / 路由桥转调 RPC 方法。
// 复用 IdentityMiddleware 已识别的 principal；未识别时兜底调用 IdentifyPrincipal。
func CallFromGin(c *gin.Context, method string, params any) *rpc.JsonRpcResponse {
	if !rpcAttempts.Allow("ip:"+c.ClientIP(),300,time.Minute) { return rpc.ErrorResponse(nil,rpc.Unavailable,"request rate exceeded",nil) }
	meta := buildContextMeta(c)
	req := &rpc.JsonRpcRequest{Version: rpc.RPC_VERSION, Method: method, Params: params}
	return dispatchWithSensitive(c.Request.Context(), c, meta, req)
}

// dispatchWithSensitive 在统一分发前对敏感方法补充二次验证，使各调用入口行为一致。
// 对已通过命名空间权限校验的敏感方法，要求调用方满足敏感操作 2FA。
// 校验基于 Principal(API Key 放行；用户需要 TOTP 或当前密码)，Dispatch 仍为权威鉴权点。
//
// 2FA code 按"每请求"提取:优先取自本条 RPC 请求的 params(2fa_code/two_factor_code/otp),
// 这对 WebSocket 长连接尤其重要——每条敏感消息携带新鲜的 TOTP 码,避免连接级握手码过期或被复用;
// 缺失时回退到 X-2FA-Code / X-Two-Factor-Code 请求头，不从 URL 读取验证码。
//
// 若请求已被 RequireSensitive2FA 中间件校验过(sensitive_2fa_verified),则跳过,避免重复校验。
func dispatchWithSensitive(ctx context.Context, c *gin.Context, meta *rpc.ContextMeta, req *rpc.JsonRpcRequest) *rpc.JsonRpcResponse {
	if !validRPCParams(req.Params,0) { return rpc.ErrorResponse(req.ID,rpc.InvalidParams,"parameter collection or nesting limit exceeded",nil) }
	if meta != nil && meta.Principal != nil && (c == nil || !c.GetBool("sensitive_2fa_verified")) &&
		rpc.IsSensitive(req.Method) && rpc.CheckPrincipal(meta.Principal, req.Method) {
		code := extractRequestTwoFACode(req)
		if code == "" && c != nil {
			code = headerOrQueryTwoFACode(c)
		}
		password, _ := rpc.GetParamAs[string](req, "reauth_password")
		if password == "" && c != nil { password = c.GetHeader("X-Reauth-Password") }
		if err := api.VerifySensitive2FACore(meta.Principal.UserUUID, code, meta.Principal.IsAPIKey, password); err != nil {
			return rpc.ErrorResponse(req.ID, rpc.PermissionDenied, err.Error(), nil)
		}
	}
	// Authentication metadata must never reach map-based mutation handlers.
	if params, ok := req.Params.(map[string]interface{}); ok {
		clean := make(map[string]interface{}, len(params))
		for key, value := range params { if key != "2fa_code" && key != "two_factor_code" && key != "otp" && key != "reauth_password" { clean[key] = value } }
		req.Params = clean
	}
	return Dispatch(ctx, meta, req)
}

// extractRequestTwoFACode 从单条 RPC 请求的命名参数中提取 2FA code。
// 仅支持对象(map)形式的 params;按 2fa_code / two_factor_code / otp 顺序查找。
func extractRequestTwoFACode(req *rpc.JsonRpcRequest) string {
	for _, key := range []string{"2fa_code", "two_factor_code", "otp"} {
		if v, ok := rpc.GetParamAs[string](req, key); ok && v != "" {
			return v
		}
	}
	return ""
}

// headerOrQueryTwoFACode 从请求头 / query 兜底提取 2FA code(不读取 body,避免消费请求体)。
func headerOrQueryTwoFACode(c *gin.Context) string {
	if code := c.GetHeader("X-2FA-Code"); code != "" {
		return code
	}
	if code := c.GetHeader("X-Two-Factor-Code"); code != "" {
		return code
	}
	return ""
}

func serveWebSocket(c *gin.Context) {
	conn, err := api.UpgradeSafeConn(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Failed to upgrade to WebSocket." + err.Error()})
		return
	}
	defer conn.Close()

	initial := api.IdentifyPrincipal(c)
	stopGuard := conn.Guard(func() bool { p := api.IdentifyPrincipal(c); return p.Type == initial.Type && p.UserUUID == initial.UserUUID && p.ClientUUID == initial.ClientUUID })
	defer stopGuard()
	for {
		var req rpc.JsonRpcRequest
		if err := conn.ReadJSON(&req); err != nil {
			var se *json.SyntaxError
			var ute *json.UnmarshalTypeError
			if errors.As(err, &se) || errors.As(err, &ute) {
				conn.WriteJSON(rpc.ErrorResponse(nil, rpc.InvalidRequest, "bad request: "+err.Error(), nil))
				continue
			}
			// 其它视为连接/IO 错误，结束循环
			break
		}
		if jerr := req.Validate(); jerr != nil {
			conn.WriteJSON(jerr.ResponseWithID(req.ID))
			continue
		}
		if !validRPCParams(req.Params, 0) { return }
		if !rpcAttempts.Allow("ip:"+c.ClientIP(), 300, time.Minute) { return }
		// 同步写：SafeConn 内部有锁，串行写避免响应乱序与并发竞态。
		api.SetPrincipal(c, api.IdentifyPrincipal(c))
		meta := buildContextMeta(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		response := dispatchWithSensitive(ctx, c, meta, &req)
		cancel()
		if err := writeRPCResponse(conn.WriteMessage, response); err != nil { return }
	}
}

func servePost(c *gin.Context) {
	body, err := api.ReadBounded(c.Request.Body, api.JSONBodyLimit)
	if err != nil {
		c.JSON(http.StatusBadRequest, rpc.ErrorResponse(nil, rpc.ParseError, "read body error", err.Error()))
		return
	}
	requests, jerr := rpc.ParseRequests(body)
	if jerr != nil {
		c.JSON(http.StatusBadRequest, jerr.Response())
		return
	}
	if len(requests) > 20 { c.JSON(http.StatusBadRequest, rpc.ErrorResponse(nil, rpc.InvalidRequest, "batch limit is 20", nil)); return }
	for _, request := range requests { if !validRPCParams(request.Params, 0) { c.JSON(http.StatusBadRequest, rpc.ErrorResponse(nil, rpc.InvalidParams, "parameter collection or nesting limit exceeded", nil)); return } }
	meta := buildContextMeta(c)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	responses := make([]*rpc.JsonRpcResponse, 0, len(requests))
	for _, rreq := range requests {
		if ctx.Err() != nil {
			responses = append(responses, rpc.ErrorResponse(rreq.ID, rpc.Unavailable, "batch deadline exceeded", nil))
			continue
		}
		responses = append(responses, dispatchWithSensitive(ctx, c, meta, rreq))
	}
	// 单条直接对象，批量数组（符合 JSON-RPC 2.0）。
	if len(responses) == 1 {
		writeRPCPost(c, responses[0])
	} else {
		writeRPCPost(c, responses)
	}
}

func validRPCParams(value any, depth int) bool {
 if depth > 16 { return false }
 switch typed := value.(type) {
 case []interface{}:
  if len(typed)>1024 { return false }; for _, item := range typed { if !validRPCParams(item,depth+1) { return false } }
 case map[string]interface{}:
  if len(typed)>1024 { return false }; for key,item := range typed { if len(key)>256 || !validRPCParams(item,depth+1) { return false } }
 }
 return true
}

func writeRPCResponse(write func(int, []byte) error, response *rpc.JsonRpcResponse) error {
 data,err := json.Marshal(response); if err != nil { return err }
 if len(data)>8<<20 { data,err = json.Marshal(rpc.ErrorResponse(response.ID,rpc.InternalError,"response exceeds size limit",nil)); if err != nil { return err } }
 return write(1,data)
}

func writeRPCPost(c *gin.Context, response any) {
 data,err := json.Marshal(response)
 if err != nil || len(data)>8<<20 { c.JSON(http.StatusBadRequest,rpc.ErrorResponse(nil,rpc.InternalError,"response exceeds size limit",nil)); return }
 c.Data(http.StatusOK,"application/json; charset=utf-8",data)
}

// buildContextMeta 从 gin.Context 构建 *rpc.ContextMeta。
// 复用 IdentityMiddleware 已识别的 principal(api.GetPrincipal)；若未识别则兜底调用
// api.IdentifyPrincipal。填充 principal、Permission(兼容)、User、各 UUID、token 等字段。
func buildContextMeta(c *gin.Context) *rpc.ContextMeta {
	// 优先读取中间件已识别的 principal；未识别时兜底自行识别(如 /api/rpc2 请求)。
	p := api.GetPrincipal(c)
	if p == nil {
		p = api.IdentifyPrincipal(c)
	}

	meta := &rpc.ContextMeta{
		Principal:  p,
		Permission: p.PrimaryRole(), // 兼容现有 handler 与 Dispatch
		RemoteIP:   c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
	}

	// 根据主体类型填充具体字段。
	switch p.Type {
	case rpc.PrincipalUser:
		meta.UserUUID = p.UserUUID
		if session, err := c.Cookie("session_token"); err == nil && session != "" {
			meta.SessionToken = session
			if user, err := accounts.GetUserBySession(session); err == nil {
				meta.User = &user
			}
		}
	case rpc.PrincipalAgent:
		meta.ClientUUID = p.ClientUUID
		// 尝试提取 client token(用于某些 handler 需要原始 token 的场景)。
		// 优先查询参数 ?Authorization=<token>，再尝试 Bearer header。
		if token := c.Query("Authorization"); token != "" {
			meta.ClientToken = token
		} else if auth := c.GetHeader("Authorization"); auth != "" && len(auth) > len("Bearer ") {
			meta.ClientToken = auth[len("Bearer "):]
		}
	}

	return meta
}
