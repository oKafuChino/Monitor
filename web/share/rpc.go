package share

import (
 "bytes"
 "context"
 "encoding/json"
	"sort"
 "io"
 "net/http"
 "strconv"
 "strings"
 "time"

 "github.com/gin-gonic/gin"
 "github.com/komari-monitor/komari/database/models"
 "github.com/komari-monitor/komari/internal/metricstore"
 "github.com/komari-monitor/komari/internal/sharing"
 "github.com/komari-monitor/komari/pkg/metric"
 "github.com/komari-monitor/komari/pkg/rpc"
 v2 "github.com/komari-monitor/komari/protocol/v2"
 "github.com/komari-monitor/komari/web/agent"
)

// A share dispatcher does not consult the extensible main-site registry.
var safeMetrics=map[string]string{
 "cpu.usage":"%","gpu.usage":"%","gpu.device.usage":"%","gpu.memory.used":"bytes","gpu.memory.total":"bytes","gpu.temperature":"°C",
 "memory.used":"bytes","swap.used":"bytes","disk.used":"bytes","load.average":"",
 "disk.io.read.rate":"bytes/s","disk.io.write.rate":"bytes/s","net.in.rate":"bytes/s","net.out.rate":"bytes/s",
 "net.total.up":"bytes","net.total.down":"bytes","process.count":"","connections.tcp":"","connections.udp":"",
 "ping.latency_ms":"ms","ping.loss":"%",
}
type queryParams struct {
 MetricKeys []string `json:"metric_keys"`
 Start time.Time `json:"start"`
 End time.Time `json:"end"`
 MaxPoints int `json:"max_points"`
	Hours int `json:"hours"`
}
func strict(data json.RawMessage,target any) error {
 if len(data)==0 {data=[]byte("{}")}
 if len(bytes.TrimSpace(data))==0 || bytes.TrimSpace(data)[0]!='{' {return io.ErrUnexpectedEOF}
 d:=json.NewDecoder(bytes.NewReader(data));d.DisallowUnknownFields();if err:=d.Decode(target);err!=nil{return err};var extra any;if d.Decode(&extra)!=io.EOF{return io.ErrUnexpectedEOF};return nil
}
func validateQuery(p *queryParams,now time.Time) bool {
 if len(p.MetricKeys)<1 || len(p.MetricKeys)>24 {return false}
 seen:=map[string]bool{};for _,key:=range p.MetricKeys {if _,ok:=safeMetrics[key];!ok || seen[key]{return false};seen[key]=true}
	if p.Hours<0 || p.Hours>720 || (p.Hours>0 && (!p.Start.IsZero() || !p.End.IsZero())) {return false}
	if p.End.IsZero(){p.End=now};if p.Start.IsZero(){hours:=p.Hours;if hours==0{hours=1};p.Start=p.End.Add(-time.Duration(hours)*time.Hour)}
 if p.MaxPoints==0{p.MaxPoints=500}
 return p.MaxPoints>0 && p.MaxPoints<=500 && p.End.After(p.Start) && !p.End.After(now) && !p.Start.Before(now.Add(-30*24*time.Hour))
}
func serveRPC(c *gin.Context,share *sharing.Context,service *sharing.Service,limits *limiter) {
 c.Request.Body=http.MaxBytesReader(c.Writer,c.Request.Body,16<<10)
 data,err:=io.ReadAll(c.Request.Body);if err!=nil{c.Status(400);return}
 var req struct {Version string `json:"jsonrpc"`; ID json.RawMessage `json:"id"`; Method string `json:"method"`; Params json.RawMessage `json:"params"`}
 if strict(data,&req)!=nil || req.Version!="2.0" || len(req.ID)==0 {c.Status(400);return}
 var id any;if json.Unmarshal(req.ID,&id)!=nil{c.Status(400);return};switch id.(type){case string,float64:default:c.Status(400);return}
 respondError:=func(code int,msg string){c.JSON(200,rpc.ErrorResponse(id,code,msg,nil))}
 var result any
	if !rpc.CheckPrincipal(rpc.NewSharePrincipal(share.Node.UUID), req.Method) { respondError(rpc.MethodNotFound,"Method not found");return }
 switch req.Method {
 case "share:getNode","share:getLatestStatus":
  var empty struct{};if strict(req.Params,&empty)!=nil{respondError(rpc.InvalidParams,"Invalid params");return}
  if req.Method=="share:getNode" {
   n:=share.Node
   tasks,err:=pingTasks(c.Request.Context(),share,service);if err!=nil{respondError(rpc.InternalError,"Data unavailable");return}
   definitions:=make([]map[string]string,0,len(safeMetrics));for key,unit:=range safeMetrics{definitions=append(definitions,map[string]string{"name":key,"unit":unit})}
   result=map[string]any{"node":map[string]any{"name":n.Name,"region":n.Region,"cpu_name":n.CpuName,"cpu_cores":n.CpuCores,"arch":n.Arch,"virtualization":n.Virtualization,"os":n.OS,"kernel_version":n.KernelVersion,"gpu_name":n.GpuName,"mem_total":n.MemTotal,"swap_total":n.SwapTotal,"disk_total":n.DiskTotal},"definitions":definitions,"ping_tasks":tasks,"share_expires_at":share.Link.ExpiresAt,"session_expires_at":share.SessionExpiresAt}
  } else {
   report,online:=agent.GetNodeSnapshot(share.Node.UUID)
   recent:=agent.GetRecentReports(share.Node.UUID);out:=make([]any,0,len(recent));for i:=range recent{out=append(out,safeReport(&recent[i]))}
   result=map[string]any{"online":online,"latest":safeReport(report),"recent":out,"share_expires_at":share.Link.ExpiresAt,"session_expires_at":share.SessionExpiresAt}
  }
 case "share:queryMetrics":
  var p queryParams;if strict(req.Params,&p)!=nil || !validateQuery(&p,time.Now().UTC()){respondError(rpc.InvalidParams,"Invalid query");return}
  if !limits.allow("history:"+share.Link.ID,12){c.Status(429);return}
  ctx,cancel:=context.WithTimeout(c.Request.Context(),10*time.Second);defer cancel()
  result,err=queryMetrics(ctx,share,service,p);if err!=nil{respondError(rpc.InternalError,"Data unavailable");return}
 default:respondError(rpc.MethodNotFound,"Method not found");return
 }
 c.JSON(200,map[string]any{"jsonrpc":"2.0","id":id,"result":result})
}

// Explicit fields prevent agent messages and future report additions leaking.
func safeReport(r *v2.Report) any {
 if r==nil{return nil}
 return map[string]any{"cpu":map[string]any{"usage":r.CPU.Usage},"ram":r.Ram,"swap":r.Swap,"disk":r.Disk,"disk_io":r.DiskIO,"load":r.Load,"network":r.Network,"connections":r.Connections,"gpu":r.GPU,"uptime":r.Uptime,"process":r.Process,"updated_at":r.UpdatedAt}
}
type pingTask struct {ID string `json:"id"`;Name string `json:"name"`; rawID string}
func pingTasks(ctx context.Context,share *sharing.Context,service *sharing.Service)([]pingTask,error){
	// SQLite JSON membership limits configuration reads to the bound node.
	var tasks []models.PingTask;if err:=service.DB.WithContext(ctx).Where("EXISTS (SELECT 1 FROM json_each(ping_tasks.clients) WHERE json_each.value = ?)",share.Node.UUID).Find(&tasks).Error;err!=nil{return nil,err}
 out:=[]pingTask{};for _,t:=range tasks{if t.AppliesToClient(share.Node.UUID){raw:=strconv.FormatUint(uint64(t.Id),10);out=append(out,pingTask{ID:sharing.LocalID(share.Link.ID,"ping",raw),Name:t.Name,rawID:raw})}}
 return out,nil
}
type chartPoint struct {Time time.Time `json:"time"`;Value *float64 `json:"value"`}
type chartSeries struct {MetricKey string `json:"metric_key"`;ID string `json:"id"`;Name string `json:"name"`;Unit string `json:"unit"`;Points []chartPoint `json:"points"`}
func queryMetrics(ctx context.Context,share *sharing.Context,service *sharing.Service,p queryParams)(any,error){
 store:=metricstore.GetStore();if store==nil{return nil,sharing.ErrUnavailable}
 tasks,err:=pingTasks(ctx,share,service);if err!=nil{return nil,err};taskMap:=map[string]pingTask{};for _,t:=range tasks{taskMap[t.rawID]=t}
 series:=[]chartSeries{}
 for _,key:=range p.MetricKeys {
	  def,err:=store.GetMetric(ctx,key);if err!=nil{return nil,err}
	  start:=p.Start
	  if def.RetentionDays<=0 { continue }
	  cutoff:=time.Now().UTC().Add(-time.Duration(def.RetentionDays)*24*time.Hour);if start.Before(cutoff){start=cutoff};if !p.End.After(start){continue}
  interval:=time.Duration((p.End.Sub(p.Start)+time.Duration(p.MaxPoints-1))/time.Duration(p.MaxPoints));if interval<time.Second{interval=time.Second};interval=store.CompatibleSeriesInterval(p.Start,time.Now().UTC(),interval)
	  points,err:=store.Series(ctx,metric.AggregateQuery{Query:metric.Query{MetricName:key,EntityID:share.Node.UUID,Start:start,End:p.End,Order:metric.OrderAsc},Aggregation:metric.AggAvg,Interval:interval,PreserveSeries:true},time.Now().UTC());if err!=nil{return nil,err}
  groups:=map[string]*chartSeries{}
  for _,point:=range points {
   id:="node";name:=key
   if strings.HasPrefix(key,"ping."){t,ok:=taskMap[point.Tags["task_id"]];if !ok{continue};id=t.ID;name=t.Name}
   if strings.HasPrefix(key,"gpu.") && key!="gpu.usage"{id=sharing.LocalID(share.Link.ID,"gpu",point.Tags["device_index"]);name="GPU "+point.Tags["device_index"]}
   item:=groups[id];if item==nil{if len(groups)>=64{continue};item=&chartSeries{MetricKey:key,ID:id,Name:name,Unit:safeMetrics[key],Points:[]chartPoint{}};groups[id]=item}
   value:=point.Value;var v *float64=&value;if key=="ping.latency_ms" && value<0{v=nil};item.Points=append(item.Points,chartPoint{Time:point.Bucket,Value:v})
  }
	  for _,item:=range groups {
	   sort.Slice(item.Points,func(i,j int)bool{return item.Points[i].Time.Before(item.Points[j].Time)})
	   // Insert null boundaries for unavailable samples, especially disk I/O.
	   withGaps:=make([]chartPoint,0,len(item.Points));for i,point:=range item.Points{if i>0 && point.Time.Sub(item.Points[i-1].Time)>interval*2 {withGaps=append(withGaps,chartPoint{Time:item.Points[i-1].Time.Add(interval),Value:nil})};withGaps=append(withGaps,point)};item.Points=withGaps
	   if len(item.Points)>p.MaxPoints{ // Keep bounded output without erasing valid zero or missing samples.
   sampled:=make([]chartPoint,0,p.MaxPoints);for i:=0;i<p.MaxPoints;i++{sampled=append(sampled,item.Points[i*len(item.Points)/p.MaxPoints])};item.Points=sampled
  };series=append(series,*item)}
 }
 return map[string]any{"series":series},nil
}
