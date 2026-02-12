module github.com/tingly-dev/tingly-box

go 1.25.3

replace github.com/openai/openai-go/v3 => ./libs/openai-go

replace github.com/anthropics/anthropic-sdk-go => ./libs/anthropic-sdk-go

replace google.golang.org/genai => ./libs/go-genai

replace github.com/tingly-dev/tingly-box/imbot => ./imbot

require (
	github.com/anthropics/anthropic-sdk-go v1.22.1
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/expr-lang/expr v1.17.7
	github.com/fsnotify/fsnotify v1.9.0
	github.com/gin-gonic/gin v1.11.0
	github.com/go-shiori/go-readability v0.0.0-20251205110129-5db1dc9836f0
	github.com/gobwas/glob v0.2.3
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/mattn/go-sqlite3 v1.14.34
	github.com/openai/openai-go/v3 v3.21.0
	github.com/otiai10/copy v1.14.1
	github.com/sirupsen/logrus v1.9.4
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	github.com/tiktoken-go/tokenizer v0.7.0
	github.com/tingly-dev/tingly-box/imbot v0.1.0
	go.opentelemetry.io/otel v1.40.0
	go.opentelemetry.io/otel/metric v1.40.0
	go.opentelemetry.io/otel/sdk/metric v1.40.0
	google.golang.org/genai v1.46.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.50.0
	golang.org/x/sys v0.41.0
	golang.org/x/text v0.34.0 // indirect
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.18.1 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de // indirect
	github.com/bwmarrin/discordgo v0.29.0 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.0 // indirect
	github.com/bytedance/sonic/loader v0.5.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/go-shiori/dom v0.0.0-20230515143342-73569d674e1c // indirect
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.12 // indirect
	github.com/googleapis/gax-go/v2 v2.17.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/slack-go/slack v0.17.3 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.65.0 // indirect
	go.opentelemetry.io/otel/sdk v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	go.uber.org/mock v0.6.0 // indirect
	golang.org/x/arch v0.24.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	google.golang.org/grpc v1.78.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
