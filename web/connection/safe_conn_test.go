package connection

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// wsEchoServer upgrades every request and echoes each frame back.
func wsEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	var upgrader websocket.Upgrader
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}))
}

func dialEcho(t *testing.T, server *httptest.Server) *SafeConn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewSafeConn(conn)
}

func TestSafeConnNoInterceptorZeroCost(t *testing.T) {
	server := wsEchoServer(t)
	defer server.Close()
	sc := dialEcho(t, server)
	defer sc.Close()

	if err := sc.WriteMessage(websocket.TextMessage, []byte("plain")); err != nil {
		t.Fatal(err)
	}
	_, data, err := sc.ReadMessage()
	if err != nil || string(data) != "plain" {
		t.Fatalf("read = %q err %v", data, err)
	}
}

func TestSafeConnConcurrentWrites(t *testing.T) {
	server := wsEchoServer(t)
	defer server.Close()
	sc := dialEcho(t, server)
	defer sc.Close()
	_ = sc.SetReadDeadline(time.Now().Add(5 * time.Second))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := sc.WriteJSON(map[string]int{"n": n}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	seen := map[int]bool{}
	for i := 0; i < 20; i++ {
		var value map[string]int
		if err := sc.ReadJSON(&value); err != nil {
			t.Fatal(err)
		}
		seen[value["n"]] = true
	}
	if len(seen) != 20 {
		t.Fatalf("missing frames: %v", seen)
	}
}
