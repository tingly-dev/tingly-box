// Scenario control-plane API: per-scenario config + flags + descriptors.
import {controlApi} from './openapi';

export const scenarioApi = {
    // Scenario API
    getScenarioConfig: async (scenario: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}', {
            headers,
            params: {path: {scenario}},
        }));
    },

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

    getScenarioStringFlag: async (scenario: string, flag: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/string-flag/{flag}', {
            headers,
            params: {path: {scenario, flag}},
        }));
    },

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

    // Whether the default Claude Code routing is switched to a profile's.
    getClaudeCodeDefaultSwitch: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/claude-code/default-switch', {
            headers,
            params: {path: {scenario: 'claude_code'}},
        }));
    },

    // Make the default Claude Code routing a copy of a profile's (id or name).
    switchClaudeCodeDefaultToProfile: async (profile: string): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/claude-code/default-switch', {
            headers,
            params: {path: {scenario: 'claude_code'}},
            body: {profile},
        }));
    },

    // Put the default Claude Code routing back as it was before the first switch.
    restoreClaudeCodeDefault: async (): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/scenario/{scenario}/claude-code/default-switch', {
            headers,
            params: {path: {scenario: 'claude_code'}},
        }));
    },

    // Scenario descriptors (includes supports_profiles flag)
    getScenarioDescriptors: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario-descriptors', {headers}));
    },
};
