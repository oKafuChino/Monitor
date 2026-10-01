package share

import (
 "archive/zip"
 "bytes"
 "fmt"
 "io"
 "mime"
 "path"
 "strings"
 "github.com/gin-gonic/gin"
 "github.com/komari-monitor/komari/web/public"
)

func loadAssets() (map[string][]byte,error) {
 archive,err:=public.ShareArchive();if err!=nil { return nil,err }
 z,err:=zip.NewReader(bytes.NewReader(archive),int64(len(archive)));if err!=nil { return nil,err }
 out:=make(map[string][]byte)
 for _,f:=range z.File {
  if f.Name!="index.html" && !(strings.HasPrefix(f.Name,"assets/") && path.Base(f.Name)==strings.TrimPrefix(f.Name,"assets/") && (strings.HasSuffix(f.Name,".js") || strings.HasSuffix(f.Name,".css"))) { return nil,fmt.Errorf("unexpected share asset") }
  r,err:=f.Open();if err!=nil{return nil,err};data,err:=io.ReadAll(io.LimitReader(r,10<<20));r.Close();if err!=nil{return nil,err};out[f.Name]=data
 }
 if len(out["index.html"])==0 { return nil,fmt.Errorf("share frontend missing") };return out,nil
}
func serveAsset(c *gin.Context,assets map[string][]byte) {
 name:="assets/"+c.Param("name");data,ok:=assets[name];if !ok {c.Status(404);return};c.Data(200,mime.TypeByExtension(path.Ext(name)),data)
}
