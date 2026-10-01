package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/komari-monitor/komari/cmd/flags"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/internal/metricstore"
	"github.com/komari-monitor/komari/pkg/metric"
	v2 "github.com/komari-monitor/komari/protocol/v2"
	agent "github.com/komari-monitor/komari/web/agent"
)

func TestDiskIOHTTPGzipAndWebSocket(t *testing.T) {
	flags.DatabaseType = "sqlite"
	flags.DatabaseFile = fmt.Sprintf("file:io-config-%d?mode=memory&cache=shared", time.Now().UnixNano())
	config.SetDb(dbcore.GetDBInstance())
	ctx := context.Background()
	if err := config.SetMany(map[string]any{metricstore.MetricDBDriverKey:"sqlite", metricstore.MetricDBDSNKey:fmt.Sprintf("file:io-metrics-%d?mode=memory&cache=shared", time.Now().UnixNano())}); err != nil { t.Fatal(err) }
	if err := metricstore.InitializeStore(); err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = metricstore.CloseStoreContext(ctx) })
	metricstore.StartReportBatcher()
	t.Cleanup(func() { _ = metricstore.StopReportBatcher(ctx) })
	const uuid = "authenticated-io-node"
	t.Cleanup(func() { agent.DeleteLatestReport(uuid); agent.DeleteConnectedClients(uuid) })
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("client_uuid", uuid); c.Next() })
	router.POST("/rpc", UploadV2RPC)
	router.GET("/rpc", WebSocketV2RPC)
	server := httptest.NewServer(router); defer server.Close()
	zero, write := 0.0, 512.0
	request := v2.Request{JSONRPC:v2.Version, Method:v2.MethodAgentReport, ID:"io", Params:v2.ReportParams{Report:v2.Report{UUID:"spoofed", UpdatedAt:time.Unix(1, 0), DiskIO:&v2.DiskIOReport{Status:"ok", ReadBytesPerSec:&zero, WriteBytesPerSec:&write, SampleIntervalMS:2000}}}}
	for _, compressed := range []bool{false, true} {
		// The metric store deduplicates timestamps at millisecond precision.
		// Separate requests so this transport test expects distinct samples.
		time.Sleep(2 * time.Millisecond)
		payload, err := json.Marshal(request); if err != nil { t.Fatal(err) }
		if compressed { var b bytes.Buffer; gz := gzip.NewWriter(&b); if _, err := gz.Write(payload); err != nil { t.Fatal(err) }; if err := gz.Close(); err != nil { t.Fatal(err) }; payload = b.Bytes() }
		req, err := http.NewRequest(http.MethodPost, server.URL+"/rpc", bytes.NewReader(payload)); if err != nil { t.Fatal(err) }
		if compressed { req.Header.Set("Content-Encoding", "gzip") }
		resp, err := http.DefaultClient.Do(req); if err != nil { t.Fatal(err) }
		var rpcResponse v2.Response; err = json.NewDecoder(resp.Body).Decode(&rpcResponse); resp.Body.Close()
		if err != nil || rpcResponse.Error != nil { t.Fatalf("HTTP report: %+v %v", rpcResponse, err) }
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/rpc", nil); if err != nil { t.Fatal(err) }; defer conn.Close()
	time.Sleep(2 * time.Millisecond)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil { t.Fatal(err) }
	if err := conn.WriteJSON(request); err != nil { t.Fatal(err) }
	var response v2.Response; if err := conn.ReadJSON(&response); err != nil || response.Error != nil { t.Fatalf("WS report: %+v %v", response, err) }
	if err := metricstore.FlushReportBatch(ctx); err != nil { t.Fatal(err) }
	latest := agent.GetLatestReport()[uuid]
	if latest == nil || latest.DiskIO == nil || *latest.DiskIO.WriteBytesPerSec != write || latest.UUID != uuid || !latest.UpdatedAt.After(time.Now().Add(-time.Minute)) { t.Fatalf("bad received report: %+v", latest) }
	points, err := metricstore.GetStore().Query(ctx, metric.Query{MetricName:metricstore.MetricDiskIORead, EntityID:uuid, Start:time.Now().Add(-time.Minute), End:time.Now().Add(time.Minute)})
	if err != nil || len(points) != 3 { t.Fatalf("three authenticated reports: %v %v", points, err) }
	for _, point := range points { if point.Value != 0 { t.Fatal("idle zero lost") } }
	if agent.GetLatestReport()["spoofed"] != nil { t.Fatal("report overwrote authenticated identity") }
	invalid := handleV2RPC(uuid, v2.Request{JSONRPC:v2.Version, Method:v2.MethodAgentReport, ID:2, Params:v2.ReportParams{Report:v2.Report{DiskIO:&v2.DiskIOReport{Status:"ok"}}}}, false)
	if invalid.Error == nil || invalid.Error.Code != -32602 { t.Fatal("invalid IO not a parameter error") }
	old := handleV2RPC(uuid, v2.Request{JSONRPC:v2.Version, Method:v2.MethodAgentReport, ID:3, Params:v2.ReportParams{Report:v2.Report{}}}, false)
	if old.Error != nil { t.Fatalf("old probe rejected: %+v", old.Error) }
}
