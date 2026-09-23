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

    // Scenario descriptors (includes supports_profiles flag)
    getScenarioDescriptors: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/scenario-descriptors', {headers}));
    },
};
