/**
 * Remote Graph Types
 * Types for remote graph configuration - connecting ImBot instances to AI agents
 *
 * @deprecated This file is kept for backward compatibility only.
 * Most types have been moved to use BotSettings.default_agent directly.
 */

/**
 * Agent Configuration - detailed settings for an agent
 */
export interface AgentConfig {
    uuid: string;
    name: string;
    agent_type: 'claude-code' | 'custom' | 'mock';
    system_prompt?: string;
    temperature?: number;
    max_tokens?: number;
    tools?: string[];
    enabled: boolean;
}

/**
 * Guide Agent configuration
 */
export interface GuideAgent {
    uuid: string;
    name: string;
    providers?: string[];
}

/**
 * Config Provider
 */
export interface ConfigProvider {
    uuid: string;
    name: string;
}

/**
 * API Response types
 */
export interface RemoteAgentsListResponse {
    success: boolean;
    agents: RemoteAgent[];
}

export interface RemoteAgentResponse {
    success: boolean;
    agent: RemoteAgent;
}

export interface RemoteAgentCreateRequest {
    name: string;
    description?: string;
    agent_type?: string;
}

export interface RemoteAgentUpdateRequest {
    name?: string;
    description?: string;
    agent_type?: string;
}

export interface DeleteResponse {
    success: boolean;
    message: string;
}

/**
 * Remote Agent configuration
 * @deprecated Use BotSettings.default_agent (string UUID) instead
 */
export interface RemoteAgent {
    uuid: string;
    name: string;
    description?: string;
    agent_type: 'claude-code' | 'custom' | 'mock';
    created_at: string;
    updated_at: string;
}

// Legacy types - kept for backward compatibility but not used in current implementation
/**
 * @deprecated Use BotSettings.default_agent instead
 */
export interface RemoteConnection {
    uuid: string;
    graph_uuid: string;
    imbot_uuid: string;
    agent_uuid: string;
    agent_config: any;
    routing_mode: 'direct' | 'smart_guide';
    enabled: boolean;
    status: 'active' | 'inactive' | 'error';
    position?: { x: number; y: number };
    created_at: string;
    updated_at: string;
}

/**
 * @deprecated Entire RemoteGraph concept is deprecated
 */
export interface RemoteGraph {
    uuid: string;
    name: string;
    description?: string;
    connections: RemoteConnection[];
    created_at: string;
    updated_at: string;
}
