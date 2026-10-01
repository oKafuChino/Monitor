package connection

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

// SafeConn serializes data and control frame writes on an authenticated socket.
type SafeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
	ID   int64
}

func NewSafeConn(conn *websocket.Conn) *SafeConn {
	return &SafeConn{conn: conn, ID: time.Now().UnixNano()}
}
func (sc *SafeConn) WriteMessage(kind int, data []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn.WriteMessage(kind, data)
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
func (sc *SafeConn) ReadMessage() (int, []byte, error) { return sc.conn.ReadMessage() }
func (sc *SafeConn) ReadJSON(v interface{}) error {
	_, data, err := sc.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
func (sc *SafeConn) Close() error                              { sc.mu.Lock(); defer sc.mu.Unlock(); return sc.conn.Close() }
func (sc *SafeConn) SetReadDeadline(t time.Time) error         { return sc.conn.SetReadDeadline(t) }
func (sc *SafeConn) SetPingHandler(h func(string) error)       { sc.conn.SetPingHandler(h) }
func (sc *SafeConn) GetConn() *websocket.Conn                  { sc.mu.Lock(); defer sc.mu.Unlock(); return sc.conn }
func (sc *SafeConn) SetCloseHandler(h func(int, string) error) { sc.conn.SetCloseHandler(h) }
