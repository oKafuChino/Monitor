package jsonrpc

import (
 "context"
 "github.com/komari-monitor/komari/database/dbcore"
 "github.com/komari-monitor/komari/database/auditlog"
 "github.com/komari-monitor/komari/internal/sharing"
 "github.com/komari-monitor/komari/pkg/rpc"
)

func init() {
 RegisterWithGroupAndMeta("createShareLink",rpc.RoleAdmin,adminCreateShareLink,&rpc.MethodMeta{Name:"admin:createShareLink",Summary:"Share one visible node"})
 RegisterWithGroupAndMeta("listShareLinks",rpc.RoleAdmin,adminListShareLinks,&rpc.MethodMeta{Name:"admin:listShareLinks",Summary:"List masked node shares"})
 RegisterWithGroupAndMeta("revokeShareLink",rpc.RoleAdmin,adminRevokeShareLink,&rpc.MethodMeta{Name:"admin:revokeShareLink",Summary:"Revoke a node share"})
}
func adminCreateShareLink(ctx context.Context, req *rpc.JsonRpcRequest) (any,*rpc.JsonRpcError) {
 var p struct { UUID string `json:"uuid"`; Duration string `json:"duration"` }
 if err:=req.BindParams(&p);err!=nil || p.UUID=="" { return nil,rpc.MakeError(rpc.InvalidParams,"Invalid params",nil) }
 actor,ip:=auditActor(ctx)
 link,url,err:=sharing.New(dbcore.GetDBInstance()).Create(p.UUID,p.Duration,actor)
 if err!=nil { return nil,rpc.MakeError(rpc.InvalidParams,err.Error(),nil) }
 auditlog.Log(ip,actor,"create node share:"+link.ID,"info")
 return map[string]any{"id":link.ID,"url":url,"duration":link.Duration,"expires_at":link.ExpiresAt},nil
}
func adminListShareLinks(_ context.Context, req *rpc.JsonRpcRequest) (any,*rpc.JsonRpcError) {
 var p struct { UUID string `json:"uuid"` }; if err:=req.BindParams(&p);err!=nil { return nil,rpc.MakeError(rpc.InvalidParams,"Invalid params",nil) }
 result,err:=sharing.New(dbcore.GetDBInstance()).List(p.UUID)
 if err!=nil { return nil,rpc.MakeError(rpc.InternalError,"Unable to list shares",nil) }; return result,nil
}
func adminRevokeShareLink(ctx context.Context, req *rpc.JsonRpcRequest) (any,*rpc.JsonRpcError) {
 var p struct { ID string `json:"id"` }; if err:=req.BindParams(&p);err!=nil || p.ID=="" { return nil,rpc.MakeError(rpc.InvalidParams,"Invalid params",nil) }
 if err:=sharing.New(dbcore.GetDBInstance()).Revoke(p.ID);err!=nil { return nil,rpc.MakeError(rpc.InternalError,"Unable to revoke share",nil) }
 actor,ip:=auditActor(ctx);auditlog.Log(ip,actor,"revoke node share:"+p.ID,"info");return nil,nil
}
