package feishu

// MessageEvent represents an incoming message event
type MessageEvent struct {
	Header EventHeader        `json:"header"`
	Event  MessageEventDetail `json:"event"`
}

// EventHeader represents the event header
type EventHeader struct {
	EventID   string `json:"event_id"`
	Timestamp string `json:"timestamp"`
	Token     string `json:"token"`
	EventType string `json:"event_type"`
}

// MessageEventDetail represents message event details
type MessageEventDetail struct {
	MessageID  string        `json:"message_id"`
	RootID     interface{}   `json:"root_id"`
	ParentID   interface{}   `json:"parent_id"`
	CreateTime string        `json:"create_time"`
	ChatType   string        `json:"chat_type"`
	MsgType    string        `json:"msg_type"`
	Content    interface{}   `json:"content"`
	Mention    MentionDetail `json:"mention"`
	Sender     SenderDetail  `json:"sender"`
	ChatID     string        `json:"chat_id"`
}

// MentionDetail represents mention details
type MentionDetail struct {
	MentionList []MentionItem `json:"mention_list"`
}

// MentionItem represents a mention item
type MentionItem struct {
	ID        string `json:"id"`
	IDType    string `json:"id_type"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	TenantKey string `json:"tenant_key"`
}

// SenderDetail represents sender details
type SenderDetail struct {
	SenderID   string `json:"sender_id"`
	SenderType string `json:"sender_type"`
	TenantKey  string `json:"tenant_key"`
}

// FeishuContentElement represents a content element
type FeishuContentElement struct {
	Tag  string `json:"tag"`
	Text string `json:"text,omitempty"`
	Href string `json:"href,omitempty"`
}

// FeishuPostContent represents the structure of a post message content
type FeishuPostContent struct {
	ZhCn FeishuPostZhCn `json:"zh_cn"`
}

// FeishuPostZhCn represents the Chinese content of a post message
type FeishuPostZhCn struct {
	Title   string                   `json:"title"`
	Content [][]FeishuContentElement `json:"content"`
}
