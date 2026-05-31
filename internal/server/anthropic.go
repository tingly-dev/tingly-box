package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type (
	// AnthropicModel Model types - based on Anthropic's official models API format
	AnthropicModel struct {
		ID          string `json:"id"`
		CreatedAt   string `json:"created_at"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
		// AuthType is a tingly-box extension (not in Anthropic's wire format)
		// consumed by the frontend to order model picker entries:
		// oauth -> api_key -> vmodel.
		AuthType string `json:"auth_type,omitempty"`
	}
	AnthropicModelsResponse struct {
		Data    []AnthropicModel `json:"data"`
		FirstID string           `json:"first_id"`
		HasMore bool             `json:"has_more"`
		LastID  string           `json:"last_id"`
	}
)

// HandleAnthropicMessages handles Anthropic v1 messages API requests
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (s *Server) HandleAnthropicMessages(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"
	logrus.Debugf("scenario: %s beta: %v", scenario, beta)

	// Validate scenario
	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	//if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportAnthropic) {
	//	c.JSON(http.StatusBadRequest, ErrorResponse{
	//		Error: ErrorDetail{
	//			Message: fmt.Sprintf("scenario %s does not support Anthropic messages", scenario),
	//			Type:    "invalid_request_error",
	//		},
	//	})
	//	return
	//}

	c.Set("server_instance", s)

	// Start scenario-level recording (client -> tingly-box traffic) only if enabled
	var recorder *ProtocolRecorder
	if s.GetScenarioRecordMode(scenarioType) != "" {
		recorder = s.BeginProtocolRecording(c, scenario)
		if recorder != nil {
			// Store recorder in context for use in handlers
			c.Set(recorderContextKey, recorder)
			// Note: RecordResponse will be called by handler after stream completes
		}
	}

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		// Record error if recording is enabled
		if recorder != nil {
			recorder.RecordError(err)
		}
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Determine provider & requestModel
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)
	var requestModel string
	var reqParams interface{} // For smart routing context extraction

	var betaMessages protocol.AnthropicBetaMessagesRequest
	var messages protocol.AnthropicMessagesRequest
	if beta {
		if err := json.Unmarshal(bodyBytes, &betaMessages); err != nil {
			// Record error if recording is enabled
			if recorder != nil {
				recorder.RecordError(err)
			}
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message error: %s", string(bodyBytes)),
					Type:    "invalid_request_error",
				},
			})
			logrus.WithError(err).WithField("body", string(bodyBytes)).Errorf("Anthropic beta decode error")
			c.Abort()
			return
		}
		requestModel = string(betaMessages.Model)
		reqParams = &betaMessages.BetaMessageNewParams

	} else {
		if err := json.Unmarshal(bodyBytes, &messages); err != nil {
			// Record error if recording is enabled
			if recorder != nil {
				recorder.RecordError(err)
			}
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: fmt.Sprintf("Message error: %s", string(bodyBytes)),
					Type:    "invalid_request_error",
				},
			})
			logrus.WithError(err).WithField("body", string(bodyBytes)).Errorf("Anthropic decode error")
			c.Abort()
			return
		}

		requestModel = string(messages.Model)
		reqParams = &messages.MessageNewParams
	}

	// Check if this is the request requestModel name first
	rule, err = s.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	s.applyVisionProxy(c, scenarioType, rule, reqParams)

	// Select service using routing pipeline
	provider, selectedService, err = s.routingSelector.SelectService(c, scenarioType, rule, reqParams)
	if err != nil {
		// Record error if recording is enabled
		if recorder != nil {
			recorder.RecordError(nil)
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		logrus.WithError(err).Errorf("Select service error")
		return
	}

	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	// Set the rule and provider in context
	if rule != nil {
		c.Set("rule", rule)
	}

	// sessionID is automatically stored by SelectService

	actualModel := selectedService.Model

	// Virtual-model providers are served by the in-process vmodel handler.
	// Resolution went through the normal routing pipeline so rules/scenarios
	// still apply, but no outbound HTTP is performed. We must rewrite Model
	// in the forwarded body so the vmodel registry sees the rule's resolved
	// model ID (actualModel) rather than the client-facing requestModel.
	//
	// NOTE: this path intentionally skips outbound dispatch helpers
	// (pre-chain, guardrails, post-recording). Usage/quota tracking for
	// vmodel is a separate concern tracked in the roadmap.
	if provider.IsVirtual() && s.virtualModelService != nil {
		var (
			rewritten []byte
			err       error
		)
		if beta {
			betaMessages.Model = anthropic.Model(actualModel)
			rewritten, err = json.Marshal(betaMessages)
		} else {
			messages.Model = anthropic.Model(actualModel)
			rewritten, err = json.Marshal(messages)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "Failed to prepare virtual-model request: " + err.Error(),
					Type:    "internal_error",
				},
			})
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(rewritten)))
		c.Request.ContentLength = int64(len(rewritten))
		s.virtualModelService.GetHandler().Messages(c)
		return
	}

	// Delegate to the appropriate implementation based on beta parameter
	if beta {
		s.AnthropicMessagesV1Beta(c, betaMessages, requestModel, provider, actualModel, rule)

	} else {
		s.AnthropicMessagesV1(c, messages, requestModel, provider, actualModel, rule)
	}
}
