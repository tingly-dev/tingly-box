/**
 * Model types for /v1/models API responses
 */

// OpenAI format models
export interface OpenAIModel {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  description?: string;
  context?: number;
  max_output?: number;
  auth_type?: string;
}

export interface OpenAIModelsResponse {
  object: string;
  data: OpenAIModel[];
}

// Anthropic format models
export interface AnthropicModel {
  id: string;
  created_at: string;
  display_name: string;
  type: string;
  capabilities?: ModelCapabilities;
  max_input_tokens?: number;
  max_tokens?: number;
  description?: string;
  auth_type?: string;
}

export interface ModelCapabilities {
  batch: CapabilitySupport;
  citations: CapabilitySupport;
  code_execution: CapabilitySupport;
  context_management?: ContextManagementCapability;
  effort?: EffortCapability;
  image_input: CapabilitySupport;
  pdf_input: CapabilitySupport;
  structured_outputs: CapabilitySupport;
  thinking?: ThinkingCapability;
}

export interface CapabilitySupport {
  supported: boolean;
}

export interface ContextManagementCapability {
  supported: boolean;
  clear_thinking_20251015?: CapabilitySupport;
  clear_tool_uses_20250919?: CapabilitySupport;
  compact_20260112?: CapabilitySupport;
}

export interface EffortCapability {
  supported: boolean;
  low?: CapabilitySupport;
  medium?: CapabilitySupport;
  high?: CapabilitySupport;
  xhigh?: CapabilitySupport;
  max?: CapabilitySupport;
}

export interface ThinkingCapability {
  supported: boolean;
  types?: ThinkingTypes;
}

export interface ThinkingTypes {
  adaptive: CapabilitySupport;
  enabled: CapabilitySupport;
}

export interface AnthropicModelsResponse {
  data: AnthropicModel[];
  first_id: string;
  has_more: boolean;
  last_id: string;
}

/**
 * Map of model IDs to their descriptions
 * Used for caching model descriptions from the /v1/models API
 */
export type ModelDescriptionMap = Record<string, string>;
