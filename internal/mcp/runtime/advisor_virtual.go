package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func NewAdvisorVirtualTool(cfg typ.AdvisorConfig, cp *client.ClientPool, store *SessionStore) VirtualTool {
	if cfg.MaxUsesPerRequest <= 0 {
		cfg.MaxUsesPerRequest = 3
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}

	schema := mcp.ToolInputSchema{Type: "object"}
	props := map[string]any{
		"reason": map[string]any{
			"type":        "string",
			"description": "Why the executor is consulting the advisor.",
		},
	}
	schema.Properties = props
	schema.Required = []string{"reason"}

	return VirtualTool{
		Name:         "advisor",
		Description:  description(cfg.MaxUsesPerRequest),
		InputSchema:  schema,
		Handler:      newAdvisorHandler(cfg, cp, store),
		IsClientTool: false, // Server tool: not exposed to clients
	}
}

func newAdvisorHandler(cfg typ.AdvisorConfig, cp *client.ClientPool, store *SessionStore) VirtualToolHandler {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		args, ok := req.Params.Arguments.(map[string]any)
		if !ok {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent("invalid arguments")},
				IsError: true,
			}, nil
		}

		reason, _ := args["reason"].(string)
		if reason == "" {
			reason = "The executor has requested strategic guidance."
		}

		// Check depth to prevent recursion.
		// Depth is incremented by response hook before tool execution, so the first
		// legitimate advisor call runs at depth=1 and must be allowed.
		depth := GetAdvisorDepth(ctx)
		if depth > 1 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent("Advisor recursion limit reached.")},
				IsError: true,
			}, nil
		}

		// Check per-request quota from context
		actx, ok := GetAdvisorContext(ctx)
		if !ok || actx.UsesRemaining <= 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent("Advisor consultations exhausted for this request.")},
				IsError: true,
			}, nil
		}

		// Enrich advisor context with session data from SessionStore
		if store != nil {
			if sessionID, _ := args["session_id"].(string); sessionID != "" {
				if sc, found := store.Get(sessionID); found {
					actx = enrichAdvisorContextWithSession(actx, sc)
					ctx = WithAdvisorContext(ctx, actx)
				}
			}
		}

		// Execute advisor call
		logrus.WithFields(logrus.Fields{
			"reason":         reason,
			"uses_remaining": actx.UsesRemaining,
			"depth":          depth,
			"format":         detectAdvisorFormat(cfg),
		}).Debug("[MCP-DEBUG] ADVISOR: calling advisor model")

		advisorCtx, cancel := context.WithTimeout(ctx, advisorCallTimeout)
		defer cancel()

		var result string
		var err error
		if detectAdvisorFormat(cfg) == FormatOpenAI {
			result, err = callOpenAI(advisorCtx, cfg, cp, reason, actx)
		} else {
			result, err = callAnthropic(advisorCtx, cfg, cp, reason, actx)
		}

		if err != nil {
			logrus.WithError(err).Error("[MCP-DEBUG] ADVISOR: consultation failed")
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent(fmt.Sprintf("Advisor error: %v", err))},
				IsError: true,
			}, nil
		}

		// Decrement uses
		actx.UsesRemaining--
		logrus.WithField("uses_remaining", actx.UsesRemaining).Debug("[MCP-DEBUG] ADVISOR: consultation completed")

		return &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent(result)}}, nil
	}
}

// enrichAdvisorContextWithSession prepends session-persistent heavy data as
// system messages so the advisor model sees workspace state and build logs
// before the conversation history.
func enrichAdvisorContextWithSession(actx *AdvisorContext, sc *SessionContext) *AdvisorContext {
	if actx == nil {
		actx = &AdvisorContext{}
	}
	var enriched []map[string]any
	if len(sc.BuildLogs) > 0 {
		enriched = append(enriched, map[string]any{
			"role":    "system",
			"content": "Build logs:\n" + strings.Join(sc.BuildLogs, "\n"),
		})
	}
	if sc.LastWorkerResp != "" {
		enriched = append(enriched, map[string]any{
			"role":    "system",
			"content": "Last worker response:\n" + sc.LastWorkerResp,
		})
	}
	actx.Messages = append(enriched, actx.Messages...)
	return actx
}
