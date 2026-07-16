import { api } from '@/services/api';

const GLOBAL_SCENARIO = '_global';

type ScenarioConfig = {
    scenario?: string;
    extensions?: Record<string, unknown>;
    Extensions?: Record<string, unknown>;
    [key: string]: unknown;
};

const requireConfig = (result: any): ScenarioConfig => {
    if (!result?.success || !result.data) {
        throw new Error(result?.error || 'Failed to load global experimental extensions');
    }
    return result.data as ScenarioConfig;
};

const extensionsOf = (config: ScenarioConfig): Record<string, unknown> => ({
    ...(config.Extensions || {}),
    ...(config.extensions || {}),
});

export const experimentalExtensions = {
    getBoolean: async (key: string): Promise<boolean> => {
        const config = requireConfig(await api.getScenarioConfig(GLOBAL_SCENARIO));
        return extensionsOf(config)[key] === true;
    },

    setBoolean: async (key: string, value: boolean): Promise<void> => {
        const config = requireConfig(await api.getScenarioConfig(GLOBAL_SCENARIO));
        const result = await api.setScenarioConfig(GLOBAL_SCENARIO, {
            ...config,
            scenario: GLOBAL_SCENARIO,
            extensions: { ...extensionsOf(config), [key]: value },
        });
        if (!result?.success) {
            throw new Error(result?.error || `Failed to save experimental extension: ${key}`);
        }
    },
};
