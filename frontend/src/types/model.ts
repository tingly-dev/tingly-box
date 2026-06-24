/**
 * Model types for /v1/models API responses
 */

// OpenAI format models
export interface ModelDetail {
  description?: string;
  context?: number;
  max_tokens?: number;
  max_completion_tokens?: number;
  input_modalities?: string[];
  output_modalities?: string[];
  auth_type?: string;
}

export interface OpenAIModel {
  id: string;
  object: string;
  created: number;
  owned_by: string;
  detail?: ModelDetail;
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
  max_input_tokens?: number;
  max_tokens?: number;
  detail?: ModelDetail;
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
