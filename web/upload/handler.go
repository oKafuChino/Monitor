package upload

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
	"crypto/sha256"
	"encoding/hex"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/web/api"
)

const maxChunkRequestSize = ChunkSize + 1*1024*1024

type Result struct {
	Message string
	Data    any
}

type Finalizer func(Session) (Result, error)

type Handler struct {
	Store      *Store
	Finalizers map[Purpose]Finalizer
	mu sync.Mutex
	owners map[string]*uploadOwner
}

type uploadOwner struct { identity string; expires time.Time; mu sync.Mutex }

func uploadIdentity(c *gin.Context) string {
 secret := c.GetString("session")
 if token := c.GetHeader("X-Setup-Token"); token!="" { secret = "setup:"+token }
 if key, ok := c.Get("api_key"); ok { secret,_ = key.(string) }
 if secret=="" { return "" }
 digest := sha256.Sum256([]byte(secret)); return hex.EncodeToString(digest[:])
}

// Lock each upload through storage and finalization. Waiting operations must
// recheck ownership after a merge/cancel may have removed the session.
func (h *Handler) acquire(c *gin.Context, id string) (func(), bool) {
 h.mu.Lock()
 owner,ok := h.owners[id]
 h.mu.Unlock()
 if !ok { api.RespondError(c,http.StatusNotFound,"upload not found or expired"); return nil,false }
 owner.mu.Lock()
 h.mu.Lock()
 current := h.owners[id]
 valid := current==owner && time.Now().Before(owner.expires) && owner.identity==uploadIdentity(c)
 h.mu.Unlock()
 if !valid { owner.mu.Unlock(); api.RespondError(c,http.StatusNotFound,"upload not found or expired"); return nil,false }
 return owner.mu.Unlock,true
}

func NewHandler(store *Store, finalizers map[Purpose]Finalizer) *Handler {
	return &Handler{Store: store, Finalizers: finalizers, owners:make(map[string]*uploadOwner)}
}

func (h *Handler) Init(c *gin.Context) {
	identity := uploadIdentity(c)
	if identity=="" { api.RespondError(c,http.StatusUnauthorized,"upload identity is required"); return }
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, owner := range h.owners { if !time.Now().Before(owner.expires) && owner.mu.TryLock() { _ = h.Store.Cancel(id); delete(h.owners,id); owner.mu.Unlock() } }
	if len(h.owners)>=16 { api.RespondError(c,http.StatusTooManyRequests,"too many active uploads"); return }
	var request struct {
		Purpose  Purpose `json:"purpose" binding:"required"`
		Size     int64   `json:"size" binding:"required"`
		Filename string  `json:"filename"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		api.RespondError(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if _, ok := h.Finalizers[request.Purpose]; !ok {
		api.RespondError(c, http.StatusBadRequest, "unsupported upload purpose")
		return
	}
	session, err := h.Store.Init(request.Purpose, request.Filename, request.Size)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	h.owners[session.ID] = &uploadOwner{identity:identity, expires:time.Now().Add(2*time.Hour)}
	api.RespondSuccess(c, gin.H{
		"upload_id":  session.ID,
		"chunk_size": ChunkSize,
	})
}

func (h *Handler) Chunk(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChunkRequestSize)
	uploadID := c.PostForm("upload_id")
	release, ok := h.acquire(c,uploadID); if !ok { return }; defer release()
	index, err := strconv.ParseInt(c.PostForm("chunk_index"), 10, 64)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "chunk_index must be an integer")
		return
	}
	chunk, _, err := c.Request.FormFile("chunk_data")
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, fmt.Sprintf("get chunk data: %v", err))
		return
	}
	defer chunk.Close()
	if err := h.Store.SaveChunk(uploadID, index, chunk); err != nil {
		h.respondUploadError(c, err)
		return
	}
	api.RespondSuccess(c, gin.H{"received": true, "chunk_index": index})
}

func (h *Handler) Merge(c *gin.Context) {
	var request struct {
		UploadID string `json:"upload_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		api.RespondError(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}
	release, owned := h.acquire(c,request.UploadID); if !owned { return }; defer release()
    session, err := h.Store.Merge(request.UploadID)
	if err != nil {
		_ = h.Store.Cancel(request.UploadID)
		h.respondUploadError(c, err)
		return
	}
	defer h.Store.Cancel(session.ID)
    defer func(){ h.mu.Lock(); delete(h.owners,session.ID); h.mu.Unlock() }()

	finalize, ok := h.Finalizers[session.Metadata.Purpose]
	if !ok {
		api.RespondError(c, http.StatusBadRequest, "unsupported upload purpose")
		return
	}
	result, err := finalize(session)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	api.RespondSuccessMessage(c, result.Message, result.Data)
}

func (h *Handler) Cancel(c *gin.Context) {
	var request struct {
		UploadID string `json:"upload_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		api.RespondError(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}
	release, owned := h.acquire(c,request.UploadID); if !owned { return }; defer release()
    if err := h.Store.Cancel(request.UploadID); err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	h.mu.Lock(); delete(h.owners,request.UploadID); h.mu.Unlock()
    api.RespondSuccess(c, gin.H{})
}

func (h *Handler) respondUploadError(c *gin.Context, err error) {
	if errors.Is(err, ErrNotFound) {
		api.RespondError(c, http.StatusNotFound, "upload not found or expired")
		return
	}
	api.RespondError(c, http.StatusBadRequest, err.Error())
}
