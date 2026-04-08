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

	// Context system message operations
	{
		Position:  PositionContextSystem,
		Operation: OpContextSystemContains,
		Meta: SmartOpMeta{
			Description: "Any system messages contain the value",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionContextSystem,
		Operation: OpContextSystemRegex,
		Meta: SmartOpMeta{
			Description: "Any system messages match regex pattern",
			Type:        ValueTypeString,
		},
	},

	// Context user message operations
	{
		Position:  PositionContextUser,
		Operation: OpContextUserContains,
		Meta: SmartOpMeta{
			Description: "Any user messages contain the value",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionContextUser,
		Operation: OpContextUserRegex,
		Meta: SmartOpMeta{
			Description: "Combined user messages match regex pattern",
			Type:        ValueTypeString,
		},
	},

	// Latest user message operations
	{
		Position:  PositionLatestUser,
		Operation: OpLatestUserContains,
		Meta: SmartOpMeta{
			Description: "Latest user message contains the value",
			Type:        ValueTypeString,
		},
	},
	{
		Position:  PositionLatestUser,
		Operation: OpLatestUserRequestType,
		Meta: SmartOpMeta{
			Description: "Latest user message content type (e.g., 'image')",
			Type:        ValueTypeString,
		},
	},

	// Tool use operations
	{
		Position:  PositionToolUse,
		Operation: OpToolUseEquals,
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

	// Service TTFT operations
	{
		Position:  PositionServiceTTFT,
		Operation: OpServiceTTFTAvgLe,
		Meta: SmartOpMeta{
			Description: "Min avg TTFT across services <= value (ms)",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionServiceTTFT,
		Operation: OpServiceTTFTAvgGe,
		Meta: SmartOpMeta{
			Description: "Avg TTFT mean across services >= value (ms)",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionServiceTTFT,
		Operation: OpServiceTTFTMaxLe,
		Meta: SmartOpMeta{
			Description: "Min P99 TTFT across services <= value (ms)",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionServiceTTFT,
		Operation: OpServiceTTFTMaxGe,
		Meta: SmartOpMeta{
			Description: "Avg P99 TTFT across services >= value (ms)",
			Type:        ValueTypeInt,
		},
	},

	// Service capacity operations (seat utilization %)
	{
		Position:  PositionServiceCapacity,
		Operation: OpServiceCapacityUtilLe,
		Meta: SmartOpMeta{
			Description: "Avg seat utilization across services <= value (%)",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionServiceCapacity,
		Operation: OpServiceCapacityUtilGe,
		Meta: SmartOpMeta{
			Description: "Avg seat utilization across services >= value (%)",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionServiceCapacity,
		Operation: OpServiceCapacityUtilLt,
		Meta: SmartOpMeta{
			Description: "Avg seat utilization across services < value (%)",
			Type:        ValueTypeInt,
		},
	},
	{
		Position:  PositionServiceCapacity,
		Operation: OpServiceCapacityUtilGt,
		Meta: SmartOpMeta{
			Description: "Avg seat utilization across services > value (%)",
			Type:        ValueTypeInt,
		},
	},
}

const (
	PositionModel           SmartOpPosition = "model"            // Request model name
	PositionThinking        SmartOpPosition = "thinking"         // Thinking mode enabled
	PositionContextSystem   SmartOpPosition = "context_system"   // System message content in context
	PositionContextUser     SmartOpPosition = "context_user"     // User message content in context
	PositionLatestUser      SmartOpPosition = "latest_user"      // Latest user message
	PositionToolUse         SmartOpPosition = "tool_use"         // Tool use/name
	PositionToken           SmartOpPosition = "token"            // Token count
	PositionServiceTTFT     SmartOpPosition = "service_ttft"     // Service TTFT characteristics
	PositionServiceCapacity SmartOpPosition = "service_capacity" // Service seat capacity (affinity utilization)
)

const (
	// Model operations
	OpModelContains SmartOpOperation = "contains" // Model name contains the value
	OpModelGlob     SmartOpOperation = "glob"     // Model name matches glob pattern
	OpModelEquals   SmartOpOperation = "equals"   // Model name equals the value

	// Thinking operations
	OpThinkingEnabled  SmartOpOperation = "enabled"  // Thinking mode is enabled
	OpThinkingDisabled SmartOpOperation = "disabled" // Thinking mode is disabled

	// Context system message operations
	OpContextSystemContains SmartOpOperation = "contains" // Any system messages contain the value
	OpContextSystemRegex    SmartOpOperation = "regex"    // Any system messages match regex pattern

	// Context user message operations
	OpContextUserContains SmartOpOperation = "contains" // Any user messages contain the value
	OpContextUserRegex    SmartOpOperation = "regex"    // Combined user messages match regex pattern

	// Latest user message operations
	OpLatestUserContains    SmartOpOperation = "contains" // Latest user message contains the value
	OpLatestUserRequestType SmartOpOperation = "type"     // Latest user message content type

	// Tool use operations
	OpToolUseEquals   SmartOpOperation = "equals"   // Latest message is `tool use` and its name equals the value
	OpToolUseContains SmartOpOperation = "contains" // Latest message is `tool use` and its name or arguments contains the value

	// Token operations
	OpTokenGe SmartOpOperation = "ge" // Token count greater than or equal to value
	OpTokenGt SmartOpOperation = "gt" // Token count greater than value
	OpTokenLe SmartOpOperation = "le" // Token count less than or equal to value
	OpTokenLt SmartOpOperation = "lt" // Token count less than value

	// Service TTFT operations
	OpServiceTTFTAvgLe SmartOpOperation = "avg_le" // Min avg TTFT across services <= value (ms)
	OpServiceTTFTAvgGe SmartOpOperation = "avg_ge" // Avg TTFT mean across services >= value (ms)
	OpServiceTTFTMaxLe SmartOpOperation = "max_le" // Min P99 TTFT across services <= value (ms)
	OpServiceTTFTMaxGe SmartOpOperation = "max_ge" // Avg P99 TTFT across services >= value (ms)

	// Service capacity operations (seat utilization %)
	OpServiceCapacityUtilLe SmartOpOperation = "util_le" // Avg seat utilization <= value (%)
	OpServiceCapacityUtilGe SmartOpOperation = "util_ge" // Avg seat utilization >= value (%)
	OpServiceCapacityUtilLt SmartOpOperation = "util_lt" // Avg seat utilization < value (%)
	OpServiceCapacityUtilGt SmartOpOperation = "util_gt" // Avg seat utilization > value (%)
)
