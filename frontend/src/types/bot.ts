// Bot platform authentication types

export interface BotPlatformConfig {
	platform: string;
	display_name: string;
	auth_type: string;
	category: string;
	fields: FieldSpec[];
}

export interface FieldSpec {
	key: string;
	label: string;
	placeholder?: string;
	required: boolean;
	secret: boolean;
	helperText?: string;
}

export interface BotSettings {
    uuid?: string;
    name?: string;
    platform: string;
    auth_type: string;
    auth: Record<string, string>;
    proxy_url?: string;
    chat_id?: string;
    bash_allowlist?: string[];
    enabled?: boolean;
    token?: string; // Legacy field for backward compatibility
    created_at?: string;
    updated_at?: string;
}

export interface BotPlatformCategory {
	key: string;
	label: string;
	platforms: BotPlatformConfig[];
}

// Category display labels
export const CategoryLabels: Record<string, string> = {
	im: 'IM Platforms',
	enterprise: 'Enterprise',
	business: 'Business',
};

// Auth type display labels
export const AuthTypeLabels: Record<string, string> = {
	token: 'Token',
	oauth: 'OAuth',
	qr: 'QR Code',
	basic: 'Basic Auth',
};

// Helper to mask secret values for display
export function maskSecret(value: string, visible = false): string {
	if (!value) return '-';
	if (visible) return value;
	if (value.length <= 8) return '*'.repeat(value.length);
	return value.substring(0, 4) + '*'.repeat(Math.min(8, value.length - 4)) + value.substring(value.length - 4);
}

// Helper to get display name for auth field value
export function getAuthDisplayValue(settings: BotSettings, config: BotPlatformConfig): string {
	if (!settings.auth || Object.keys(settings.auth).length === 0) {
		return '-';
	}

	// For token auth, show masked token
	if (config.auth_type === 'token') {
		const token = settings.auth['token'];
		return token ? maskSecret(token) : '-';
	}

	// For OAuth, show clientId
	if (config.auth_type === 'oauth') {
		const clientId = settings.auth['clientId'];
		return clientId ? maskSecret(clientId) : '-';
	}

	return 'Configured';
}
