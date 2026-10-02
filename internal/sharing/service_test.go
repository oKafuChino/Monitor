package sharing

import (
 "encoding/json"
 "path/filepath"
 "strings"
 "testing"
 "time"
 "github.com/komari-monitor/komari/database/models"
 "gorm.io/driver/sqlite"
 "gorm.io/gorm"
)
func testService(t *testing.T) *Service {
 t.Helper();db,err:=gorm.Open(sqlite.Open(filepath.Join(t.TempDir(),"shares.db")),&gorm.Config{});if err!=nil{t.Fatal(err)}
 sqlDB,err:=db.DB();if err!=nil{t.Fatal(err)};sqlDB.SetMaxOpenConns(1);t.Cleanup(func(){sqlDB.Close();Configure("")})
 if err:=db.AutoMigrate(&models.Client{},&models.ShareLink{},&models.ShareSession{});err!=nil{t.Fatal(err)}
 for _,node:=range []models.Client{{UUID:"a",Name:"Node A",Token:"agent-a"},{UUID:"b",Name:"Node B",Token:"agent-b"},{UUID:"hidden",Name:"Hidden",Token:"agent-h",Hidden:true}}{if err:=db.Create(&node).Error;err!=nil{t.Fatal(err)}}
 Configure("https://share.example.com");return New(db)
}
func TestExpiryCalendarAndUTC(t *testing.T){
 for _,tc:=range []struct{in,want string}{{"2024-01-31T12:30:00Z","2024-02-29T12:30:00Z"},{"2025-01-31T12:30:00Z","2025-02-28T12:30:00Z"},{"2025-12-31T12:30:00Z","2026-01-31T12:30:00Z"},{"2026-05-31T12:30:00Z","2026-06-30T12:30:00Z"}}{now,_:=time.Parse(time.RFC3339,tc.in);got,err:=Expiry(now,"1mo");if err!=nil||got.Format(time.RFC3339)!=tc.want{t.Fatalf("expiry %s: %v %v",tc.in,got,err)}}
 now:=time.Date(2026,1,2,8,0,0,0,time.FixedZone("HK",8*3600))
 for d,hours:=range map[string]int{"1d":24,"1w":168}{got,err:=Expiry(now,d);if err!=nil||got.Sub(now)!=time.Duration(hours)*time.Hour||got.Location()!=time.UTC{t.Fatal("bad UTC expiry")}}
 if got,err:=Expiry(now,"forever");err!=nil||got!=nil{t.Fatal("forever")};if _,err:=Expiry(now,"30d");err==nil{t.Fatal("accepted arbitrary duration")}
}
func TestSharePersistenceBindingAndRevocation(t *testing.T){
 s:=testService(t);l,url,err:=s.Create("a","forever","admin");if err!=nil{t.Fatal(err)};token:=strings.TrimPrefix(url,PublicBase()+"/s/")
 if len(token)!=43||l.TokenHash!=Digest(token)||strings.Contains(l.TokenMask,token){t.Fatal("unsafe credential")}
 body,_:=json.Marshal(l);if strings.Contains(string(body),l.TokenHash)||strings.Contains(string(body),token){t.Fatal("digest exposed")}
 credential,expiry,err:=s.Exchange(token);if err!=nil{t.Fatal(err)};if credential==token||time.Until(expiry)>24*time.Hour{t.Fatal("unsafe session")}
 c,err:=New(s.DB).Authenticate(credential);if err!=nil||c.Node.UUID!="a"{t.Fatalf("binding lost: %v",err)}
 _,url2,err:=s.Create("a","1d","admin");if err!=nil{t.Fatal(err)};other,_,err:=s.Exchange(strings.TrimPrefix(url2,PublicBase()+"/s/"));if err!=nil{t.Fatal(err)}
 if err:=s.Revoke(l.ID);err!=nil{t.Fatal(err)};if err:=s.Revoke(l.ID);err!=nil{t.Fatal(err)}
 if _,err:=s.Authenticate(credential);err==nil{t.Fatal("revoked session valid")};if _,_,err:=s.Exchange(token);err==nil{t.Fatal("revoked token valid")};if _,err:=s.Authenticate(other);err!=nil{t.Fatal("unrelated share revoked")}
 for _,id:=range []string{"hidden","missing"}{if _,_,err:=s.Create(id,"1d","admin");err==nil{t.Fatal("shared hidden/missing node")}}
 if err:=s.DB.Transaction(func(tx *gorm.DB)error{if err:=models.RevokeNodeShares(tx,"a");err!=nil{return err};return tx.Model(&models.Client{}).Where("uuid = ?","a").Update("hidden",true).Error});err!=nil{t.Fatal(err)}
 s.DB.Model(&models.Client{}).Where("uuid = ?","a").Update("hidden",false)
 if _,err:=s.Authenticate(other);err==nil{t.Fatal("unhide resurrected share")}
}
func TestExpiryAndSessionLimits(t *testing.T){
 s:=testService(t);l,url,err:=s.Create("a","1d","admin");if err!=nil{t.Fatal(err)};token:=strings.TrimPrefix(url,PublicBase()+"/s/")
 credential,_,err:=s.Exchange(token);if err!=nil{t.Fatal(err)};past:=time.Now().UTC().Add(-time.Second)
 s.DB.Model(&models.ShareLink{}).Where("id = ?",l.ID).Update("expires_at",past)
 if _,err:=s.Authenticate(credential);err==nil{t.Fatal("expired link valid")};if valid(models.ShareLink{ExpiresAt:&past},past){t.Fatal("boundary valid")}
 s.DB.Model(&models.ShareLink{}).Where("id = ?",l.ID).Update("expires_at",nil)
 for i:=1;i<MaxSessionsPerLink;i++{if _,_,err:=s.Exchange(token);err!=nil{t.Fatal(err)}}
 if _,_,err:=s.Exchange(token);err==nil{t.Fatal("session cap ignored")}
 if err:=s.DB.Delete(&models.Client{},"uuid = ?","a").Error;err!=nil{t.Fatal(err)};if _,err:=s.Authenticate(credential);err==nil{t.Fatal("deleted node valid")}
}
func TestConfigValidation(t *testing.T){
 for _,tc:=range []struct{listen,base string}{{":25775","https://share.example.com"},{"127.0.0.1:25774","https://share.example.com"},{"127.0.0.1:0","https://share.example.com"},{"127.0.0.1:25775","http://share.example.com"},{"127.0.0.1:25775","https://share.example.com/path"},{"127.0.0.1:25775","https://share.example.com?x=1"},{"127.0.0.1:25775","https://user@share.example.com"}}{if _,err:=ValidateConfig("127.0.0.1:25774",tc.listen,tc.base);err==nil{t.Fatalf("accepted %#v",tc)}}
 if _,err:=ValidateConfig("127.0.0.1:25774","127.0.0.1:25775","http://localhost:25775");err!=nil{t.Fatal(err)}
 if u,err:=ValidateConfig("127.0.0.1:25774","","");err!=nil||u!=nil{t.Fatal("disabled listener")}
}

func TestTokenCollisionRetryAndNodeLimit(t *testing.T){
 s:=testService(t);first:=strings.Repeat("a",43);second:=strings.Repeat("b",43)
 s.generateToken=func()(string,error){return first,nil}
 if _,_,err:=s.Create("a","forever","admin");err!=nil{t.Fatal(err)}
 attempts:=0;s.generateToken=func()(string,error){attempts++;if attempts==1{return first,nil};return second,nil}
 if _,_,err:=s.Create("a","forever","admin");err!=nil||attempts!=2{t.Fatalf("collision retry: %d %v",attempts,err)}
 s.generateToken=nil
 for i:=2;i<MaxActivePerNode;i++{if _,_,err:=s.Create("a","forever","admin");err!=nil{t.Fatal(err)}}
 if _,_,err:=s.Create("a","forever","admin");err==nil{t.Fatal("node active cap ignored")}
 if _,_,err:=s.Create("b","forever","admin");err!=nil{t.Fatal("node cap affected another node",err)}
}

func TestPublicBaseChangesWithoutRestart(t *testing.T) {
 s := testService(t)
 base := "https://first.example.com"
 ConfigureSource(func() string { return base })
 _, first, err := s.Create("a", "1d", "admin")
 if err != nil || !strings.HasPrefix(first,base+"/s/") { t.Fatalf("first URL: %s %v",first,err) }
 base = "https://second.example.com"
 _, second, err := s.Create("a", "1d", "admin")
 if err != nil || !strings.HasPrefix(second,base+"/s/") { t.Fatalf("updated URL: %s %v",second,err) }
 base = ""
 if _,_,err := s.Create("a","1d","admin"); err == nil { t.Fatal("creation allowed without origin") }
 if _,err := ValidateConfig("127.0.0.1:25774","127.0.0.1:25775",""); err != nil { t.Fatal("listener cannot start before admin configuration",err) }
}
