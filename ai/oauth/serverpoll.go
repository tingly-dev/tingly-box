package oauth

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// ServerPollFlow is an in-progress server-mediated login (OAuthMethodServerPoll)
// for any issuer that uses one: ZCode and WorkBuddy. Callers — the web handler
// and the CLI — only need the URL to show and a way to wait for the token, so
// this is the one type they see; the issuer-specific flow rides inside.
type ServerPollFlow struct {
	// AuthorizeURL is the URL the user must open (any device).
	AuthorizeURL string
	// FlowID identifies the flow upstream (ZCode flow id, WorkBuddy state).
	FlowID string
	// ExpiresAt is the upstream deadline, zero when unreported.
	ExpiresAt time.Time
	// PollInterval is the delay between upstream polls.
	PollInterval time.Duration
	// Timeout bounds the whole wait for the user's authorization.
	Timeout time.Duration

	Issuer     ai.Issuer
	Name       string
	RedirectTo string

	zcode     *ZCodeFlow
	workbuddy *WorkBuddyFlow
	qoder     *QoderFlow
}

// InitiateServerPollFlow starts the server-mediated login of issuer.
func (m *Manager) InitiateServerPollFlow(ctx context.Context, userID string, issuer ai.Issuer, redirectTo, name string, opts ...Option) (*ServerPollFlow, error) {
	switch {
	case IsZCodeIssuer(issuer):
		flow, err := m.InitiateZCodeFlow(ctx, userID, issuer, redirectTo, name, opts...)
		if err != nil {
			return nil, err
		}
		return &ServerPollFlow{
			AuthorizeURL: flow.AuthorizeURL,
			FlowID:       flow.FlowID,
			ExpiresAt:    flow.ExpiresAt,
			PollInterval: flow.PollInterval,
			Timeout:      ZCodeLoginTimeout,
			Issuer:       issuer,
			Name:         name,
			zcode:        flow,
		}, nil
	case ai.IsQoderIssuer(issuer):
		client := m.qoderClient(applyOptions(opts...), issuer)
		flow, err := client.Start()
		if err != nil {
			return nil, err
		}
		return &ServerPollFlow{AuthorizeURL: flow.AuthorizeURL, FlowID: flow.Nonce, PollInterval: time.Second, Timeout: 5 * time.Minute, Issuer: issuer, Name: name, RedirectTo: redirectTo, qoder: flow}, nil
	case ai.IsWorkBuddyIssuer(issuer):
		flow, err := m.InitiateWorkBuddyFlow(ctx, userID, issuer, redirectTo, name, opts...)
		if err != nil {
			return nil, err
		}
		return &ServerPollFlow{
			AuthorizeURL: flow.AuthorizeURL,
			FlowID:       flow.State,
			PollInterval: flow.PollInterval,
			// The account lookup runs after the token poll; budget for both
			// so a late sign-in still gets its account info.
			Timeout:   WorkBuddyLoginTimeout + workbuddyAccountTimeout,
			Issuer:    issuer,
			Name:      name,
			workbuddy: flow,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %s has no server-mediated login", ErrInvalidProvider, issuer)
	}
}

// CompleteServerPollFlow waits for the user's authorization and returns the
// credential as an ordinary *Token, ready for the shared provider-creation path.
func (m *Manager) CompleteServerPollFlow(ctx context.Context, flow *ServerPollFlow, opts ...Option) (*Token, error) {
	switch {
	case flow == nil:
		return nil, fmt.Errorf("login: flow not started")
	case flow.zcode != nil:
		return m.CompleteZCodeFlow(ctx, flow.zcode, opts...)
	case flow.qoder != nil:
		client := m.qoderClient(applyOptions(opts...), flow.Issuer)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		credential, err := client.Poll(ctx, flow.qoder)
		if err != nil {
			return nil, err
		}
		token := credential.asToken(flow.Issuer)
		token.Name = flow.Name
		token.RedirectTo = flow.RedirectTo
		token.putMetadata(QoderMachineID, flow.qoder.MachineID)
		profile, err := client.Profile(ctx, credential.token())
		if err != nil {
			return nil, err
		}
		if profile.UID != "" && credential.UserID != "" && profile.UID != credential.UserID {
			return nil, fmt.Errorf("qoder: account identity mismatch")
		}
		if profile.UID == "" {
			profile.UID = credential.UserID
		}
		if profile.UID == "" {
			return nil, fmt.Errorf("qoder: account identity missing")
		}
		token.putMetadata("account_id", profile.UID)
		token.putMetadata("qoder_organization_id", profile.OrganizationID)
		if token.Metadata == nil {
			token.Metadata = map[string]any{}
		}
		token.Metadata["qoder_organization_tags"] = profile.OrganizationTags
		token.Metadata["qoder_data_policy_agreed"] = profile.DataPolicyAgreed
		if token.Name == "" {
			token.Name = profile.Name
		}
		if token.Name == "" {
			token.Name = credential.UserName
		}
		return token, nil
	case flow.workbuddy != nil:
		return m.CompleteWorkBuddyFlow(ctx, flow.workbuddy, opts...)
	default:
		return nil, fmt.Errorf("login: flow not started")
	}
}
