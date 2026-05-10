package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/wire"
)

// conn is one live WebSocket connection wired into the hub.
type conn struct {
	ws           *websocket.Conn
	writeTimeout time.Duration

	role   wire.Role
	botID  string
	chatID string

	writeMu sync.Mutex
	closed  bool
}

func newConn(ws *websocket.Conn, writeTimeout time.Duration) *conn {
	return &conn{ws: ws, writeTimeout: writeTimeout}
}

// write serializes WriteJSON across goroutines (the read loop drives ack +
// fanout writes; pingLoop drives ping writes).
func (c *conn) write(f wire.Frame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return websocket.ErrCloseSent
	}
	if c.writeTimeout > 0 {
		_ = c.ws.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	}
	return c.ws.WriteJSON(f)
}

func (c *conn) writeAck(reqID, messageID string) error {
	data, _ := wire.EncodeData(wire.Ack{MessageID: messageID, Timestamp: time.Now().Unix()})
	return c.write(wire.Frame{Kind: wire.KindAck, ID: reqID, Bot: c.botID, Chat: c.chatID, Data: data})
}

func (c *conn) writeError(reqID, code, msg string) error {
	data, _ := wire.EncodeData(wire.ErrorPayload{Code: code, Message: msg})
	return c.write(wire.Frame{Kind: wire.KindError, ID: reqID, Bot: c.botID, Chat: c.chatID, Data: data})
}

func (c *conn) ping() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return websocket.ErrCloseSent
	}
	if c.writeTimeout > 0 {
		_ = c.ws.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	}
	return c.ws.WriteMessage(websocket.PingMessage, nil)
}

func (c *conn) close() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	_ = c.ws.Close()
}
