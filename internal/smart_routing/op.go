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
	{
		Position:  PositionToolUse,
		Operation: OpToolUseContains,
		Meta: SmartOpMeta{
			Description: "Latest message is `tool use` and its name or arguments contains the value",
			Type:        ValueTypeString,
		},
	},

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
