package imbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/channel"
)

type channelTransportFacts struct{ registry *channel.Registry }

func (s channelTransportFacts) TransportFacts(botUUID string, _ access.CapabilityName, _ access.ActionName) (access.TransportStatus, bool) {
	if s.registry == nil {
		return access.TransportOffline, false
	}
	_, ok := s.registry.Get(botUUID)
	if !ok {
		return access.TransportOffline, false
	}
	return access.TransportOnline, true
}

type CapabilityUpdateRequest struct {
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}
type CapabilityListResponse struct {
	Capabilities []access.BotCapability `json:"capabilities"`
	BotRunning   bool                   `json:"bot_running"`
}
type CapabilityUpdateResponse struct {
	Capability access.BotCapability `json:"capability"`
	BotRunning bool                 `json:"bot_running"`
	Reason     string               `json:"reason,omitempty"`
}
type BlockedUpdateRequest struct {
	Blocked *bool `json:"blocked"`
}
type PermissionUpdateRequest struct {
	Effect access.AccessEffect `json:"effect"`
}

// PermissionWrite is one explicit capability/action/effect row in a batch
// permission update.
type PermissionWrite struct {
	Capability access.CapabilityName `json:"capability"`
	Action     access.ActionName     `json:"action"`
	Effect     access.AccessEffect   `json:"effect"`
}

// PermissionsUpdateRequest carries a batch of permission rows that must be
// applied atomically.
type PermissionsUpdateRequest struct {
	Permissions []PermissionWrite `json:"permissions"`
}
type DirectChatDetailResponse struct {
	Chat        access.DirectChat   `json:"chat"`
	Permissions []access.Permission `json:"permissions"`
}
type DirectChatListResponse struct {
	Chats []DirectChatDetailResponse `json:"chats"`
}
type GroupDetailResponse struct {
	Group        access.Group                                  `json:"group"`
	Capabilities map[access.CapabilityName]access.AccessEffect `json:"capabilities"`
	Actors       []access.GroupActor                           `json:"actors"`
}
type GroupListResponse struct {
	Groups []access.Group `json:"groups"`
}
type GroupActorPutRequest struct {
	ExternalActorID string `json:"external_actor_id"`
	DisplayName     string `json:"display_name,omitempty"`
	Label           string `json:"label,omitempty"`
}
type GroupActorsResponse struct {
	Actors []access.GroupActor `json:"actors"`
}
type AuthorizeCheckRequest struct {
	ActorID    string                `json:"actor_id,omitempty"`
	Capability access.CapabilityName `json:"capability"`
	Action     access.ActionName     `json:"action"`
	RouteID    string                `json:"route_id,omitempty"`
	RequestID  string                `json:"request_id,omitempty"`
}
type AuthorizeCheckResponse struct {
	Decision access.AuthorizationDecision `json:"decision"`
}
type RouteTargetRequest struct {
	Kind access.TargetKind `json:"kind"`
	ID   string            `json:"id"`
}
type RouteWriteRequest struct {
	Name        string             `json:"name"`
	Source      string             `json:"source"`
	Events      []string           `json:"events"`
	Target      RouteTargetRequest `json:"target"`
	Enabled     bool               `json:"enabled"`
	Options     json.RawMessage    `json:"options,omitempty"`
	GrantNotify bool               `json:"grant_notify,omitempty"`
}
type RouteResponse struct {
	Route access.Route `json:"route"`
}
type RouteListResponse struct {
	Routes []access.Route `json:"routes"`
}
type OKResponse struct {
	OK bool `json:"ok"`
}

func routeFromRequest(botUUID, routeID string, req RouteWriteRequest) access.Route {
	events, _ := json.Marshal(req.Events)
	return access.Route{
		ID:          routeID,
		BotUUID:     botUUID,
		Name:        req.Name,
		Source:      req.Source,
		EventFilter: events,
		Target:      access.TargetRef{Kind: req.Target.Kind, ID: req.Target.ID},
		Enabled:     req.Enabled,
		Options:     req.Options,
	}
}

func (h *Handler) requireAccess(c *gin.Context) bool {
	if h.accessStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bot access store not available"})
		return false
	}
	return true
}
func accessError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, db.ErrAccessTargetNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "target not found"})
	case errors.Is(err, db.ErrInvalidCapability), errors.Is(err, db.ErrInvalidPermission):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case strings.HasPrefix(err.Error(), "target_has_routes"):
		c.JSON(http.StatusConflict, gin.H{"error": "target_has_routes", "details": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func (h *Handler) ListCapabilities(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	caps, err := h.accessStore.ListCapabilities(c.Request.Context(), c.Param("bot"))
	if err != nil {
		accessError(c, err)
		return
	}
	running := h.botMgr != nil && h.botMgr.IsRunning(c.Param("bot"))
	c.JSON(http.StatusOK, CapabilityListResponse{Capabilities: caps, BotRunning: running})
}
func (h *Handler) PutCapability(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	name := access.CapabilityName(c.Param("capability"))
	if !name.Valid() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "unknown capability"})
		return
	}
	var req CapabilityUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	capability := access.BotCapability{BotUUID: c.Param("bot"), Name: name, Enabled: req.Enabled, Config: req.Config}
	botSettings, err := h.store.GetSettingsByUUID(capability.BotUUID)
	if err != nil || botSettings.UUID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "bot not found"})
		return
	}
	if err := h.accessStore.PutCapability(c.Request.Context(), capability); err != nil {
		accessError(c, err)
		return
	}
	enabled, err := h.accessStore.AnyCapabilityEnabled(c.Request.Context(), capability.BotUUID)
	if err != nil {
		accessError(c, err)
		return
	}
	// Capability changes and the Bot lifecycle gate form one operational
	// action: enabling any capability starts a stopped Bot; disabling the last
	// capability turns the now-unused Bot off. When another capability remains,
	// preserve the Bot gate so disabling Notify cannot wake or stop Remote (and
	// vice versa).
	desiredBotEnabled := botSettings.Enabled
	if req.Enabled {
		desiredBotEnabled = true
	} else if !enabled {
		desiredBotEnabled = false
	}
	if desiredBotEnabled != botSettings.Enabled {
		if err := h.store.SetEnabled(capability.BotUUID, desiredBotEnabled); err != nil {
			accessError(c, err)
			return
		}
	}
	shouldRun := desiredBotEnabled && enabled
	if h.botMgr != nil {
		if shouldRun && !h.botMgr.IsRunning(capability.BotUUID) {
			_ = h.botMgr.StartBot(context.Background(), capability.BotUUID)
		} else if !shouldRun && h.botMgr.IsRunning(capability.BotUUID) {
			_ = h.botMgr.StopBot(capability.BotUUID)
		}
	}
	stored, _, _ := h.accessStore.GetCapability(c.Request.Context(), capability.BotUUID, name)
	reason := ""
	if !enabled {
		reason = "no_enabled_capability"
	}
	c.JSON(http.StatusOK, CapabilityUpdateResponse{Capability: stored, BotRunning: h.botMgr != nil && h.botMgr.IsRunning(capability.BotUUID), Reason: reason})
}

func (h *Handler) ListDirectChats(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	chats, err := h.accessStore.ListDirectChats(c.Request.Context(), c.Param("bot"))
	if err != nil {
		accessError(c, err)
		return
	}
	out := make([]DirectChatDetailResponse, 0, len(chats))
	for _, chat := range chats {
		p, err := h.accessStore.ListDirectChatPermissions(c.Request.Context(), c.Param("bot"), chat.ID)
		if err != nil {
			accessError(c, err)
			return
		}
		out = append(out, DirectChatDetailResponse{Chat: chat, Permissions: p})
	}
	c.JSON(http.StatusOK, DirectChatListResponse{Chats: out})
}
func (h *Handler) GetDirectChat(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	chat, ok, err := h.accessStore.GetDirectChat(c.Request.Context(), c.Param("bot"), c.Param("chat"))
	if err != nil {
		accessError(c, err)
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
		return
	}
	p, err := h.accessStore.ListDirectChatPermissions(c.Request.Context(), c.Param("bot"), chat.ID)
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, DirectChatDetailResponse{Chat: chat, Permissions: p})
}
func (h *Handler) PutDirectChatBlocked(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req BlockedUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Blocked == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "blocked is required"})
		return
	}
	if err := h.accessStore.SetDirectChatBlocked(c.Request.Context(), c.Param("bot"), c.Param("chat"), *req.Blocked); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// PutDirectChatPermissions applies a batch of explicit Direct Chat permission
// rows in one store transaction, so a preset can never half-apply.
func (h *Handler) PutDirectChatPermissions(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req PermissionsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Permissions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "permissions is required"})
		return
	}
	perms := make([]access.Permission, 0, len(req.Permissions))
	for _, p := range req.Permissions {
		perms = append(perms, access.Permission{Capability: p.Capability, Action: p.Action, Effect: p.Effect})
	}
	if err := h.accessStore.SetDirectChatPermissions(c.Request.Context(), c.Param("bot"), c.Param("chat"), perms); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) PutDirectChatPermission(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req PermissionUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.accessStore.SetDirectChatPermission(c.Request.Context(), c.Param("bot"), c.Param("chat"), access.CapabilityName(c.Param("capability")), access.ActionName(c.Param("action")), req.Effect); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
func (h *Handler) UnpairDirectChat(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	if err := h.accessStore.UnpairDirectChat(c.Request.Context(), c.Param("bot"), c.Param("chat")); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
func (h *Handler) DeleteDirectChat(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	if err := h.accessStore.DeleteDirectChat(c.Request.Context(), c.Param("bot"), c.Param("chat")); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) ListGroups(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	groups, err := h.accessStore.ListGroups(c.Request.Context(), c.Param("bot"))
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, GroupListResponse{Groups: groups})
}
func (h *Handler) GetGroup(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	group, ok, err := h.accessStore.GetGroup(c.Request.Context(), c.Param("bot"), c.Param("group"))
	if err != nil {
		accessError(c, err)
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	caps, err := h.accessStore.ListGroupCapabilities(c.Request.Context(), c.Param("bot"), group.ID)
	if err != nil {
		accessError(c, err)
		return
	}
	actors, err := h.accessStore.ListGroupActors(c.Request.Context(), c.Param("bot"), group.ID)
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, GroupDetailResponse{Group: group, Capabilities: caps, Actors: actors})
}
func (h *Handler) PutGroupBlocked(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req BlockedUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Blocked == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "blocked is required"})
		return
	}
	if err := h.accessStore.SetGroupBlocked(c.Request.Context(), c.Param("bot"), c.Param("group"), *req.Blocked); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
func (h *Handler) PutGroupCapability(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req PermissionUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.accessStore.SetGroupCapability(c.Request.Context(), c.Param("bot"), c.Param("group"), access.CapabilityName(c.Param("capability")), req.Effect); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
func (h *Handler) DeleteGroup(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	if err := h.accessStore.DeleteGroup(c.Request.Context(), c.Param("bot"), c.Param("group")); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
func (h *Handler) ListGroupActors(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	actors, err := h.accessStore.ListGroupActors(c.Request.Context(), c.Param("bot"), c.Param("group"))
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, GroupActorsResponse{Actors: actors})
}
func (h *Handler) PutGroupActor(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req GroupActorPutRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ExternalActorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "external_actor_id is required"})
		return
	}
	actor, err := h.accessStore.AddGroupActor(c.Request.Context(), c.Param("bot"), c.Param("group"), req.ExternalActorID, req.DisplayName, req.Label)
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, actor)
}
func (h *Handler) PutGroupActorPermission(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req PermissionUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.accessStore.SetGroupActorPermission(c.Request.Context(), c.Param("bot"), c.Param("group"), c.Param("actor"), access.CapabilityName(c.Param("capability")), access.ActionName(c.Param("action")), req.Effect); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
func (h *Handler) DeleteGroupActor(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	if err := h.accessStore.RemoveGroupActor(c.Request.Context(), c.Param("bot"), c.Param("group"), c.Param("actor")); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

func (h *Handler) authorizeCheck(c *gin.Context, target access.TargetRef) {
	if h.authorizer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorizer unavailable"})
		return
	}
	var req AuthorizeCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	decision := h.authorizer.Evaluate(c.Request.Context(), access.AuthorizationRequest{BotUUID: c.Param("bot"), Target: target, ActorID: req.ActorID, Capability: req.Capability, Action: req.Action, RouteID: req.RouteID, RequestID: req.RequestID})
	c.JSON(http.StatusOK, AuthorizeCheckResponse{Decision: decision})
}
func (h *Handler) AuthorizeDirectChat(c *gin.Context) {
	h.authorizeCheck(c, access.TargetRef{Kind: access.TargetDirectChat, ID: c.Param("chat")})
}
func (h *Handler) AuthorizeGroup(c *gin.Context) {
	h.authorizeCheck(c, access.TargetRef{Kind: access.TargetGroup, ID: c.Param("group")})
}

func (h *Handler) ListRoutes(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	routes, err := h.accessStore.ListRoutes(c.Request.Context(), c.Param("bot"))
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, RouteListResponse{Routes: routes})
}
func (h *Handler) GetRoute(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	route, ok, err := h.accessStore.GetRoute(c.Request.Context(), c.Param("bot"), c.Param("route"))
	if err != nil {
		accessError(c, err)
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}
	c.JSON(http.StatusOK, RouteResponse{Route: route})
}
func (h *Handler) CreateRoute(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req RouteWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Source == "" || req.Target.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, source and target are required"})
		return
	}
	route, err := h.accessStore.CreateRoute(c.Request.Context(), routeFromRequest(c.Param("bot"), "", req), req.GrantNotify)
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusCreated, RouteResponse{Route: route})
}
func (h *Handler) UpdateRoute(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	var req RouteWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Source == "" || req.Target.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, source and target are required"})
		return
	}
	route, err := h.accessStore.UpdateRoute(c.Request.Context(), routeFromRequest(c.Param("bot"), c.Param("route"), req), req.GrantNotify)
	if err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, RouteResponse{Route: route})
}
func (h *Handler) DeleteRoute(c *gin.Context) {
	if !h.requireAccess(c) {
		return
	}
	if err := h.accessStore.DeleteRoute(c.Request.Context(), c.Param("bot"), c.Param("route")); err != nil {
		accessError(c, err)
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
