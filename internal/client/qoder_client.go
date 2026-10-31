package client

// Qoder's device protocol is described in
// https://github.com/Liki4/qodercli2api/blob/master/docs/inference-protocol.md.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const qoderVersion = "1.1.74"
const qoderPath = "/api/v2/service/pro/sse/agent_chat_generation"
const qoderAlphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"
const qoderPublicKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

// QoderClient uses the shared streamed-completion accumulator because Qoder
// only supplies streaming responses. Its transport implements the wire format.
type QoderClient struct{ *OpenAIClient }

func NewQoderClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*QoderClient, error) {
	if provider.OAuthDetail == nil {
		return nil, fmt.Errorf("qoder: missing OAuth credentials")
	}
	rt := &qoderTransport{inner: createSessionBoundTransport(provider, sessionID), provider: provider}
	base, err := newOpenAIClientWithTransport(provider, provider.APIBase, wrapWithLogging(rt, provider))
	if err != nil {
		return nil, err
	}
	return &QoderClient{OpenAIClient: base}, nil
}

// ChatCompletionsNew accumulates the upstream stream into a full completion.
func (c *QoderClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	req.StreamOptions.IncludeUsage = openai.Bool(true)
	stream := c.OpenAIClient.ChatCompletionsNewStreaming(ctx, req)
	if stream == nil {
		return nil, fmt.Errorf("qoder: chat stream not started")
	}
	defer stream.Close()
	var acc openai.ChatCompletionAccumulator
	for stream.Next() {
		if !acc.AddChunk(stream.Current()) {
			return nil, fmt.Errorf("qoder: malformed chat chunk")
		}
	}
	if err := stream.Err(); err != nil {
		var apiErr *openai.Error
		if !errors.As(err, &apiErr) && acc.ID == "" && len(acc.Choices) == 0 {
			return nil, &protocol.EmptyResponseError{Err: err}
		}
		return nil, err
	}
	if len(acc.Choices) == 0 {
		return nil, &protocol.EmptyResponseError{Err: fmt.Errorf("qoder: empty stream")}
	}
	result := acc.ChatCompletion
	result.Object = "chat.completion"
	return &result, nil
}
func (c *QoderClient) ResponsesNew(context.Context, responses.ResponseNewParams) (*responses.Response, error) {
	return nil, fmt.Errorf("qoder: responses endpoint is not supported")
}
func (c *QoderClient) ResponsesNewStreaming(context.Context, responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	return nil
}

// ListModels supplies a fallback list because Qoder stores its account-specific
// catalogue locally and does not expose a stable public models endpoint.
func (c *QoderClient) ListModels(context.Context) (*ModelListResult, error) {
	// These are known model keys, not an account entitlement list. The upstream
	// can reject models unavailable to the signed-in account.
	return &ModelListResult{Models: []string{"auto", "ultimate", "performance", "efficient", "lite", "cmodel", "qmodel_38max", "qmodel_latest", "qmodel", "kmodel_latest", "kmodel", "gmodel", "gm51model", "dmodel", "dfmodel", "mmodel"}}, nil
}

type qoderTransport struct {
	inner    http.RoundTripper
	provider *typ.Provider
}

func (t *qoderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost || !strings.HasSuffix(req.URL.Path, "/chat/completions") {
		return nil, fmt.Errorf("qoder: unsupported endpoint")
	}
	if req.Body == nil {
		return nil, fmt.Errorf("qoder: missing request body")
	}
	raw, err := io.ReadAll(io.LimitReader(req.Body, (32<<20)+1))
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(raw) > 32<<20 {
		return nil, fmt.Errorf("qoder: chat request exceeds size limit")
	}
	var chat map[string]any
	if err = json.Unmarshal(raw, &chat); err != nil {
		return nil, fmt.Errorf("qoder: invalid chat request")
	}
	model, _ := chat["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("qoder: model is required")
	}
	payload, err := qoderChatBody(chat, model)
	if err != nil {
		return nil, err
	}
	encoded := qoderEncode(payload)
	detail := t.provider.OAuthDetail
	meta := detail.ExtraFields
	uid, _ := meta["account_id"].(string)
	machine, _ := meta[oauth.QoderMachineID].(string)
	if uid == "" || machine == "" || detail.AccessToken == "" {
		return nil, fmt.Errorf("qoder: missing account or machine identity; sign in again")
	}
	org, _ := meta["qoder_organization_id"].(string)
	policy, _ := meta["qoder_data_policy_agreed"].(bool)
	tags := []string{}
	switch v := meta["qoder_organization_tags"].(type) {
	case []string:
		tags = v
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				tags = append(tags, s)
			}
		}
	}
	info, key, err := qoderAuth(uid, detail.AccessToken, org, tags, policy)
	if err != nil {
		return nil, err
	}
	date := strconv.FormatInt(time.Now().Unix(), 10)
	authPayload, err := json.Marshal(struct {
		Version     string `json:"version"`
		RequestID   string `json:"requestId"`
		Info        string `json:"info"`
		CosyVersion string `json:"cosyVersion"`
		IDEVersion  string `json:"ideVersion"`
	}{"v1", uuid.NewString(), info, qoderVersion, ""})
	if err != nil {
		return nil, err
	}
	p64 := base64.StdEncoding.EncodeToString(authPayload)
	digest := md5.Sum([]byte(strings.Join([]string{p64, key, date, string(encoded), qoderPath}, "\n")))
	target, err := url.Parse(strings.TrimRight(t.provider.APIBase, "/") + "/algo" + qoderPath + "?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1")
	if err != nil {
		return nil, err
	}
	out := req.Clone(req.Context())
	out.URL = target
	out.Host = target.Host
	out.Body = io.NopCloser(bytes.NewReader(encoded))
	out.ContentLength = int64(len(encoded))
	out.GetBody = nil
	out.Header = make(http.Header)
	out.Header.Set("Content-Type", "application/json")
	out.Header.Set("Accept", "text/event-stream")
	out.Header.Set("Authorization", "Bearer COSY."+p64+"."+hex.EncodeToString(digest[:]))
	out.Header.Set("Cosy-Key", key)
	out.Header.Set("Cosy-Date", date)
	out.Header.Set("Cosy-User", uid)
	out.Header.Set("Cosy-MachineId", machine)
	out.Header.Set("Cosy-Version", qoderVersion)
	out.Header.Set("User-Agent", "qoder/"+qoderVersion)
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}
	out.Header.Set("Cosy-MachineOS", arch+"_"+runtime.GOOS)
	out.Header.Set("Cosy-ClientType", "5")
	out.Header.Set("Cosy-Business-Product", "cli")
	out.Header.Set("Cosy-Business-Type", "agent")
	out.Header.Set("Cosy-Scene", "assistant")
	out.Header.Set("Login-Version", "v2")
	out.Header.Set("X-Model-Key", model)
	out.Header.Set("X-Model-Source", "system")
	if policy {
		out.Header.Set("Cosy-Data-Policy", "agree")
	} else {
		out.Header.Set("Cosy-Data-Policy", "disagree")
	}
	if org != "" {
		out.Header.Set("Cosy-Organization-Id", org)
	}
	if len(tags) > 0 {
		out.Header.Set("Cosy-Organization-Tags", strings.Join(tags, ","))
	}
	resp, err := t.inner.RoundTrip(out)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	resp.Body = &qoderSSEBody{src: resp.Body, reader: bufio.NewReader(resp.Body), model: model}
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	resp.Header.Set("Content-Type", "text/event-stream")
	return resp, nil
}

func qoderEncode(raw []byte) []byte {
	const standard = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	coded := []byte(base64.StdEncoding.EncodeToString(raw))
	for i, c := range coded {
		n := strings.IndexByte(standard, c)
		if n == 64 {
			coded[i] = '$'
		} else {
			coded[i] = qoderAlphabet[n]
		}
	}
	n := len(coded) / 3
	out := make([]byte, 0, len(coded))
	out = append(out, coded[len(coded)-n:]...)
	out = append(out, coded[n:len(coded)-n]...)
	out = append(out, coded[:n]...)
	return out
}

func qoderAuth(uid, token, org string, tags []string, policy bool) (string, string, error) {
	raw, err := json.Marshal(struct {
		UID    string   `json:"uid"`
		Token  string   `json:"security_oauth_token"`
		Org    string   `json:"organization_id"`
		Tags   []string `json:"organization_tags"`
		Policy bool     `json:"data_policy_agreed"`
	}{uid, token, org, tags, policy})
	if err != nil {
		return "", "", err
	}
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	key := []byte(hex.EncodeToString(b)[:16])
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	pad := aes.BlockSize - len(raw)%aes.BlockSize
	raw = append(raw, bytes.Repeat([]byte{byte(pad)}, pad)...)
	cipher.NewCBCEncrypter(block, key).CryptBlocks(raw, raw)
	pemBlock, _ := pem.Decode([]byte(qoderPublicKey))
	if pemBlock == nil {
		return "", "", fmt.Errorf("qoder: invalid public key")
	}
	pub, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
	if err != nil {
		return "", "", err
	}
	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", "", fmt.Errorf("qoder: invalid RSA key")
	}
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, rsaKey, key)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(raw), base64.StdEncoding.EncodeToString(encrypted), nil
}

func qoderReasoningModel(model string) bool {
	switch model {
	case "ultimate", "cmodel", "qmodel_38max", "gmodel", "gm51model", "dmodel", "dfmodel":
		return true
	}
	return false
}

func qoderChatBody(chat map[string]any, model string) ([]byte, error) {
	messages, ok := chat["messages"].([]any)
	if !ok {
		return nil, fmt.Errorf("qoder: messages required")
	}
	var systems []string
	converted := make([]any, 0, len(messages))
	for _, v := range messages {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("qoder: invalid message")
		}
		role, _ := m["role"].(string)
		if role == "system" || role == "developer" {
			content, ok := m["content"].(string)
			if !ok {
				return nil, fmt.Errorf("qoder: system messages must contain text")
			}
			if content != "" {
				systems = append(systems, content)
			}
			continue
		}
		if content, ok := m["content"].(string); ok && content != "" && role != "tool" {
			m["contents"] = []any{map[string]any{"type": "text", "text": content}}
		}
		converted = append(converted, m)
	}
	system := strings.Join(systems, "\n\n")
	if system != "" {
		converted = append([]any{map[string]any{"role": "system", "content": system}}, converted...)
	}
	messages = converted
	tools := chat["tools"]
	if tools == nil {
		tools = []any{}
	}
	params := map[string]any{}
	for _, k := range []string{"max_tokens", "max_completion_tokens", "temperature", "top_p", "stop", "tool_choice", "reasoning_effort"} {
		if v, ok := chat[k]; ok {
			if k == "max_completion_tokens" {
				k = "max_tokens"
			}
			params[k] = v
		}
	}
	if _, ok := params["max_tokens"]; !ok {
		params["max_tokens"] = 32000
	}
	id := uuid.NewString()
	business := map[string]any{"product": "cli", "version": qoderVersion, "type": "agent", "id": uuid.NewString(), "name": "", "begin_at": time.Now().UnixMilli(), "stage": "start"}
	body := map[string]any{"business": business, "request_id": id, "request_set_id": id, "chat_record_id": id, "session_id": uuid.NewString(), "stream": true, "chat_task": "FREE_INPUT", "chat_context": map[string]any{}, "is_reply": true, "is_retry": false, "source": 1, "version": "3", "agent_id": "agent_common", "task_id": "common", "session_type": "qodercli", "aliyun_user_type": "", "model_config": map[string]any{"key": model, "format": "openai", "source": "system", "enable": true, "is_vl": true, "is_reasoning": qoderReasoningModel(model)}, "custom_model": nil, "system": system, "messages": messages, "tools": tools, "parameters": params}
	return json.Marshal(body)
}

// qoderSSEBody unwraps upstream envelopes and waits for event:finish; an
// enclosed [DONE] is not the terminal event.
type qoderSSEBody struct {
	src       io.ReadCloser
	reader    *bufio.Reader
	pending   bytes.Buffer
	err       error
	model     string
	event     string
	done      bool
	toolIndex int
	seenTool  bool
}

func (b *qoderSSEBody) Read(p []byte) (int, error) {
	for b.pending.Len() == 0 && b.err == nil && !b.done {
		line, err := b.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			b.err = err
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "event:") {
			b.event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			if b.event == "finish" {
				b.pending.WriteString("data: [DONE]\n\n")
				b.done = true
			}
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var env struct {
				Body   string `json:"body"`
				Status int    `json:"statusCodeValue"`
			}
			if json.Unmarshal([]byte(data), &env) != nil {
				b.err = fmt.Errorf("qoder: malformed stream envelope")
				break
			}
			if env.Status != 0 && env.Status != 200 {
				b.err = fmt.Errorf("qoder: upstream stream status %d", env.Status)
				break
			}
			if env.Body != "" && env.Body != "[DONE]" {
				var chunk map[string]any
				if json.Unmarshal([]byte(env.Body), &chunk) != nil {
					b.err = fmt.Errorf("qoder: malformed stream chunk")
					break
				}
				// Qoder restarts tool indexes at zero for each new call.
				if choices, ok := chunk["choices"].([]any); ok {
					for _, item := range choices {
						choice, _ := item.(map[string]any)
						delta, _ := choice["delta"].(map[string]any)
						calls, _ := delta["tool_calls"].([]any)
						for _, v := range calls {
							call, ok := v.(map[string]any)
							if !ok {
								continue
							}
							if id, _ := call["id"].(string); id != "" {
								if b.seenTool {
									b.toolIndex++
								} else {
									b.seenTool = true
								}
							}
							call["index"] = b.toolIndex
						}
					}
				}
				chunk["model"] = b.model
				v, _ := json.Marshal(chunk)
				b.pending.WriteString("data: ")
				b.pending.Write(v)
				b.pending.WriteString("\n\n")
			}
		}
		if err == io.EOF {
			if !b.done {
				b.err = io.ErrUnexpectedEOF
			}
			break
		}
	}
	if b.pending.Len() > 0 {
		return b.pending.Read(p)
	}
	if b.err != nil {
		return 0, b.err
	}
	return 0, io.EOF
}
func (b *qoderSSEBody) Close() error { return b.src.Close() }
