package share

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "net/url"
 "path/filepath"
 "strings"
 "testing"
 "time"
 "github.com/gin-gonic/gin"
 "github.com/komari-monitor/komari/database/models"
 "github.com/komari-monitor/komari/internal/sharing"
 "github.com/komari-monitor/komari/web/agent"
 v2 "github.com/komari-monitor/komari/protocol/v2"
 "gorm.io/driver/sqlite"
 "gorm.io/gorm"
)
func TestShareClosedRoutesAndBinding(t *testing.T){
 gin.SetMode(gin.TestMode);db,err:=gorm.Open(sqlite.Open(filepath.Join(t.TempDir(),"share.db")),&gorm.Config{});if err!=nil{t.Fatal(err)}
 sqlDB,_:=db.DB();sqlDB.SetMaxOpenConns(1);t.Cleanup(func(){sqlDB.Close();sharing.Configure("");agent.DeleteLatestReport("a")})
 if err:=db.AutoMigrate(&models.Client{},&models.ShareLink{},&models.ShareSession{},&models.PingTask{});err!=nil{t.Fatal(err)}
 if err:=db.Create(&models.Client{UUID:"a",Name:"Shared",Token:"secret-agent",IPv4:"192.0.2.1",Remark:"private",PublicRemark:"main.example.com",Price:42}).Error;err!=nil{t.Fatal(err)}
 sharing.Configure("https://share.example.com");service:=sharing.New(db)
 link,full,err:=service.Create("a","1d","admin");if err!=nil{t.Fatal(err)};origin,_:=url.Parse(sharing.PublicBase());router,err:=New(service,origin,nil);if err!=nil{t.Fatal(err)}
 request:=func(method,path,body string,cookie *http.Cookie)*httptest.ResponseRecorder{r:=httptest.NewRequest(method,"https://share.example.com"+path,strings.NewReader(body));if cookie!=nil{r.AddCookie(cookie)};w:=httptest.NewRecorder();router.ServeHTTP(w,r);return w}
 for _,p:=range []string{"/","/api/nodes","/api/rpc2","/api/login","/admin","/sw.js","/manifest.webmanifest","/assets/main.js","/api/clients","/node/other","/node?uuid=b"}{w:=request("GET",p,"",nil);if w.Code!=404&&w.Code!=400{t.Fatalf("route leaked %s: %d",p,w.Code)}}
 legacy:=&http.Cookie{Name:"temp_key",Value:"legacy"};if w:=request("GET","/node","",legacy);w.Code!=404{t.Fatal("legacy token accepted")}
 w:=request("GET",strings.TrimPrefix(full,sharing.PublicBase()),"",nil);if w.Code!=302||w.Header().Get("Location")!="/node"{t.Fatal("exchange failed")}
 cookies:=w.Result().Cookies();if len(cookies)!=1||!cookies[0].HttpOnly||!cookies[0].Secure||cookies[0].Domain!=""||cookies[0].MaxAge>86400{t.Fatal("unsafe cookie")};cookie:=cookies[0]
 for k,v:=range map[string]string{"Cache-Control":"no-store","Referrer-Policy":"no-referrer"}{if w.Header().Get(k)!=v{t.Fatal("missing header",k)}}
 rpcCall:=func(method,params string)*httptest.ResponseRecorder{return request("POST","/api/share/rpc",`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`,cookie)}
 for _,method:=range []string{"public:getNodesInformation","admin:listClients","agent.report","rpc.help"}{w:=rpcCall(method,"{}");if !strings.Contains(w.Body.String(),"Method not found"){t.Fatal("method escaped allowlist",method)}}
 for _,params:=range []string{`{"uuid":"b"}`,`{"entity_ids":["b"]}`,`{"tags":{}}`}{if !strings.Contains(rpcCall("share:getNode",params).Body.String(),"Invalid params"){t.Fatal("UUID accepted")}}
 nodeBody:=rpcCall("share:getNode","{}").Body.String();for _,secret:=range []string{"secret-agent","192.0.2.1","main.example.com","public_remark","node_uuid","token_hash"}{if strings.Contains(nodeBody,secret){t.Fatal("privacy leak",secret)}}
 agent.RecordReport(v2.Report{UUID:"a",Message:"do not expose",CPU:v2.CPUReport{Usage:12},UpdatedAt:time.Now().UTC()})
 latest:=rpcCall("share:getLatestStatus","{}").Body.String();if strings.Contains(latest,"do not expose")||!strings.Contains(latest,`"usage":12`){t.Fatal("bad status DTO")}
 for _,p:=range []string{`{"metric_keys":["secret"]}`,`{"metric_keys":["cpu.usage"],"max_points":501}`,`{"metric_keys":["cpu.usage"],"hours":721}`,`{"metric_keys":["cpu.usage"],"entity_id":"b"}`}{if !strings.Contains(rpcCall("share:queryMetrics",p).Body.String(),"Invalid query"){t.Fatal("unsafe query",p)}}
 if w:=request("POST","/api/share/rpc",`[{"jsonrpc":"2.0","id":1,"method":"share:getNode"}]`,cookie);w.Code!=400{t.Fatal("batch allowed")}
 r:=httptest.NewRequest("POST","https://share.example.com/api/share/rpc",strings.NewReader("{}"));r.AddCookie(cookie);r.Header.Set("Origin","https://evil.example.com");w=httptest.NewRecorder();router.ServeHTTP(w,r);if w.Code!=403{t.Fatal("cross origin allowed")}
 if err:=service.Revoke(link.ID);err!=nil{t.Fatal(err)};if w:=rpcCall("share:getNode","{}");w.Code!=404{t.Fatal("revocation delayed")}
}
func TestQueryBoundaryAndSafeReport(t *testing.T){
 now:=time.Now().UTC();p:=queryParams{MetricKeys:[]string{"disk.io.read.rate"},Hours:720,MaxPoints:500};if !validateQuery(&p,now){t.Fatal("valid max window refused")}
 p=queryParams{MetricKeys:[]string{"cpu.usage"},End:now.Add(time.Second)};if validateQuery(&p,now){t.Fatal("future end allowed")}
 r:=safeReport(&v2.Report{UUID:"secret-uuid",Message:"secret-message",Method:"secret-method"});b,_:=json.Marshal(r);if strings.Contains(string(b),"secret-"){t.Fatal("unsafe DTO")}
 l:=newLimiter();for i:=0;i<3;i++{if !l.allow("test",3){t.Fatal("early rate limit")}};if l.allow("test",3){t.Fatal("rate limit ignored")}
}
