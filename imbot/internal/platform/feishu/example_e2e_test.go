//go:build ignore
// +build ignore

// Feishu/Lark E2E Test Example
//
// This file demonstrates how to manually test Feishu and Lark bots.
// See feishu_e2e_test.go for the actual e2e tests.
//
// Quick Start:
//
// 1. For Feishu:
//    export FEISHU_APP_ID="cli-your-app-id"
//    export FEISHU_APP_SECRET="your-app-secret"
//    export FEISHU_TEST_CHAT_ID="your-chat-id"  # Optional
//    go test -tags=e2e -v -run TestE2E_FeishuBot_RealBot ./imbot/internal/platform/feishu/
//
// 2. For Lark:
//    export LARK_APP_ID="cli-your-app-id"
//    export LARK_APP_SECRET="your-app-secret"
//    export LARK_TEST_CHAT_ID="your-chat-id"  # Optional
//    go test -tags=e2e -v -run TestE2E_LarkBot_RealBot ./imbot/internal/platform/feishu/
//
// Or use FEISHU_DOMAIN to specify domain:
//    export FEISHU_APP_ID="cli-your-app-id"
//    export FEISHU_APP_SECRET="your-app-secret"
//    export FEISHU_DOMAIN="lark"
//    go test -tags=e2e -v -run TestE2E_FeishuBot_RealBot ./imbot/internal/platform/feishu/
//
// Getting Credentials:
//
// Feishu: https://open.feishu.cn/
//   1. Create a new app
//   2. Enable Bot capability
//   3. Add permissions: im:message, im:message.p2p_msg:readonly
//   4. Add event: im.message.receive_v1
//   5. Select Long Connection mode
//   6. Get App ID and App Secret from "Credentials & Basic Info"
//
// Lark: https://open.larksuite.com/
//   Same steps as Feishu, but on Lark's platform

package main

func main() {
	// This is a documentation-only file
	// See feishu_e2e_test.go for actual test code
}
