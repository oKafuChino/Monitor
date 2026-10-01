package server

import (
 "context"
 "errors"
 "net"
 "net/http"
 "testing"
 "time"
 "github.com/gin-gonic/gin"
 "github.com/komari-monitor/komari/internal/lifecycle"
)
func freeAddress(t *testing.T) string {
 t.Helper();l,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)};addr:=l.Addr().String();l.Close();return addr
}
func TestSharePortConflictReleasesMain(t *testing.T){
 blocker,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)};defer blocker.Close()
 main:=freeAddress(t);app:=New(Options{ListenAddr:main,ShareListen:blocker.Addr().String()});app.engine=gin.New();app.shareEngine=gin.New()
 cleaned:=false;app.addCleanup("test",func(context.Context)error{cleaned=true;return nil})
 if err:=app.Run();err==nil{t.Fatal("conflict accepted")};if !cleaned{t.Fatal("resources retained")}
 l,err:=net.Listen("tcp",main);if err!=nil{t.Fatal("main port retained",err)};l.Close()
}
func TestMainPortConflictNeverStartsShare(t *testing.T){
 blocker,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)};defer blocker.Close()
 addr:=freeAddress(t);app:=New(Options{ListenAddr:blocker.Addr().String(),ShareListen:addr});app.engine=gin.New();app.shareEngine=gin.New()
 if err:=app.Run();err==nil{t.Fatal("conflict accepted")};l,err:=net.Listen("tcp",addr);if err!=nil{t.Fatal("share port started",err)};l.Close()
}
func TestDualListenersCloseOnRestart(t *testing.T){
 main,share:=freeAddress(t),freeAddress(t)
 app:=New(Options{ListenAddr:main,ShareListen:share});app.engine=gin.New();app.shareEngine=gin.New()
 app.engine.GET("/ping",func(c *gin.Context){c.Status(204)});app.shareEngine.GET("/healthz",func(c *gin.Context){c.Status(204)})
 done:=make(chan error,1);go func(){done<-app.Run()}()
 client:=&http.Client{Timeout:100*time.Millisecond}
 ready:=func(addr,path string)bool{r,err:=client.Get("http://"+addr+path);if err!=nil{return false};r.Body.Close();return r.StatusCode==204}
 deadline:=time.Now().Add(3*time.Second);for time.Now().Before(deadline){if ready(main,"/ping")&&ready(share,"/healthz"){break};time.Sleep(10*time.Millisecond)}
 if !ready(main,"/ping")||!ready(share,"/healthz"){lifecycle.RequestRestart("test");t.Fatal("both listeners not ready")}
 lifecycle.RequestRestart("test")
 select{case err:=<-done:if !errors.Is(err,ErrRestartRequested){t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("shutdown stalled")}
 for _,addr:=range []string{main,share}{l,err:=net.Listen("tcp",addr);if err!=nil{t.Fatal("port retained",err)};l.Close()}
}
