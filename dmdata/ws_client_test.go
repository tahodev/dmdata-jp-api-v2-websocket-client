package dmdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// gzip圧縮+base64のXML電文と非圧縮電文を順に1通ずつ送るテスト用WebSocketサーバー。
func newTestServer(t *testing.T, messages [][]byte) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer c.Close()
		for _, m := range messages {
			if err := c.WriteMessage(websocket.TextMessage, m); err != nil {
				return
			}
		}
		// クライアントが読み終わるまで少し待ってから閉じる
		time.Sleep(100 * time.Millisecond)
	}))
}

func gzipBase64(t *testing.T, s string) string {
	t.Helper()
	buf := bytes.Buffer{}
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// 実際の電文メッセージはトップレベルに "type":"data" を持つ。
type wireData struct {
	Type string `json:"type"`
	Data
}

func dataMessage(t *testing.T, compression, encoding, body string) []byte {
	t.Helper()
	b, err := json.Marshal(wireData{
		Type: "data",
		Data: Data{
			Head:        Head{Type: "XG44", Time: "2026-09-28T00:00:00Z"},
			Format:      "xml",
			Compression: compression,
			Encoding:    encoding,
			Body:        body,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStartWebSocketReceivesGzipAndPlainXML(t *testing.T) {
	xmlGzip := "<Report><Head><Title>gzip電文</Title></Head></Report>"
	xmlPlain := "<Report><Head><Title>非圧縮電文</Title></Head></Report>"
	server := newTestServer(t, [][]byte{
		dataMessage(t, "gzip", "base64", gzipBase64(t, xmlGzip)),
		dataMessage(t, "", "utf-8", xmlPlain),
	})
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	got := []string{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	StartWebSocket(ctx, url, func(head Head, xml string) {
		if head.Type != "XG44" {
			t.Errorf("head.Type = %q, want XG44", head.Type)
		}
		got = append(got, xml)
	})

	if len(got) != 2 {
		t.Fatalf("callback called %d times, want 2", len(got))
	}
	if got[0] != xmlGzip {
		t.Errorf("gzip message body = %q, want %q", got[0], xmlGzip)
	}
	if got[1] != xmlPlain {
		t.Errorf("plain message body = %q, want %q", got[1], xmlPlain)
	}
}
