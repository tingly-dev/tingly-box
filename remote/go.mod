module github.com/tingly-dev/tingly-box/remote

go 1.26.0

replace github.com/tingly-dev/tingly-box/imbot => ../imbot

replace github.com/tingly-dev/tingly-box/agentboot => ../agentboot

require (
	github.com/sirupsen/logrus v1.9.4
	github.com/stretchr/testify v1.12.1
	github.com/tingly-dev/tingly-box/agentboot v0.0.0-00010101000000-000000000000
	github.com/tingly-dev/tingly-box/imbot v0.0.0-00010101000000-000000000000
)

require (
	github.com/bwmarrin/discordgo v0.29.0 // indirect
	github.com/go-telegram/bot v1.23.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/larksuite/oapi-sdk-go/v3 v3.9.10 // indirect
	github.com/mdp/qrterminal/v3 v3.2.1 // indirect
	github.com/open-dingtalk/dingtalk-stream-sdk-go v0.9.2-beta.1 // indirect
	github.com/slack-go/slack v0.27.0 // indirect
	github.com/tingly-dev/weixin v0.4.10 // indirect
	github.com/yuin/goldmark v1.8.5 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	rsc.io/qr v0.2.0 // indirect
)
