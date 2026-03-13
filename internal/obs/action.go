package obs

// ActionType represents the type of action performed
type ActionType = string

const (
	ActionAddProvider    ActionType = "add_provider"
	ActionDeleteProvider ActionType = "delete_provider"
	ActionUpdateProvider ActionType = "update_provider"
	ActionStartServer    ActionType = "start_server"
	ActionStopServer     ActionType = "stop_server"
	ActionRestartServer  ActionType = "restart_server"
	ActionGenerateToken  ActionType = "generate_token"
	ActionUpdateDefaults ActionType = "update_defaults"
	ActionFetchModels    ActionType = "fetch_models"
)
