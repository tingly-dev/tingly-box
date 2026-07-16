package taskapi

import (
	"github.com/tingly-dev/tingly-box/swagger"
)

func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.GET("/tasks/agents", handler.Agents,
		swagger.WithTags("tasks"),
		swagger.WithDescription("List native agent CLI availability"),
		swagger.WithResponseModel(AgentListResponse{}))
	router.GET("/tasks", handler.List,
		swagger.WithTags("tasks"),
		swagger.WithDescription("List agent tasks"),
		swagger.WithResponseModel(TaskListResponse{}))
	router.POST("/tasks", handler.Create,
		swagger.WithTags("tasks"),
		swagger.WithDescription("Create an agent task with a stable generated or existing workspace"),
		swagger.WithRequestModel(CreateRequest{}),
		swagger.WithResponseModel(TaskResponse{}))
	router.GET("/tasks/:id", handler.Get,
		swagger.WithTags("tasks"),
		swagger.WithDescription("Get an agent task"),
		swagger.WithResponseModel(TaskResponse{}))
	router.PATCH("/tasks/:id", handler.Update,
		swagger.WithTags("tasks"),
		swagger.WithDescription("Update an agent task's durable title or goal"),
		swagger.WithRequestModel(UpdateRequest{}),
		swagger.WithResponseModel(TaskResponse{}))
	router.GET("/tasks/:id/runs", handler.ListRuns,
		swagger.WithTags("tasks"),
		swagger.WithDescription("List bounded execution history for an agent task"),
		swagger.WithResponseModel(RunListResponse{}))
	router.GET("/tasks/:id/runs/:runID", handler.GetRun,
		swagger.WithTags("tasks"),
		swagger.WithDescription("Get one bounded execution for an agent task"),
		swagger.WithResponseModel(RunResponse{}))
	router.POST("/tasks/:id/wake", handler.Wake,
		swagger.WithTags("tasks"),
		swagger.WithDescription("Run now, run again, or run with a one-time instruction"),
		swagger.WithRequestModel(WakeRequest{}),
		swagger.WithResponseModel(TaskResponse{}))
	router.POST("/tasks/:id/stop", handler.Stop,
		swagger.WithTags("tasks"),
		swagger.WithDescription("Stop an agent task"))
}
