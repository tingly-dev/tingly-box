// Config Apply control-plane API: safe endpoints that generate tool configs
// from system state (Claude Code, OpenCode, Codex, DSH) plus the Codex
// OpenAI-session importer.
import {rawControlCall} from './rawControlCall';
import {controlApi, errorMessage} from './openapi';

export const configApplyApi = {
    // Config Apply API - Safe endpoints that generate config from system state.
    // `preferences` is the source of truth: each key is a Claude Code env
    // var name (e.g. ANTHROPIC_MODEL), and the backend writes them straight
    // into ~/.claude/settings.json under "env".
    // `coAuthoredBy` left out, or equal to what settings.json says, leaves its
    // keys as the user wrote them (off when the file has none).
    applyClaudeConfig: async (preferences: Record<string, string>, installStatusLine?: boolean, defaultMode: string = 'auto', showThinkingSummaries: boolean = true, coAuthoredBy?: boolean): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/config/apply/claude', {
            headers,
            body: {preferences, installStatusLine, defaultMode, showThinkingSummaries, coAuthoredBy},
        }));
    },

    applyOpenCodeConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/config/apply/opencode', {headers}));
    },

    getOpenCodeConfigPreview: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/config/preview/opencode', {headers}));
    },

    // WorkBuddy: merges the scenario's models into each installed edition's models.json (~/.workbuddy, ~/.workbuddy-ai).
    applyWorkBuddyConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/config/apply/workbuddy', {headers}));
    },

    getWorkBuddyConfigPreview: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/config/preview/workbuddy', {headers}));
    },

    applyCodexConfig: async (
        preferences?: Record<string, string>,
        writeCatalog?: boolean,
        authMode?: 'apikey' | 'chatgpt' | 'hybrid',
        oauthProviderUuid?: string,
    ): Promise<any> =>
        rawControlCall(
            (client, headers) => client.POST('/api/v1/config/apply/codex', {
                headers,
                body: {
                    preferences: preferences ?? {},
                    writeCatalog: writeCatalog ?? true,
                    authMode: authMode ?? 'apikey',
                    oauthProviderUuid: oauthProviderUuid ?? '',
                },
            }),
            // Callers read `message` (not `error`) on this endpoint — keep the
            // shape but carry the backend's real message instead of a generic.
            (err) => ({success: false, message: errorMessage(err)}),
            (error: any) => ({success: false, message: error?.message || 'Failed to apply Codex configuration'}),
        ),

    getCodexConfigPreview: async (
        preferences?: Record<string, string>,
        writeCatalog?: boolean,
        authMode?: 'apikey' | 'chatgpt' | 'hybrid',
        oauthProviderUuid?: string,
    ): Promise<any> =>
        rawControlCall(
            (client, headers) => client.POST('/api/v1/config/preview/codex', {
                headers,
                body: {
                    preferences: preferences ?? {},
                    writeCatalog: writeCatalog ?? true,
                    authMode: authMode ?? 'apikey',
                    oauthProviderUuid: oauthProviderUuid ?? '',
                },
            }),
            (err) => ({success: false, message: errorMessage(err)}),
            (error: any) => ({success: false, message: error?.message || 'Failed to preview Codex configuration'}),
        ),

    applyDshConfig: async (
        preferences?: Record<string, string>,
    ): Promise<any> =>
        rawControlCall(
            (client, headers) => client.POST('/api/v1/config/apply/dsh', {
                headers,
                body: {
                    preferences: preferences ?? {},
                },
            }),
            // Callers read `message` (not `error`) on this endpoint — keep the
            // shape but carry the backend's real message instead of a generic.
            (err) => ({success: false, message: errorMessage(err)}),
            (error: any) => ({success: false, message: error?.message || 'Failed to apply DeepSeek Harness configuration'}),
        ),

    getDshConfigPreview: async (
        preferences?: Record<string, string>,
    ): Promise<any> =>
        rawControlCall(
            (client, headers) => client.POST('/api/v1/config/preview/dsh', {
                headers,
                body: {
                    preferences: preferences ?? {},
                },
            }),
            (err) => ({success: false, message: errorMessage(err)}),
            (error: any) => ({success: false, message: error?.message || 'Failed to preview DeepSeek Harness configuration'}),
        ),

    importCodexOpenAISessions: async (payload: {
        sourceProvider?: string;
        targetProvider?: string;
        codexHome?: string;
        sqliteHome?: string;
        stateDbPath?: string;
        includeArchived?: boolean;
        createBackup?: boolean;
        dryRun?: boolean;
    } = {}): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/codex/import/openai', {
            headers,
            body: payload,
        }));
    },
};
