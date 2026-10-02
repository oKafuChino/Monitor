package sharing

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "errors"
 "fmt"
 "net"
 "net/url"
 "strconv"
 "sync"
 "time"

 "github.com/google/uuid"
 "github.com/komari-monitor/komari/database/models"
 "gorm.io/gorm"
)

var ErrUnavailable = errors.New("share unavailable")
const MaxActivePerNode = 20
const MaxActiveGlobal = 1000
const MaxSessionsPerLink = 100
var configMu sync.RWMutex
var publicBase string
var publicBaseSource func() string

// URLs come from administrator configuration, never request headers.
func Configure(base string) { configMu.Lock(); publicBase = base; publicBaseSource = nil; configMu.Unlock() }
// ConfigureSource reads persisted settings for each request. A save takes
// effect immediately without replacing either HTTP listener.
func ConfigureSource(source func() string) { configMu.Lock(); publicBaseSource = source; publicBase = ""; configMu.Unlock() }
func PublicBase() string {
 configMu.RLock(); source, base := publicBaseSource, publicBase; configMu.RUnlock()
 if source != nil { return source() }; return base
}

func ValidateConfig(main, listen, base string) (*url.URL, error) {
 if listen == "" { if base != "" { return nil, errors.New("share URL requires share listener") }; return nil, nil }
 host, port, err := net.SplitHostPort(listen)
 if err != nil || host == "" { return nil, errors.New("share listener requires explicit host and port") }
 n, err := strconv.Atoi(port)
 if err != nil || n < 1 || n > 65535 { return nil, errors.New("invalid share port") }
 _, mainPort, err := net.SplitHostPort(main)
 mainNumber, portErr := strconv.Atoi(mainPort)
 if err != nil || portErr != nil || n == mainNumber { return nil, errors.New("main and share ports must differ") }
 if base == "" { return nil, nil }
 return ValidatePublicBase(base)
}

func ValidatePublicBase(base string) (*url.URL,error) {
 u, err := url.Parse(base)
 if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
  return nil, errors.New("share public URL must be an origin without path, credentials, query or fragment")
 }
 local := u.Hostname() == "localhost"
 if u.Port() != "" { publicPort, err := strconv.Atoi(u.Port()); if err != nil || publicPort < 1 || publicPort > 65535 { return nil, errors.New("invalid share public port") } }
 if ip := net.ParseIP(u.Hostname()); ip != nil { local = ip.IsLoopback() }
 if u.Scheme != "https" && !(u.Scheme == "http" && local) { return nil, errors.New("share URL requires HTTPS (HTTP allowed only on loopback)") }
 return u, nil
}

func Expiry(now time.Time, duration string) (*time.Time, error) {
 now = now.UTC()
 var end time.Time
 switch duration {
 case "1d": end = now.Add(24*time.Hour)
 case "1w": end = now.Add(7*24*time.Hour)
 case "1mo":
  first := time.Date(now.Year(), now.Month()+1, 1, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
  day := now.Day(); last := time.Date(first.Year(), first.Month()+1, 0, 0,0,0,0,time.UTC).Day()
  if day > last { day = last }; end = first.AddDate(0,0,day-1)
 case "forever": return nil, nil
 default: return nil, errors.New("invalid share duration")
 }
 return &end, nil
}

func Digest(token string) string { sum := sha256.Sum256([]byte(token)); return hex.EncodeToString(sum[:]) }
func randomToken() (string, error) { var b [32]byte; if _, err := rand.Read(b[:]); err != nil { return "", err }; return base64.RawURLEncoding.EncodeToString(b[:]), nil }
type Service struct { DB *gorm.DB; generateToken func() (string,error) }
func New(db *gorm.DB) *Service { return &Service{DB:db} }
func (s *Service) token() (string,error) { if s.generateToken != nil { return s.generateToken() }; return randomToken() }

func (s *Service) Create(node, duration, actor string) (models.ShareLink, string, error) {
 base := PublicBase()
 if base == "" { return models.ShareLink{}, "", errors.New("enable share listener and configure share public URL in site settings") }
 if _, err := ValidatePublicBase(base); err != nil { return models.ShareLink{}, "", err }
 now := time.Now().UTC(); expiry, err := Expiry(now, duration)
 if err != nil { return models.ShareLink{}, "", err }
 var link models.ShareLink; var token string
 err = s.DB.Transaction(func(tx *gorm.DB) error {
  var client models.Client
  if err := tx.Where("uuid = ? AND hidden = ?", node, false).First(&client).Error; err != nil { return ErrUnavailable }
  active := tx.Model(&models.ShareLink{}).Where("revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", now)
  var count int64
  if err := active.Session(&gorm.Session{}).Count(&count).Error; err != nil { return err }; if count >= MaxActiveGlobal { return errors.New("active share limit reached") }
  if err := active.Session(&gorm.Session{}).Where("node_uuid = ?", node).Count(&count).Error; err != nil { return err }; if count >= MaxActivePerNode { return errors.New("node share limit reached") }
  for attempt:=0; attempt<3; attempt++ {
   token, err = s.token(); if err != nil { return err }
   hash := Digest(token)
   if err := tx.Model(&models.ShareLink{}).Where("token_hash = ?",hash).Count(&count).Error; err != nil { return err }; if count > 0 { continue }
   link = models.ShareLink{ID:uuid.NewString(), TokenHash:hash, TokenMask:token[:4]+"…"+token[len(token)-4:], NodeUUID:node, Duration:duration, ExpiresAt:expiry, CreatedBy:actor, CreatedAt:now, UpdatedAt:now}
   return tx.Create(&link).Error
  }; return errors.New("unable to allocate share credential")
 })
 if err != nil { return models.ShareLink{}, "", err }
 return link, base+"/s/"+token, nil
}

func valid(link models.ShareLink, now time.Time) bool { return link.RevokedAt == nil && (link.ExpiresAt == nil || now.Before(*link.ExpiresAt)) }
func (s *Service) node(link models.ShareLink, now time.Time) (models.Client, error) {
 var node models.Client
 if !valid(link,now) { return node, ErrUnavailable }
 if err := s.DB.Where("uuid = ? AND hidden = ?",link.NodeUUID,false).First(&node).Error; err != nil { return node, ErrUnavailable }
 return node,nil
}

func (s *Service) Exchange(token string) (string, time.Time, error) {
 var expiry time.Time; var credential string
 if len(token) != 43 { return "", expiry, ErrUnavailable }
 err := s.DB.Transaction(func(tx *gorm.DB) error {
  service := New(tx); now := time.Now().UTC(); var link models.ShareLink
  if err := tx.Where("token_hash = ?",Digest(token)).First(&link).Error; err != nil { return ErrUnavailable }
  if _, err := service.node(link,now); err != nil { return err }
  if err := tx.Where("expires_at <= ?",now).Delete(&models.ShareSession{}).Error; err != nil { return err }
  var count int64
  if err := tx.Model(&models.ShareSession{}).Where("share_link_id = ?",link.ID).Count(&count).Error; err != nil { return err }
  if count >= MaxSessionsPerLink { return ErrUnavailable }
  expiry = now.Add(24*time.Hour); if link.ExpiresAt != nil && link.ExpiresAt.Before(expiry) { expiry = *link.ExpiresAt }
  var err error; credential,err = randomToken(); if err != nil { return err }
  return tx.Create(&models.ShareSession{TokenHash:Digest(credential),ShareLinkID:link.ID,ExpiresAt:expiry}).Error
 })
 return credential, expiry, err
}

type Context struct { Link models.ShareLink; Node models.Client; SessionExpiresAt time.Time }
func (s *Service) Authenticate(credential string) (*Context,error) {
 if len(credential) != 43 { return nil, ErrUnavailable }
 now:=time.Now().UTC(); var session models.ShareSession; var link models.ShareLink
 if err:=s.DB.Where("token_hash = ? AND expires_at > ?",Digest(credential),now).First(&session).Error; err != nil { return nil,ErrUnavailable }
 if err:=s.DB.Where("id = ?",session.ShareLinkID).First(&link).Error; err != nil { return nil,ErrUnavailable }
 node,err:=s.node(link,now); if err != nil { return nil,err }
 return &Context{Link:link,Node:node,SessionExpiresAt:session.ExpiresAt},nil
}

func (s *Service) Revoke(id string) error {
 return s.DB.Model(&models.ShareLink{}).Where("id = ? AND revoked_at IS NULL",id).Update("revoked_at",time.Now().UTC()).Error
}

func (s *Service) CleanupSessions() error {
 return s.DB.Where("expires_at <= ? OR share_link_id IN (SELECT id FROM share_links WHERE revoked_at IS NOT NULL)",time.Now().UTC()).Delete(&models.ShareSession{}).Error
}
func (s *Service) List(node string) ([]map[string]any,error) {
 var links []models.ShareLink
 q:=s.DB.Order("created_at DESC"); if node!="" { q=q.Where("node_uuid = ?",node) }
 if err:=q.Find(&links).Error; err!=nil { return nil,err }
 out:=make([]map[string]any,0,len(links))
 for _,l:=range links {
  var client models.Client; _=s.DB.Select("name").Where("uuid = ?",l.NodeUUID).First(&client).Error
  status:="active"; if l.RevokedAt!=nil { status="revoked" } else if !valid(l,time.Now().UTC()) { status="expired" }
  out=append(out,map[string]any{"id":l.ID,"node_uuid":l.NodeUUID,"node_name":client.Name,"token_mask":l.TokenMask,"duration":l.Duration,"expires_at":l.ExpiresAt,"revoked_at":l.RevokedAt,"created_at":l.CreatedAt,"status":status})
 }
 return out,nil
}

func LocalID(linkID, kind, id string) string { return Digest(fmt.Sprintf("%s:%s:%s",linkID,kind,id))[:24] }
