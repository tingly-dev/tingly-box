// echo-tool is the smallest possible Peer: an external tool that treats
// tingly-box as an IM platform (.design/peer.md). It speaks the whole
// protocol — two verbs, standard library only:
//
//	POST /api/v1/peers/{id}/send      {text, reply_to_update_id?}
//	GET  /api/v1/peers/{id}/updates   ?offset=&timeout=   (long-poll)
//
// It announces itself, then loops on getUpdates Telegram-style — the offset
// of each poll confirms the batch before it — and echoes every message back,
// threaded to the sender's bubble. Crash it anywhere: unconfirmed updates
// replay on the next start.
//
// Setup (once, with your operator token):
//
//	curl -s -X POST http://127.0.0.1:12580/api/v1/peers \
//	  -H "Authorization: Bearer $TB_USER_TOKEN" \
//	  -d '{"name":"echo","bot_uuid":"<bot-uuid>","chat_id":"<chat-id>"}'
//	# → {"peer":{"uuid":"…"},"token":"tb-peer-…"}   token is shown once
//
// Run:
//
//	go run ./remote/peer/examples/echo-tool \
//	  -peer <peer-uuid> -token tb-peer-… [-base http://127.0.0.1:12580]
//
// Then chat with it: @echo hello → it echoes back. /peers shows it online.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type update struct {
	UpdateID int64  `json:"update_id"`
	Type     string `json:"type"`
	SenderID string `json:"sender_id"`
	Text     string `json:"text"`
}

type client struct {
	base, peer, token string
	http              *http.Client
}

// send delivers one message into the peer's bound chat; replyTo threads it
// to an inbound update when > 0.
func (c *client) send(ctx context.Context, text string, replyTo int64) error {
	body, _ := json.Marshal(map[string]any{"text": text, "reply_to_update_id": replyTo})
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/peers/%s/send", c.base, c.peer), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send: HTTP %d", resp.StatusCode)
	}
	return nil
}

// updates long-polls the inbound stream. Passing offset=N confirms every
// update with id < N (the getUpdates idiom: next call passes last_id+1), so
// a tool that crashes before processing a batch sees it again.
func (c *client) updates(ctx context.Context, offset int64) ([]update, error) {
	url := fmt.Sprintf("%s/api/v1/peers/%s/updates?timeout=25s&offset=%d", c.base, c.peer, offset)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updates: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Updates []update `json:"updates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Updates, nil
}

func main() {
	base := flag.String("base", "http://127.0.0.1:12580", "tingly-box base URL")
	peer := flag.String("peer", os.Getenv("TB_PEER_ID"), "peer UUID (or env TB_PEER_ID)")
	token := flag.String("token", os.Getenv("TB_PEER_TOKEN"), "tb-peer- token (or env TB_PEER_TOKEN)")
	flag.Parse()
	if *peer == "" || *token == "" {
		log.Fatal("need -peer and -token (create them via POST /api/v1/peers, see file header)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The long-poll client must out-wait the server's 25s park.
	c := &client{base: *base, peer: *peer, token: *token, http: &http.Client{Timeout: 40 * time.Second}}

	if err := c.send(ctx, "echo-tool online — say something and I'll say it back", 0); err != nil {
		log.Printf("announce failed (bot not running yet?): %v", err)
	}

	// The getUpdates loop. offset 0 on the first call re-reads whatever a
	// previous run left unconfirmed.
	var offset int64
	for ctx.Err() == nil {
		batch, err := c.updates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("poll: %v (retrying)", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, u := range batch {
			// The envelope is typed; skip kinds this tool doesn't know.
			if u.Type == "message" {
				log.Printf("[%d] %s: %q", u.UpdateID, u.SenderID, u.Text)
				if err := c.send(ctx, "echo: "+u.Text, u.UpdateID); err != nil {
					log.Printf("send: %v", err)
				}
			}
			// Only advance past what we actually processed — this is the
			// crash-safety line: confirm happens on the NEXT poll.
			offset = u.UpdateID + 1
		}
	}
	log.Println("bye")
}
