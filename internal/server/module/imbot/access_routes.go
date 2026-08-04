package imbot

import "github.com/tingly-dev/tingly-box/swagger"

func RegisterAccessRoutes(router *swagger.RouteGroup, h *Handler) {
	router.GET("/bots/:bot/capabilities", h.ListCapabilities, swagger.WithTags("bot-access"), swagger.WithDescription("List explicit product capabilities for a bot."), swagger.WithPathParam("bot", "string", "Bot UUID"), swagger.WithResponseModel(CapabilityListResponse{}))
	router.PUT("/bots/:bot/capabilities/:capability", h.PutCapability, swagger.WithTags("bot-access"), swagger.WithDescription("Enable or disable one bot capability and return the derived runtime state."), swagger.WithPathParam("bot", "string", "Bot UUID"), swagger.WithPathParam("capability", "string", "notify or remote_control"), swagger.WithRequestModel(CapabilityUpdateRequest{}), swagger.WithResponseModel(CapabilityUpdateResponse{}))

	router.GET("/bots/:bot/chats", h.ListDirectChats, swagger.WithTags("bot-access"), swagger.WithDescription("List Direct Chats only; Groups are a separate resource."), swagger.WithPathParam("bot", "string", "Bot UUID"), swagger.WithResponseModel(DirectChatListResponse{}))
	router.GET("/bots/:bot/chats/:chat", h.GetDirectChat, swagger.WithTags("bot-access"), swagger.WithDescription("Get Direct Chat identity and concrete permissions."), swagger.WithPathParam("bot", "string", "Bot UUID"), swagger.WithPathParam("chat", "string", "Direct Chat UUID"), swagger.WithResponseModel(DirectChatDetailResponse{}))
	router.PUT("/bots/:bot/chats/:chat/blocked", h.PutDirectChatBlocked, swagger.WithTags("bot-access"), swagger.WithDescription("Set the reversible hard block for a Direct Chat."), swagger.WithRequestModel(BlockedUpdateRequest{}), swagger.WithResponseModel(OKResponse{}))
	router.PUT("/bots/:bot/chats/:chat/permissions/:capability/:action", h.PutDirectChatPermission, swagger.WithTags("bot-access"), swagger.WithDescription("Set one explicit Direct Chat capability/action permission."), swagger.WithRequestModel(PermissionUpdateRequest{}), swagger.WithResponseModel(OKResponse{}))
	router.PUT("/bots/:bot/chats/:chat/permissions", h.PutDirectChatPermissions, swagger.WithTags("bot-access"), swagger.WithDescription("Atomically set multiple explicit Direct Chat permissions in one transaction; presets use this so a partial failure can never leave mixed rows."), swagger.WithRequestModel(PermissionsUpdateRequest{}), swagger.WithResponseModel(OKResponse{}))
	router.POST("/bots/:bot/chats/:chat/unpair", h.UnpairDirectChat, swagger.WithTags("bot-access"), swagger.WithDescription("Remove peer trust and Remote Control permissions without deleting the Direct Chat."), swagger.WithResponseModel(OKResponse{}))
	router.DELETE("/bots/:bot/chats/:chat", h.DeleteDirectChat, swagger.WithTags("bot-access"), swagger.WithDescription("Delete a Direct Chat after dependency checks."), swagger.WithResponseModel(OKResponse{}))
	router.POST("/bots/:bot/chats/:chat/authorize-check", h.AuthorizeDirectChat, swagger.WithTags("bot-access"), swagger.WithDescription("Evaluate the production authorization path without executing the action."), swagger.WithRequestModel(AuthorizeCheckRequest{}), swagger.WithResponseModel(AuthorizeCheckResponse{}))

	router.GET("/bots/:bot/groups", h.ListGroups, swagger.WithTags("bot-access"), swagger.WithDescription("List Groups only; Direct Chats are a separate resource."), swagger.WithResponseModel(GroupListResponse{}))
	router.GET("/bots/:bot/groups/:group", h.GetGroup, swagger.WithTags("bot-access"), swagger.WithDescription("Get Group capability access and authorized actors."), swagger.WithResponseModel(GroupDetailResponse{}))
	router.PUT("/bots/:bot/groups/:group/blocked", h.PutGroupBlocked, swagger.WithTags("bot-access"), swagger.WithDescription("Set the reversible hard block for a Group."), swagger.WithRequestModel(BlockedUpdateRequest{}), swagger.WithResponseModel(OKResponse{}))
	router.PUT("/bots/:bot/groups/:group/capabilities/:capability", h.PutGroupCapability, swagger.WithTags("bot-access"), swagger.WithDescription("Set Group access for one capability; this never grants an Actor permission."), swagger.WithRequestModel(PermissionUpdateRequest{}), swagger.WithResponseModel(OKResponse{}))
	router.DELETE("/bots/:bot/groups/:group", h.DeleteGroup, swagger.WithTags("bot-access"), swagger.WithDescription("Delete a Group after dependency checks."), swagger.WithResponseModel(OKResponse{}))
	router.POST("/bots/:bot/groups/:group/authorize-check", h.AuthorizeGroup, swagger.WithTags("bot-access"), swagger.WithDescription("Evaluate the production authorization path without executing the action."), swagger.WithRequestModel(AuthorizeCheckRequest{}), swagger.WithResponseModel(AuthorizeCheckResponse{}))
	router.GET("/bots/:bot/groups/:group/actors", h.ListGroupActors, swagger.WithTags("bot-access"), swagger.WithDescription("List observed and explicitly authorized Group Actors."), swagger.WithResponseModel(GroupActorsResponse{}))
	router.PUT("/bots/:bot/groups/:group/actors/:actor", h.PutGroupActor, swagger.WithTags("bot-access"), swagger.WithDescription("Add or update a Group Actor controller binding."), swagger.WithRequestModel(GroupActorPutRequest{}))
	router.PUT("/bots/:bot/groups/:group/actors/:actor/permissions/:capability/:action", h.PutGroupActorPermission, swagger.WithTags("bot-access"), swagger.WithDescription("Set one concrete Group Actor permission."), swagger.WithRequestModel(PermissionUpdateRequest{}), swagger.WithResponseModel(OKResponse{}))
	router.DELETE("/bots/:bot/groups/:group/actors/:actor", h.DeleteGroupActor, swagger.WithTags("bot-access"), swagger.WithDescription("Remove a Group Actor binding and its permissions."), swagger.WithResponseModel(OKResponse{}))

	router.GET("/bots/:bot/routes", h.ListRoutes, swagger.WithTags("bot-routes"), swagger.WithDescription("List Notify routes independently from bot capabilities."), swagger.WithResponseModel(RouteListResponse{}))
	router.POST("/bots/:bot/routes", h.CreateRoute, swagger.WithTags("bot-routes"), swagger.WithDescription("Create a Notify route to one internal Direct Chat or Group UUID."), swagger.WithRequestModel(RouteWriteRequest{}), swagger.WithResponseModel(RouteResponse{}))
	router.GET("/bots/:bot/routes/:route", h.GetRoute, swagger.WithTags("bot-routes"), swagger.WithDescription("Get one Notify route."), swagger.WithResponseModel(RouteResponse{}))
	router.PUT("/bots/:bot/routes/:route", h.UpdateRoute, swagger.WithTags("bot-routes"), swagger.WithDescription("Replace one Notify route while preserving its stable identity."), swagger.WithRequestModel(RouteWriteRequest{}), swagger.WithResponseModel(RouteResponse{}))
	router.DELETE("/bots/:bot/routes/:route", h.DeleteRoute, swagger.WithTags("bot-routes"), swagger.WithDescription("Delete one Notify route."), swagger.WithResponseModel(OKResponse{}))
}
