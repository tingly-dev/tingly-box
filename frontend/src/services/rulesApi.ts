// Rules control-plane API (per-scenario rule CRUD + flag registry).
import {controlApi} from './openapi';

export const rulesApi = {
    // Rules API
    getRules: async (scenario: string): Promise<any> => {
        if (!scenario.trim()) {
            return {success: false, error: 'Scenario is required', data: []};
        }
        const result = await controlApi((client, headers) => client.GET('/api/v1/rules', {
            headers,
            params: {query: {scenario}}
        }));
        return result?.success === false ? {...result, data: result.data ?? []} : result;
    },

    getRule: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/rule/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    createRule: async (uuid: string, data: any): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/rule', {
            headers,
            body: data
        })),

    updateRule: async (uuid: string, data: any): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v1/rule/{uuid}', {
            headers,
            params: {path: {uuid}},
            body: data
        })),

    deleteRule: async (uuid: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v1/rule/{uuid}', {
            headers,
            params: {path: {uuid}}
        })),

    getRuleFlagRegistry: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/rule/flags/registry', {headers}));
    },
};
