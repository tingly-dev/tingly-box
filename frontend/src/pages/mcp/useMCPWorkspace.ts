import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import type { MCPConfigResponse, MCPRoutingSnapshot, MCPRuntimeConfig, MCPSourceConfig } from './types';

// Configuration, the applied routing snapshot and execution state, shared by the
// Tools and Profiles pages. Routing is what the gateway actually does, so both
// pages read availability from it rather than recomputing it from the config.
export function useMCPWorkspace() {
    const notify = useNotify();
    const flags = useFeatureFlags();
    const [config, setConfig] = useState<MCPRuntimeConfig>({});
    const [routing, setRouting] = useState<MCPRoutingSnapshot | null>(null);
    const [enabled, setEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const [checking, setChecking] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [routeError, setRouteError] = useState('');
    const routeGeneration = useRef(0);

    const refreshRoutes = useCallback(async () => {
        const generation = ++routeGeneration.current;
        setChecking(true);
        try {
            const response = await api.getMCPRouting();
            if (generation !== routeGeneration.current) return;
            if (!response.success) throw new Error(response.error || 'Could not check tool connections');
            setRouting(response.routing);
            setEnabled(response.enabled);
            setRouteError('');
        } catch (e) {
            if (generation === routeGeneration.current) {
                setRouting(null);
                setRouteError(e instanceof Error ? e.message : String(e));
            }
        } finally {
            if (generation === routeGeneration.current) setChecking(false);
        }
    }, []);

    const acceptConfig = useCallback(
        async (response: MCPConfigResponse) => {
            if (!response?.success || !response.config)
                throw new Error(response?.error || 'Could not save MCP configuration');
            setConfig(response.config);
            setRouting(null);
            setEnabled(response.enabled);
            await refreshRoutes();
        },
        [refreshRoutes]
    );

    const reload = useCallback(async () => acceptConfig(await api.getMCPConfig()), [acceptConfig]);

    useEffect(() => {
        let active = true;
        void api
            .getMCPConfig()
            .then((response: MCPConfigResponse) => {
                if (!active) return;
                if (!response?.success || !response.config)
                    throw new Error(response?.error || 'Could not load MCP configuration');
                setConfig(response.config);
                setEnabled(response.enabled);
                setLoading(false);
                void refreshRoutes();
            })
            .catch((e) => {
                if (active) {
                    setError(e instanceof Error ? e.message : String(e));
                    setLoading(false);
                }
            });
        return () => {
            active = false;
            routeGeneration.current++;
        };
    }, [refreshRoutes]);

    const run = async (action: () => Promise<void>) => {
        setBusy(true);
        try {
            await action();
        } catch (e) {
            notify.error(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const saveSource = async (patch: MCPSourceConfig) => {
        const exists = (config.sources || []).some((s) => s.id === patch.id);
        await acceptConfig(await (exists ? api.patchMCPSource(patch.id!, patch) : api.createMCPSource(patch)));
    };

    const enableExecution = () =>
        run(async () => {
            const response = await api.setScenarioFlag('_global', 'mcp', true);
            if (!response?.success) throw new Error(response?.error || 'Enable failed');
            await flags.refresh();
            await reload();
        });

    return {
        config,
        routing,
        enabled,
        loading,
        checking,
        busy,
        error,
        routeError,
        refreshRoutes,
        acceptConfig,
        reload,
        run,
        saveSource,
        enableExecution,
    };
}

export type MCPWorkspace = ReturnType<typeof useMCPWorkspace>;
