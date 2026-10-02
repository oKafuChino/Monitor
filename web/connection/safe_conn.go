package connection

import (
	"bytes"
	"encoding/json"
	"io"
	"errors"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

// SafeConn serializes data and control frame writes on an authenticated socket.
type SafeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
	ID   int64
	authorize func() bool
	authMu sync.RWMutex
	closeOnce sync.Once
	release func()
}

func NewSafeConn(conn *websocket.Conn) *SafeConn {
	conn.SetReadLimit(1 << 20)
	return &SafeConn{conn: conn, ID: time.Now().UnixNano()}
}
func (sc *SafeConn) WriteMessage(kind int, data []byte) error {
	sc.authMu.RLock(); authorize := sc.authorize; sc.authMu.RUnlock()
	if authorize != nil && !authorize() { _ = sc.Close(); return errors.New("authorization revoked") }
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.conn.SetWriteDeadline(time.Now().Add(10*time.Second))
	return sc.conn.WriteMessage(kind, data)
}

// Guard closes a socket when its authentication principal is revoked.
func (sc *SafeConn) Guard(authorize func() bool) func() {
	sc.authMu.Lock(); sc.authorize = authorize; sc.authMu.Unlock()
	done := make(chan struct{})
	go func() { ticker := time.NewTicker(time.Second); defer ticker.Stop(); for { select {
		case <-done: return
		case <-ticker.C: if !authorize() { _ = sc.Close(); return }
	} } }()
	return func() { close(done) }
}
func (sc *SafeConn) WriteControl(kind int, data []byte, deadline time.Time) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn.WriteControl(kind, data, deadline)
}
func (sc *SafeConn) WriteJSON(v interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}
	return sc.WriteMessage(websocket.TextMessage, buf.Bytes())
}
func (sc *SafeConn) ReadMessage() (int, []byte, error) {
	sc.conn.SetReadDeadline(time.Now().Add(90*time.Second))
	kind, reader, err := sc.conn.NextReader()
	if err != nil { return kind, nil, err }
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if len(data) > 1<<20 {
		_ = sc.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseMessageTooBig, "message too large"), time.Now().Add(time.Second))
		return kind, nil, errors.New("message too large")
	}
	return kind, data, err
}
func (sc *SafeConn) ReadJSON(v interface{}) error {
	_, data, err := sc.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
func (sc *SafeConn) ReleaseOnClose(release func()) { sc.release = release }
func (sc *SafeConn) Close() error {
	var err error
	sc.closeOnce.Do(func(){ err = sc.conn.Close(); if sc.release!=nil { sc.release() } })
	return err
}
func (sc *SafeConn) SetReadDeadline(t time.Time) error         { return sc.conn.SetReadDeadline(t) }
func (sc *SafeConn) SetPingHandler(h func(string) error)       { sc.conn.SetPingHandler(h) }
func (sc *SafeConn) GetConn() *websocket.Conn                  { sc.mu.Lock(); defer sc.mu.Unlock(); return sc.conn }
func (sc *SafeConn) SetCloseHandler(h func(int, string) error) { sc.conn.SetCloseHandler(h) }
