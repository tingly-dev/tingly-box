// MCP Local Mode Type Definitions

export type MCPMode = 'servertool' | 'clienttool';

export type MCPConnectionType = 'stdio' | 'http' | 'sse';

export type MCPAuthType = 'none' | 'headers' | 'oauth';

export type MCPClientState = 'connected' | 'connecting' | 'disconnected' | 'error';

export interface MCPStdioConfig {
    command: string;
    args?: string[];
    env?: string[];
    cwd?: string;
}

export interface MCPOAuthConfig {
    client_id: string;
    client_secret?: string;
    authorize_url: string;
    token_url: string;
    scopes?: string[];
}

export interface MCPSourceConfig {
    id?: string;
    name?: string;
    enabled?: boolean;
    transport?: 'http' | 'stdio' | 'sse';
    endpoint?: string;
    headers?: Record<string, string>;
    tools?: string[];
    command?: string;
    args?: string[];
    cwd?: string;
    env?: Record<string, string>;
    proxy_url?: string;

    // Local mode specific fields
    connection_type?: MCPConnectionType;
    auth_type?: MCPAuthType;
    allowed_extra_headers?: string[];
    stdio_config?: MCPStdioConfig;
    oauth_config?: MCPOAuthConfig;
    tools_to_execute?: string[];
    tools_to_auto_execute?: string[];
    is_ping_available?: boolean;
}

export interface MCPTool {
    name: string;
    description?: string;
}

export interface MCPClient {
    id: string;
    config: MCPSourceConfig;
    tools: MCPTool[];
    state: MCPClientState;
}

// Form value types for the editor
export interface MCPClientFormValue {
    name: string;
    enabled: boolean;
    connection_type: MCPConnectionType;

    // STDIO config
    stdio_command: string;
    stdio_args: string[];
    stdio_cwd: string;
    stdio_env: Array<{ key: string; value: string }>;

    // HTTP/SSE config
    endpoint: string;
    headers: Array<{ key: string; value: string }>;

    // Auth config
    auth_type: MCPAuthType;
    allowed_extra_headers: string[];

    // OAuth config
    oauth_client_id: string;
    oauth_client_secret: string;
    oauth_authorize_url: string;
    oauth_token_url: string;
    oauth_scopes: string[];

    // Tools config
    tools_to_execute: string[];
    tools_to_auto_execute: string[];

    // Other
    proxy_url: string;
}

export const defaultMCPClientFormValue = (): MCPClientFormValue => ({
    name: '',
    enabled: true,
    connection_type: 'stdio',

    // STDIO
    stdio_command: '',
    stdio_args: [],
    stdio_cwd: '',
    stdio_env: [],

    // HTTP/SSE
    endpoint: '',
    headers: [],

    // Auth
    auth_type: 'none',
    allowed_extra_headers: [],

    // OAuth
    oauth_client_id: '',
    oauth_client_secret: '',
    oauth_authorize_url: '',
    oauth_token_url: '',
    oauth_scopes: [],

    // Tools
    tools_to_execute: ['*'],
    tools_to_auto_execute: [],

    // Other
    proxy_url: '',
});

export const clientToFormValue = (client?: MCPClient): MCPClientFormValue => {
    const form = defaultMCPClientFormValue();
    if (!client) {
        return form;
    }

    const cfg = client.config;

    // Convert env object to array
    const stdioEnv: Array<{ key: string; value: string }> = [];
    if (cfg.env) {
        Object.entries(cfg.env).forEach(([key, value]) => {
            stdioEnv.push({ key, value });
        });
    }

    // Convert headers object to array
    const headers: Array<{ key: string; value: string }> = [];
    if (cfg.headers) {
        Object.entries(cfg.headers).forEach(([key, value]) => {
            headers.push({ key, value });
        });
    }

    return {
        name: cfg.name || client.id || '',
        enabled: cfg.enabled ?? true,
        connection_type: cfg.connection_type || 'stdio',

        stdio_command: cfg.command || cfg.stdio_config?.command || '',
        stdio_args: cfg.args || cfg.stdio_config?.args || [],
        stdio_cwd: cfg.cwd || cfg.stdio_config?.cwd || '',
        stdio_env: stdioEnv,

        endpoint: cfg.endpoint || '',
        headers,

        auth_type: cfg.auth_type || 'none',
        allowed_extra_headers: cfg.allowed_extra_headers || [],

        oauth_client_id: cfg.oauth_config?.client_id || '',
        oauth_client_secret: cfg.oauth_config?.client_secret || '',
        oauth_authorize_url: cfg.oauth_config?.authorize_url || '',
        oauth_token_url: cfg.oauth_config?.token_url || '',
        oauth_scopes: cfg.oauth_config?.scopes || [],

        tools_to_execute: cfg.tools_to_execute || ['*'],
        tools_to_auto_execute: cfg.tools_to_auto_execute || [],

        proxy_url: cfg.proxy_url || '',
    };
};

export const formValueToClientRequest = (form: MCPClientFormValue) => {
    // Build env map from array
    const envMap: Record<string, string> = {};
    form.stdio_env.forEach(({ key, value }) => {
        if (key.trim()) {
            envMap[key.trim()] = value;
        }
    });

    // Build headers map from array
    const headersMap: Record<string, string> = {};
    form.headers.forEach(({ key, value }) => {
        if (key.trim()) {
            headersMap[key.trim()] = value;
        }
    });

    const request: any = {
        name: form.name.trim(),
        enabled: form.enabled,
        connection_type: form.connection_type,
        auth_type: form.auth_type,
        tools_to_execute: form.tools_to_execute,
        tools_to_auto_execute: form.tools_to_auto_execute,
        allowed_extra_headers: form.allowed_extra_headers,
    };

    if (form.proxy_url.trim()) {
        request.proxy_url = form.proxy_url.trim();
    }

    // Connection-specific config
    if (form.connection_type === 'stdio') {
        request.stdio_config = {
            command: form.stdio_command.trim(),
            args: form.stdio_args.filter(Boolean),
            cwd: form.stdio_cwd.trim() || undefined,
            env: Object.keys(envMap).length > 0 ? Object.keys(envMap) : undefined,
        };
        if (Object.keys(envMap).length > 0) {
            request.env = envMap;
        }
    } else {
        request.connection_string = form.endpoint.trim();
        if (Object.keys(headersMap).length > 0) {
            request.headers = headersMap;
        }
    }

    // OAuth config
    if (form.auth_type === 'oauth') {
        request.oauth_config = {
            client_id: form.oauth_client_id.trim(),
            client_secret: form.oauth_client_secret.trim() || undefined,
            authorize_url: form.oauth_authorize_url.trim(),
            token_url: form.oauth_token_url.trim(),
            scopes: form.oauth_scopes.filter(Boolean),
        };
    }

    return request;
};

// Validation
export const isValidClientName = (name: string): boolean => {
    if (!name || name.length === 0) {
        return false;
    }

    // Check for invalid characters (no spaces, hyphens, tabs)
    for (const c of name) {
        if (c === '-' || c === ' ' || c === '\t') {
            return false;
        }
    }

    // Can't start with number
    if (name[0] >= '0' && name[0] <= '9') {
        return false;
    }

    return true;
};

export const validateClientForm = (form: MCPClientFormValue): string | null => {
    if (!isValidClientName(form.name)) {
        return 'Invalid name. Must contain only ASCII characters, no hyphens or spaces, and cannot start with a number';
    }

    if (form.connection_type === 'stdio') {
        if (!form.stdio_command.trim()) {
            return 'Command is required for STDIO connection';
        }
    } else {
        if (!form.endpoint.trim()) {
            return 'Endpoint URL is required for HTTP/SSE connection';
        }
        // Basic URL validation
        try {
            void new URL(form.endpoint.trim());
        } catch {
            return 'Invalid endpoint URL';
        }
    }

    if (form.auth_type === 'oauth') {
        if (!form.oauth_client_id.trim()) {
            return 'OAuth Client ID is required';
        }
        if (!form.oauth_authorize_url.trim()) {
            return 'OAuth Authorize URL is required';
        }
        try {
            void new URL(form.oauth_authorize_url.trim());
        } catch {
            return 'Invalid OAuth Authorize URL';
        }
        if (!form.oauth_token_url.trim()) {
            return 'OAuth Token URL is required';
        }
        try {
            void new URL(form.oauth_token_url.trim());
        } catch {
            return 'Invalid OAuth Token URL';
        }
    }

    return null;
};
