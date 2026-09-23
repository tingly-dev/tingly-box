// Shared config-loading hook for the Guardrails pages (Rules + Groups).
// Handles the initial config load, the builtins/scenario fetch (Rules only),
// the optional registry card load (Rules only), and the default-group
// bootstrap effect that both pages previously duplicated near-verbatim.
import { useEffect, useState } from 'react';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { DEFAULT_GROUP_ID, type GuardrailsPolicy, type PolicyGroup, type RegistryPolicyEntry } from './types';

type UseGuardrailsConfigOptions = {
    // Also fetch built-in policies and track supported scenarios (Rules page).
    loadBuiltins?: boolean;
    // Load the remote policy registry (Rules page Download Management card).
    registry?: boolean;
    // Defer default-group bootstrap until scenarios are known (Rules page).
    requireScenarios?: boolean;
    // Groups page historically toasts this message with a trailing period.
    defaultGroupErrorMessage?: string;
};

export const useGuardrailsConfig = (options: UseGuardrailsConfigOptions = {}) => {
    const {
        loadBuiltins = false,
        registry = false,
        requireScenarios = false,
        defaultGroupErrorMessage = 'Failed to create default group',
    } = options;
    const notify = useNotify();
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [supportedScenarios, setSupportedScenarios] = useState<string[]>([]);
    const [groups, setGroups] = useState<PolicyGroup[]>([]);
    const [policies, setPolicies] = useState<GuardrailsPolicy[]>([]);
    const [builtins, setBuiltins] = useState<GuardrailsPolicy[]>([]);
    const [initializingDefaultGroup, setInitializingDefaultGroup] = useState(false);
    const [registryURL, setRegistryURL] = useState('');
    const [registryPolicies, setRegistryPolicies] = useState<RegistryPolicyEntry[]>([]);
    const [registryLoading, setRegistryLoading] = useState(true);
    const [registryLoadError, setRegistryLoadError] = useState<string | null>(null);

    const loadPolicies = async (silent = false) => {
        try {
            if (!silent) {
                setLoading(true);
            }
            const [guardrailsConfig, builtinResponse] = await Promise.allSettled([
                api.getGuardrailsConfig(),
                loadBuiltins ? api.getGuardrailsBuiltins() : Promise.resolve(null),
            ]);

            if (guardrailsConfig.status !== 'fulfilled' || builtinResponse.status !== 'fulfilled') {
                throw (guardrailsConfig.status === 'rejected'
                    ? (guardrailsConfig as PromiseRejectedResult).reason
                    : (builtinResponse as PromiseRejectedResult).reason);
            }

            const config = guardrailsConfig.value?.config || {};
            const scenarios = Array.isArray(guardrailsConfig.value?.supported_scenarios)
                ? guardrailsConfig.value.supported_scenarios.filter((value: string) => value && value !== '_global')
                : [];
            if (loadBuiltins) {
                setSupportedScenarios(scenarios);
                setBuiltins(Array.isArray(builtinResponse.value?.policies) ? builtinResponse.value.policies : []);
            }
            setGroups(Array.isArray(config.groups) ? config.groups : []);
            setPolicies(Array.isArray(config.policies) ? config.policies : []);
            setLoadError(null);
        } catch (error) {
            console.error('Failed to load guardrails config:', error);
            setGroups([]);
            setPolicies([]);
            if (loadBuiltins) {
                setBuiltins([]);
                setSupportedScenarios([]);
            }
            setLoadError('Failed to load guardrails config');
        } finally {
            if (!silent) {
                setLoading(false);
            }
        }
    };

    const loadRegistry = async (force = false) => {
        try {
            setRegistryLoading(true);
            setRegistryLoadError(null);
            const response = await api.getGuardrailsRegistry(force);
            if (response?.success === false) {
                throw new Error(response?.error || 'Failed to load registry');
            }
            setRegistryURL(response?.url || '');
            setRegistryPolicies(Array.isArray(response?.policies) ? response.policies : []);
            setRegistryLoadError(null);
        } catch (error: any) {
            setRegistryURL('');
            setRegistryPolicies([]);
            setRegistryLoadError(error?.message || 'Failed to load registry');
        } finally {
            setRegistryLoading(false);
        }
    };

    useEffect(() => {
        loadPolicies();
    }, []);

    useEffect(() => {
        if (registry) {
            loadRegistry();
        }
    }, [registry]);

    useEffect(() => {
        if (loading || loadError || initializingDefaultGroup) {
            return;
        }
        if (requireScenarios && supportedScenarios.length === 0) {
            return;
        }
        if (groups.some((group) => group.id === DEFAULT_GROUP_ID)) {
            return;
        }

        const ensureDefaultGroup = async () => {
            try {
                setInitializingDefaultGroup(true);
                const result = await api.createGuardrailsGroup({
                    id: DEFAULT_GROUP_ID,
                    name: 'Default',
                    enabled: true,
                    severity: 'high',
                });
                if (!result?.success) {
                    notify.error(result?.error || defaultGroupErrorMessage);
                    return;
                }
                await loadPolicies(true);
            } catch (error: any) {
                notify.error(error?.message || defaultGroupErrorMessage);
            } finally {
                setInitializingDefaultGroup(false);
            }
        };

        ensureDefaultGroup();
    }, [defaultGroupErrorMessage, groups, initializingDefaultGroup, loadError, loading, notify, requireScenarios, supportedScenarios]);

    return {
        loading,
        loadError,
        groups,
        policies,
        builtins,
        supportedScenarios,
        loadPolicies,
        initializingDefaultGroup,
        registryURL,
        registryPolicies,
        registryLoading,
        registryLoadError,
        loadRegistry,
    };
};
