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
  // The vendor's published context window (Tingly Box's own field).
  context_window?: number;
  // Anthropic Models API names: the most input one request may carry (below
  // context_window when the window counts output too) and the most output.
  max_input_tokens?: number;
  max_tokens?: number;
  // Codex's list only (Tingly Box's own field): what the rule's "Auto" writes
  // into Codex's model catalog for this model, with its 1M flag off and on.
  codex_windows?: CodexAutoWindows;
  auth_type?: string;
}

// What one Codex catalog model is told about its context, both in input
// tokens (config.CodexContextWindow): context_window is what Codex plans
// around, max_context_window how far Codex may raise it. source says why:
// the routed model's input limit (catalog), the larger window the 1M flag
// opens (extended), or a guess for a model the catalog doesn't know (default).
export interface CodexWindow {
  context_window: number;
  max_context_window: number;
  source: 'catalog' | 'extended' | 'default';
}

export interface CodexAutoWindows {
  off: CodexWindow;
  on: CodexWindow;
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
  // Tingly Box extension: the vendor's published context window.
  context_window?: number;
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
