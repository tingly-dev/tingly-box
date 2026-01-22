package smartrouting

// Operations is a comprehensive list of all available operations for smart routing.
// This registry defines all operations across all positions for documentation,
// UI rendering, and future API integrations.
var Operations = []SmartOp{
	// Model operations
	{
		Position:  PositionModel,
		Operation: OpModelContains,
		Meta: SmartOpMeta{
			Description: "Model name contains the value",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionModel,
		Operation: OpModelGlob,
		Meta: SmartOpMeta{
			Description: "Model name matches glob pattern",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionModel,
		Operation: OpModelEquals,
		Meta: SmartOpMeta{
			Description: "Model name equals the value",
			Type:        ValueTypeString,
		},
	},

	// Thinking operations
	{
		Position:  PositionThinking,
		Operation: OpThinkingEnabled,
		Meta: SmartOpMeta{
			Description: "Thinking mode is enabled",
			Type:        ValueTypeBool,
		},
	},
	{
		Position:  PositionThinking,
		Operation: OpThinkingDisabled,
		Meta: SmartOpMeta{
			Description: "Thinking mode is disabled",
			Type:        ValueTypeBool,
		},
	},

	// System message operations
	{
		Position:  PositionSystem,
		Operation: OpSystemAnyContains,
		Meta: SmartOpMeta{
			Description: "Any system messages contain the value",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionSystem,
		Operation: OpSystemRegex,
		Meta: SmartOpMeta{
			Description: "Any system messages match regex pattern",
			Type:        ValueTypeString,
		},
	},

	// User message operations
	{
		Position:  PositionUser,
		Operation: OpUserAnyContains,
		Meta: SmartOpMeta{
			Description: "Any user messages contain the value",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionUser,
		Operation: OpUserContains,
		Meta: SmartOpMeta{
			Description: "Lastest message is `user` role and it contains the value",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionUser,
		Operation: OpUserRegex,
		Meta: SmartOpMeta{
			Description: "Combined user messages match regex pattern",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionUser,
		Operation: OpUserRequestType,
		Meta: SmartOpMeta{
			Description: "Lastest message is `user` role and check its content type (e.g., 'image')",
			Type:        ValueTypeString,
		},
	},

	// Tool use operations
	{
		Position:  PositionToolUse,
		Operation: OpToolUseIs,
		Meta: SmartOpMeta{
			Description: "Latest message is `tool use` and it is name is the value",
			Type:        ValueTypeString,
		},
	},
	// Keep it
	// {
	// 	Position:  PositionToolUse,
	// 	Operation: OpToolUseContains,
	// 	Meta: SmartOpMeta{
	// 		Description: "Latest message is `tool use` and its name or arguments contains the value",
	// 		Type:        ValueTypeString,
	// 	},
	// },

	// Token operations
	{
		Position:  PositionToken,
		Operation: OpTokenGe,
		Meta: SmartOpMeta{
			Description: "Token count greater than or equal to value",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionToken,
		Operation: OpTokenGt,
		Meta: SmartOpMeta{
			Description: "Token count greater than value",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionToken,
		Operation: OpTokenLe,
		Meta: SmartOpMeta{
			Description: "Token count less than or equal to value",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionToken,
		Operation: OpTokenLt,
		Meta: SmartOpMeta{
			Description: "Token count less than value",
			Type:        ValueTypeInt,
		},
	},
}

const (
	PositionModel    SmartOpPosition = "model"    // Request model name
	PositionThinking SmartOpPosition = "thinking" // Thinking mode enabled
	PositionSystem   SmartOpPosition = "system"   // System message content
	PositionUser     SmartOpPosition = "user"     // User message content
	PositionToolUse  SmartOpPosition = "tool_use" // Tool use/name
	PositionToken    SmartOpPosition = "token"    // Token count
)

const (
	// Model operations
	OpModelContains SmartOpOperation = "contains" // Model name contains the value
	OpModelGlob     SmartOpOperation = "glob"     // Model name matches glob pattern
	OpModelEquals   SmartOpOperation = "equals"   // Model name equals the value

	// Thinking operations
	OpThinkingEnabled  SmartOpOperation = "enabled"  // Thinking mode is enabled
	OpThinkingDisabled SmartOpOperation = "disabled" // Thinking mode is disabled

	// System message operations
	OpSystemAnyContains SmartOpOperation = "any_contains" // Any system messages contain the value
	OpSystemRegex       SmartOpOperation = "regex"        // Any system messages match regex pattern

	// User message operations
	OpUserAnyContains SmartOpOperation = "any_contains" // Any user messages contain the value
	OpUserContains    SmartOpOperation = "contains"     // Latest message is `user` role and it contains the value
	OpUserRegex       SmartOpOperation = "regex"        // Combined user messages match regex pattern
	OpUserRequestType SmartOpOperation = "type"         // Latest message is `user` role and check its content type

	// Tool use operations
	OpToolUseIs       SmartOpOperation = "is"       // Latest message is `tool use` and its name is the value
	OpToolUseContains SmartOpOperation = "contains" // Latest message is `tool use` and its name or arguments contains the value

	// Token operations
	OpTokenGe SmartOpOperation = "ge" // Token count greater than or equal to value
	OpTokenGt SmartOpOperation = "gt" // Token count greater than value
	OpTokenLe SmartOpOperation = "le" // Token count less than or equal to value
	OpTokenLt SmartOpOperation = "lt" // Token count less than value
)
