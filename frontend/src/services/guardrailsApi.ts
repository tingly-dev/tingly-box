// Guardrails control-plane API: config/builtins/registry, credentials,
// history, policies, groups, and fragment import/export.
import {controlApi} from './openapi';

export const guardrailsApi = {
    // Guardrails API
    getGuardrailsConfig: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/config', {headers}));
    },
    getGuardrailsBuiltins: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/builtins', {headers}));
    },
    getGuardrailsRegistry: async (forceRefresh = false): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/registry', {
            headers,
            params: {query: {refresh: forceRefresh ? '1' : undefined}},
        }));
    },
    installGuardrailsRegistryPolicy: async (id: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/registry/install', {
            headers,
            body: {id},
        }));
    },
    getGuardrailsCredentials: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/credentials', {headers}));
    },
    getGuardrailsCredential: async (credentialId: string): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/credential/{id}', {
            headers,
            params: {path: {id: credentialId}},
        }));
    },
    createGuardrailsCredential: async (payload: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/credential', {
            headers,
            body: payload,
        }));
    },
    updateGuardrailsCredential: async (credentialId: string, payload: any): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/guardrails/credential/{id}', {
            headers,
            params: {path: {id: credentialId}},
            body: payload,
        }));
    },
    deleteGuardrailsCredential: async (credentialId: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/credential/{id}', {
            headers,
            params: {path: {id: credentialId}},
        }));
    },
    getGuardrailsHistory: async (): Promise<any> => {
        return controlApi((client, headers) => client.GET('/api/v1/guardrails/history', {headers}));
    },
    clearGuardrailsHistory: async (): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/history', {headers}));
    },
    createGuardrailsPolicy: async (payload: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/policy', {
            headers,
            body: payload,
        }));
    },
    updateGuardrailsPolicy: async (policyId: string, payload: any): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/guardrails/policy/{id}', {
            headers,
            params: {path: {id: policyId}},
            body: payload,
        }));
    },
    deleteGuardrailsPolicy: async (policyId: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/policy/{id}', {
            headers,
            params: {path: {id: policyId}},
        }));
    },
    createGuardrailsGroup: async (payload: any): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/group', {
            headers,
            body: payload,
        }));
    },
    updateGuardrailsGroup: async (groupId: string, payload: any): Promise<any> => {
        return controlApi((client, headers) => client.PUT('/api/v1/guardrails/group/{id}', {
            headers,
            params: {path: {id: groupId}},
            body: payload,
        }));
    },
    deleteGuardrailsGroup: async (groupId: string): Promise<any> => {
        return controlApi((client, headers) => client.DELETE('/api/v1/guardrails/group/{id}', {
            headers,
            params: {path: {id: groupId}},
        }));
    },

    importGuardrailsFragment: async (content: string, fileName?: string): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/fragment/import', {
            headers,
            body: {content, file_name: fileName},
        }));
    },
    exportGuardrailsFragments: async (paths: string[]): Promise<any> => {
        return controlApi((client, headers) => client.POST('/api/v1/guardrails/fragment/export', {
            headers,
            body: {paths},
        }));
    },
};
