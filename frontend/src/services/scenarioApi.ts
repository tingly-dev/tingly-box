// Scenario control-plane API: per-scenario config + flags + descriptors.
import {controlApi} from './openapi';
import {readCached} from './readCache';

export const scenarioApi = {
    // Scenario API
    getScenarioConfig: async (scenario: string): Promise<any> =>
        readCached(`scenario:${scenario}`, () => controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}', {
            headers,
            params: {path: {scenario}},
        }))),

    setScenarioConfig: async (scenario: string, config: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/scenario/{scenario}', {
            headers,
            params: {path: {scenario}},
            body: config,
        }));
    },

    getScenarioFlag: async (scenario: string, flag: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
        }));
    },

    setScenarioFlag: async (scenario: string, flag: string, value: boolean): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
            body: {value},
        }));
    },

    getScenarioStringFlag: async (scenario: string, flag: string): Promise<any> =>
        readCached(`string-flag:${scenario}:${flag}`, () => controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/string-flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
        }))),

    setScenarioStringFlag: async (scenario: string, flag: string, value: string): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/string-flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
            body: {value},
        }));
    },

    // Claude Code model slots with a rule of their own in unified mode.
    // `scenario` is claude_code or a profile (claude_code:<id>).
    getClaudeCodeSlots: async (scenario: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/claude-code/slots', {
            headers,
            params: {path: {scenario}},
        }));
    },

    // Give a slot a rule of its own (enabled), or hand it back to the main rule.
    setClaudeCodeSlot: async (scenario: string, slot: string, enabled: boolean): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/claude-code/slots/{slot}', {
            headers,
            params: {path: {scenario, slot}},
            body: {enabled},
        }));
    },

    // Overwrite the default Claude Code routing with a copy of a profile's (id or name).
    copyProfileToClaudeCodeDefault: async (profile: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/scenario/{scenario}/claude-code/use-profile', {
            headers,
            params: {path: {scenario: 'claude_code'}},
            body: {profile},
        }));
    },

    // Scenario descriptors (includes supports_profiles flag)
    getScenarioDescriptors: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario-descriptors', {headers}));
    },
};
