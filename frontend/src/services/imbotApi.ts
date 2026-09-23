// ImBot settings control-plane API: platform configs, settings CRUD, restart/
// toggle, TOFU pairing codes, and the Weixin QR / Feishu-Lark registration
// flows. (IM bot interaction — capabilities/chats/groups/permissions +
// notify/interact/wait — follows a different contract and lives in botApi.ts.)
import {controlApi} from './openapi';

export const imbotApi = {
    // ========== ImBot Settings API ==========

    // Get ImBot platform configurations
    getImBotPlatforms: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-platforms', {headers})),

    // List all ImBot settings
    getImBotSettingsList: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-settings', {headers})),

    getImBotSetting: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    createImBotSetting: async (data: {
        name?: string;
        platform: string;
        auth_type: string;
        auth?: Record<string, string>;
        proxy_url?: string;
        chat_id_lock?: string;
        bash_allowlist?: string[];
        default_agent?: string;
        agent_type?: string;
        default_cwd?: string;
        enabled?: boolean;
        require_pairing?: boolean;
        persistent_session?: boolean;
    }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-settings', {
            headers,
            body: data as any
        })),

    // 404 ("ImBot setting not found") already comes back verbatim from the
    // backend in the unwrap()ped error message — no special-casing needed.
    updateImBotSetting: async (uuid: string, data: {
        name?: string;
        auth_type?: string;
        auth?: Record<string, string>;
        proxy_url?: string;
        chat_id_lock?: string;
        bash_allowlist?: string[];
        enabled?: boolean;
        default_agent?: string;
        default_cwd?: string;
        require_pairing?: boolean;
        persistent_session?: boolean;
        smartguide_provider?: string;
        smartguide_model?: string;
        remote_agent?: boolean;
    }): Promise<any> =>
        controlApi((client, headers) => client.PUT('/api/v1/imbot-settings/{uuid}', {
            headers,
            params: {path: {uuid}},
            body: data
        })),

    deleteImBotSetting: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v1/imbot-settings/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    restartImBot: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-admin/restart/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    toggleImBotSetting: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/toggle', {
            headers,
            params: {path: {uuid}}
        })),

    // Reveal current TOFU pairing code (audit-logged on every call).
    getImBotPairingCode: async (uuid: string): Promise<{
        success: boolean;
        active?: boolean;
        code?: string;
        expires_at?: string;
        message?: string;
        error?: string;
    }> =>
        controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}/pairing-code', {
            headers,
            params: {path: {uuid}}
        })),

    // Mint a fresh TOFU pairing code, invalidating the previous one.
    rotateImBotPairingCode: async (uuid: string): Promise<{
        success: boolean;
        active?: boolean;
        code?: string;
        expires_at?: string;
        message?: string;
        error?: string;
    }> =>
        controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/pairing-code/rotate', {
            headers,
            params: {path: {uuid}}
        })),

    // ========== Weixin QR Login API ==========

    // Start Weixin QR login flow
    weixinQRStart: async (botUUID: string, platform?: string, botName?: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/weixin/qr-start', {
            headers,
            params: {path: {uuid: botUUID}},
            body: {bot_uuid: botUUID, bot_platform: platform, bot_name: botName},
        }));
    },

    // Poll Weixin QR login status
    weixinQRStatus: async (botUUID: string, qrCodeId: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}/weixin/qr-status', {
            headers,
            params: {path: {uuid: botUUID}, query: {qrcode_id: qrCodeId}},
        }));
    },

    // Cancel Weixin QR login flow
    weixinQRCancel: async (botUUID: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/weixin/qr-cancel', {
            headers,
            params: {path: {uuid: botUUID}},
        }));
    },

    // ========== Feishu/Lark One-Click Registration API ==========

    // Start Feishu/Lark one-click app registration; returns a QR verification link
    feishuRegStart: async (botUUID: string, platform?: string, botName?: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/feishu/qr-start', {
            headers,
            params: {path: {uuid: botUUID}},
            body: {bot_uuid: botUUID, bot_platform: platform, bot_name: botName},
        }));
    },

    // Poll Feishu/Lark one-click registration status
    feishuRegStatus: async (botUUID: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/imbot-settings/{uuid}/feishu/qr-status', {
            headers,
            params: {path: {uuid: botUUID}},
        }));
    },

    // Cancel a pending Feishu/Lark one-click registration
    feishuRegCancel: async (botUUID: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/imbot-settings/{uuid}/feishu/qr-cancel', {
            headers,
            params: {path: {uuid: botUUID}},
        }));
    },
};
