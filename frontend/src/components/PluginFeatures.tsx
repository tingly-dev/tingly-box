import { Box, CircularProgress, Typography } from '@mui/material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';
import { ConfigRow } from './ConfigRow';
import {
    PluginToggleButton,
    RecordingV2Control,
    SessionAffinityControl,
    ThinkingEffortControl,
    UserAgentControl,
    VisionProxyControl,
} from './flags';
import type { VisionService } from './flags';
import type { Provider } from '@/types/provider';

export interface PluginFeaturesProps {
    scenario: string;
}

interface PluginFeatureConfig {
    key: string;
    label: string;
    description: string;
    scenarios?: readonly string[];
}

// Scenario-level boolean plugins. Only flags that genuinely have a
// scenario-level default belong here. `clean_header` was deliberately
// dropped: it is now rule-only (backend `SetScenarioFlag` rejects it as an
// unknown flag, so the toggle could never persist) — it lives on the per-rule
// Plugins card instead. See .design/rule-flags.md §4 / §12.
const PLUGIN_FEATURES: PluginFeatureConfig[] = [
    { key: 'smart_compact', label: 'Smart Compact', description: 'Remove thinking blocks from conversation history to reduce context' },
];

const VISION_PROXY_SERVICE_KEY = 'vision_proxy_service';

const PluginFeatures: React.FC<PluginFeaturesProps> = ({ scenario }) => {
    const baseScenario = scenario.includes(':') ? scenario.split(':')[0] : scenario;

    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [effort, setEffort] = useState<string>('');
    const [recordV2Mode, setRecordV2Mode] = useState<string>('');
    const [userAgent, setUserAgent] = useState<string>('');
    const [sessionAffinity, setSessionAffinity] = useState<number>(0);
    const [loading, setLoading] = useState(true);
    const [updating, setUpdating] = useState<Record<string, boolean>>({});

    const [visionService, setVisionService] = useState<VisionService | null>(null);
    const [providers, setProviders] = useState<Provider[]>([]);

    const visibleFeatures = PLUGIN_FEATURES.filter(f => !f.scenarios || f.scenarios.includes(baseScenario as any));

    const loadData = async () => {
        try {
            setLoading(true);

            const [effortResult, recordV2Result, uaResult, affinityResult, cfgResult, providersResult, ...featureResults] =
                await Promise.all([
                    api.getScenarioStringFlag(scenario, 'thinking_effort'),
                    api.getScenarioStringFlag(scenario, 'recording_v2'),
                    api.getScenarioStringFlag(scenario, 'custom_user_agent'),
                    api.getScenarioIntFlag(scenario, 'session_affinity'),
                    api.getScenarioConfig(scenario),
                    api.getProviders(),
                    ...visibleFeatures.map(f => api.getScenarioFlag(scenario, f.key)),
                ]);

            if (effortResult?.success && effortResult?.data?.value !== undefined) {
                setEffort(effortResult.data.value);
            }
            if (recordV2Result?.success && recordV2Result?.data?.value !== undefined) {
                setRecordV2Mode(recordV2Result.data.value);
            }
            if (uaResult?.success && uaResult?.data?.value !== undefined) {
                setUserAgent(uaResult.data.value);
            }
            if (affinityResult?.success && affinityResult?.data?.value !== undefined) {
                setSessionAffinity(affinityResult.data.value);
            }

            const ext = cfgResult?.data?.extensions || cfgResult?.data?.Extensions;
            const svc = ext?.[VISION_PROXY_SERVICE_KEY];
            setVisionService(svc?.provider && svc?.model ? { provider: svc.provider, model: svc.model } : null);

            if (providersResult?.success && Array.isArray(providersResult.data)) {
                setProviders(providersResult.data);
            }

            const newFeatures: Record<string, boolean> = {};
            visibleFeatures.forEach((f, i) => {
                newFeatures[f.key] = featureResults[i]?.success && featureResults[i]?.data?.value !== undefined
                    ? featureResults[i].data.value
                    : false;
            });
            setFeatures(newFeatures);
        } catch (error) {
            console.error('Failed to load scenario features:', error);
        } finally {
            setLoading(false);
        }
    };

    const setFeature = (featureKey: string, value: boolean) => {
        if (updating[featureKey]) return;
        setUpdating(prev => ({ ...prev, [featureKey]: true }));
        api.setScenarioFlag(scenario, featureKey, value)
            .then(result => result.success ? setFeatures(prev => ({ ...prev, [featureKey]: value })) : loadData())
            .catch(() => loadData())
            .finally(() => setUpdating(prev => ({ ...prev, [featureKey]: false })));
    };

    // Scenario string-flags share one optimistic save flow; build the setters
    // from a single factory keyed by flag name (also the in-flight key).
    const makeStringFlagSetter = (
        flagKey: string,
        current: string,
        setLocal: (value: string) => void,
    ) => (next: string) => {
        if (updating[flagKey] || next === current) return;
        setUpdating(prev => ({ ...prev, [flagKey]: true }));
        api.setScenarioStringFlag(scenario, flagKey, next)
            .then(result => (result.success ? setLocal(next) : loadData()))
            .catch(() => loadData())
            .finally(() => setUpdating(prev => ({ ...prev, [flagKey]: false })));
    };

    const setEffortLevel = makeStringFlagSetter('thinking_effort', effort, setEffort);
    const setRecordV2 = makeStringFlagSetter('recording_v2', recordV2Mode, setRecordV2Mode);
    const setUserAgentValue = makeStringFlagSetter('custom_user_agent', userAgent, setUserAgent);

    const handleSessionAffinityChange = (value: number) => {
        if (updating.session_affinity || value === sessionAffinity) return;
        setUpdating(prev => ({ ...prev, session_affinity: true }));
        api.setScenarioIntFlag(scenario, 'session_affinity', value)
            .then(result => result.success ? setSessionAffinity(value) : loadData())
            .catch(() => loadData())
            .finally(() => setUpdating(prev => ({ ...prev, session_affinity: false })));
    };

    const handleVisionChange = async (next: VisionService | null) => {
        setUpdating(prev => ({ ...prev, vision_proxy_service: true }));
        try {
            const cfgResult = await api.getScenarioConfig(scenario);
            const cfg = cfgResult?.data || {};
            const extensions = { ...(cfg.extensions || cfg.Extensions || {}) };
            if (next) {
                extensions[VISION_PROXY_SERVICE_KEY] = next;
            } else {
                delete extensions[VISION_PROXY_SERVICE_KEY];
            }
            const result = await api.setScenarioConfig(scenario, { ...cfg, scenario, extensions });
            if (result?.success) {
                setVisionService(next);
            } else {
                loadData();
            }
        } catch {
            loadData();
        } finally {
            setUpdating(prev => ({ ...prev, vision_proxy_service: false }));
        }
    };

    useEffect(() => {
        loadData();
    }, [scenario]);

    if (loading) {
        return (
            <Box sx={{ display: 'flex', flexDirection: 'column', py: 2, gap: 2, alignItems: 'center', justifyContent: 'center', minHeight: 100 }}>
                <CircularProgress size={24} />
                <Typography variant="body2" color="text.secondary">Loading features...</Typography>
            </Box>
        );
    }

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <ConfigRow
                tabs={[
                    {
                        key: 'plugins',
                        label: 'Plugins',
                        content: (
                            <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 1.5, rowGap: 1, width: '100%' }}>
                                <ThinkingEffortControl
                                    value={effort}
                                    disabled={updating.thinking_effort}
                                    onChange={setEffortLevel}
                                />
                                {visibleFeatures.map(feature => (
                                    <PluginToggleButton
                                        key={feature.key}
                                        label={feature.label}
                                        description={feature.description}
                                        value={features[feature.key] || false}
                                        disabled={updating[feature.key] || false}
                                        onChange={v => setFeature(feature.key, v)}
                                    />
                                ))}
                                <VisionProxyControl
                                    value={visionService}
                                    providers={providers}
                                    disabled={updating.vision_proxy_service || false}
                                    onChange={handleVisionChange}
                                />
                                <RecordingV2Control
                                    value={recordV2Mode}
                                    disabled={updating.recording_v2 || false}
                                    onChange={setRecordV2}
                                />
                                <UserAgentControl
                                    value={userAgent}
                                    disabled={updating.custom_user_agent || false}
                                    onChange={setUserAgentValue}
                                />
                                <SessionAffinityControl
                                    value={sessionAffinity}
                                    onChange={handleSessionAffinityChange}
                                    disabled={updating.session_affinity || false}
                                />
                            </Box>
                        ),
                    },
                ]}
                activeTab="plugins"
                onTabChange={() => {}}
                maxWidth="responsive"
            />
        </Box>
    );
};

export default PluginFeatures;
