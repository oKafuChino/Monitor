package client

import (
	"context"
	"time"
	"fmt"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/internal/metricstore"
	"github.com/komari-monitor/komari/internal/securitylimit"
	"github.com/komari-monitor/komari/utils/renewal"
	v2 "github.com/komari-monitor/komari/protocol/v2"
	agent_runtime "github.com/komari-monitor/komari/web/agent"
)

var ingestAttempts = securitylimit.New(10000)

var gpuIdentityMu sync.Mutex
type gpuIdentity struct { signatures map[string]bool; expires time.Time }
var gpuIdentities = map[string]gpuIdentity{}

func acceptGPUIdentity(uuid string, report v2.Report) bool {
 if report.GPU==nil { return true }
 names:=make([]string,0,len(report.GPU.DetailedInfo)); for _,device:=range report.GPU.DetailedInfo { names=append(names,device.Name) }
 raw,_:=json.Marshal(names); digest:=sha256.Sum256(raw); signature:=hex.EncodeToString(digest[:])
 gpuIdentityMu.Lock(); defer gpuIdentityMu.Unlock()
 now:=time.Now(); identity,ok:=gpuIdentities[uuid]
 if !ok || !now.Before(identity.expires) {
  if len(gpuIdentities)>=10000 { for id,old:=range gpuIdentities { if !now.Before(old.expires) { delete(gpuIdentities,id) } }; if len(gpuIdentities)>=10000 && !ok { return false } }
  identity=gpuIdentity{signatures:map[string]bool{},expires:now.Add(24*time.Hour)}
 }
 if !identity.signatures[signature] && len(identity.signatures)>=4 { return false }
 identity.signatures[signature]=true; gpuIdentities[uuid]=identity; return true
}

// ingest.go
// agent 上报数据的传输无关处理逻辑。v2 的 HTTP/WebSocket JSON-RPC 入口
// 经过协议解析后，统一调用这里的函数落库并更新运行时状态。

// ingestReport 保存一次负载上报并刷新运行时状态。
// markPresence 为 true 时按 POST 上报会话刷新在线状态（WS 连接自行管理在线状态，应传 false）。
func ingestReport(uuid string, report v2.Report, markPresence bool) error {
	if !ingestAttempts.Allow("report:"+uuid, 90, time.Minute) { return fmt.Errorf("report rate limit exceeded") }
	client, err := clients.GetClientByUUID(uuid)
	if err != nil { return fmt.Errorf("node no longer exists") }
	report.UUID = uuid
	report.UpdatedAt = time.Now().UTC()
	if err := clients.ReportVerify(report); err != nil {
		return err
	}
	if !acceptGPUIdentity(uuid,report) { return fmt.Errorf("GPU identity change budget exceeded") }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	savedReport, err := metricstore.WriteReport(ctx, report)
	if err != nil {
		return err
	}
	agent_runtime.RecordReport(savedReport)
	agent_runtime.MarkV2Client(uuid)
	if markPresence {
		refreshPostPresence(uuid)
	}
	renewal.CheckAndAutoRenewal(client)
	return nil
}

// ingestBasicInfo 保存客户端基础信息。fallbackIP 在上报未携带 IP 时用作兜底。
func ingestBasicInfo(uuid string, info map[string]interface{}, fallbackIP string) error {
	if !ingestAttempts.Allow("info:"+uuid, 10, time.Minute) { return fmt.Errorf("basic info rate limit exceeded") }
	if info == nil {
		info = map[string]interface{}{}
	}
	return saveClientBasicInfo(info, uuid, fallbackIP)
}

// ingestPingResult 保存一条 ping 探测结果。
func ingestPingResult(uuid string, taskID uint, value int) error {
	return tasks.SavePingRecord(models.PingRecord{
		Client: uuid,
		TaskId: taskID,
		Value:  value,
		Time:   time.Now().UTC(),
	})
}
