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

    // Claude Code model slots: which rule each slot (default/haiku/sonnet/
    // opus/fable/subagent) requests. `scenario` is claude_code or a profile.
    getClaudeCodeSlots: async (scenario: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario/{scenario}/claude-code/slots', {
            headers,
            params: {path: {scenario}},
        }));
    },

    // Bind a slot to a rule; an empty ruleUuid makes it follow the default slot.
    setClaudeCodeSlot: async (scenario: string, slot: string, ruleUuid: string): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/claude-code/slots/{slot}', {
            headers,
            params: {path: {scenario, slot}},
            body: {rule_uuid: ruleUuid},
        }));
    },

    // Give a slot its own rule (a copy of the rule it uses now) and bind it.
    createClaudeCodeSlotRule: async (scenario: string, slot: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/scenario/{scenario}/claude-code/slots/{slot}/rule', {
            headers,
            params: {path: {scenario, slot}},
        }));
    },

    applyClaudeCodeSlotPreset: async (scenario: string, preset: 'unified' | 'separate'): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/scenario/{scenario}/claude-code/slot-preset', {
            headers,
            params: {path: {scenario}},
            body: {preset},
        }));
    },

    // Scenario descriptors (includes supports_profiles flag)
    getScenarioDescriptors: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario-descriptors', {headers}));
    },
};
