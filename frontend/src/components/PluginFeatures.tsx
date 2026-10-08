import { Box, Button, CircularProgress, Popover, Stack, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import React, { useEffect, useState } from 'react';
import { KeyboardArrowDown as IconChevronDown } from '@/components/icons';
import { api } from '../services/api';
import { ConfigRow } from './ConfigRow';
import { VisionProxyControl } from './flags';
import type { VisionService } from './flags';
import { normalizePoints, RECORDING_POINTS } from './flags/RecordingV2Control';
import { EFFORT_LEVELS } from './flags/ThinkingEffortControl';
import type { Provider } from '@/types/provider';

interface PluginFeaturesProps {
    scenario: string;
}

// Scenario-level plugins shown here: Thinking, Vision Proxy and Record. The
// scenario's boolean plugins are gone — Smart Compact was removed (backend
// #1911), and `clean_header` is rule-only (see .design/rule-flags.md §12).

const VISION_PROXY_SERVICE_KEY = 'vision_proxy_service';

// Endpoints that don't speak the chat/completion shape. Thinking effort and
// Vision Proxy have no meaning
// for an embedding or image-generation endpoint, so we hide them there instead
// of showing dead controls. Kept as a blacklist so any new *chat* scenario
// automatically inherits the full plugin set. See UX principle #9 (reduce
// visual noise) / #1 (organize around the user's real question).
const NON_CHAT_SCENARIOS = new Set(['embed', 'imagegen', 'decisions']);

const PluginFeatures: React.FC<PluginFeaturesProps> = ({ scenario }) => {
    const baseScenario = scenario.includes(':') ? scenario.split(':')[0] : scenario;
    const isChatShaped = !NON_CHAT_SCENARIOS.has(baseScenario);

    const [effort, setEffort] = useState<string>('');
    const [recordV2Mode, setRecordV2Mode] = useState<string>('');
    const [loading, setLoading] = useState(true);
    const [updating, setUpdating] = useState<Record<string, boolean>>({});

    const [visionService, setVisionService] = useState<VisionService | null>(null);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [panelAnchor, setPanelAnchor] = useState<HTMLElement | null>(null);

    const loadData = async () => {
        try {
            setLoading(true);

            const [effortResult, recordV2Result, cfgResult, providersResult] =
                await Promise.all([
                    api.getScenarioStringFlag(scenario, 'thinking_effort'),
                    api.getScenarioStringFlag(scenario, 'recording_v2'),
                    api.getScenarioConfig(scenario),
                    api.getProviders(),
                ]);

            if (effortResult?.success && effortResult?.data?.value !== undefined) {
                setEffort(effortResult.data.value);
            }
            if (recordV2Result?.success && recordV2Result?.data?.value !== undefined) {
                setRecordV2Mode(recordV2Result.data.value);
            }

            const ext = cfgResult?.data?.extensions || cfgResult?.data?.Extensions;
            const svc = ext?.[VISION_PROXY_SERVICE_KEY];
            setVisionService(svc?.provider && svc?.model ? { provider: svc.provider, model: svc.model } : null);

            if (providersResult?.success && Array.isArray(providersResult.data)) {
                setProviders(providersResult.data);
            }
        } catch (error) {
            console.error('Failed to load scenario features:', error);
        } finally {
            setLoading(false);
        }
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

    // One strip summarizing every scenario plugin; clicking it opens a panel
    // to change them. Thinking and Vision Proxy — the ones people actually use —
    // always show in the summary; Record only once it is on.
    const recordPoints = normalizePoints(recordV2Mode);
    const offeredRecordPoints = recordPoints.filter(p => RECORDING_POINTS.some(o => o.value === p));
    // One segment per plugin: name, value. `active` = doing something beyond
    // the pass-through default; only then does the value light up.
    const summary: { key: string; label: string; value: string; active: boolean }[] = [];
    if (isChatShaped) {
        summary.push({
            key: 'thinking', label: 'Thinking',
            value: EFFORT_LEVELS.find(l => l.value === effort)?.label ?? effort, active: effort !== '',
        });
        summary.push({
            // Full name: "Vision" alone reads as "can the model see", not
            // "describe images through another model".
            key: 'vision', label: 'Vision Proxy',
            value: visionService?.model ?? 'Off', active: visionService !== null,
        });
    }
    if (offeredRecordPoints.length > 0 || !isChatShaped) {
        summary.push({
            key: 'record',
            label: 'Record',
            value: offeredRecordPoints.length
                ? RECORDING_POINTS.filter(p => offeredRecordPoints.includes(p.value)).map(p => p.short).join('+')
                : 'Off',
            active: offeredRecordPoints.length > 0,
        });
    }

    // Toggling a capture point keeps stored points this UI doesn't offer yet.
    const toggleRecordPoint = (point: string) => {
        const next = recordPoints.includes(point) ? recordPoints.filter(p => p !== point) : [...recordPoints, point];
        setRecordV2(normalizePoints(next.join(',')).join(','));
    };

    const panelRow = (label: string, control: React.ReactNode) => (
        <Box sx={{ display: 'grid', gridTemplateColumns: '104px 1fr', alignItems: 'center', columnGap: 1.5 }}>
            <Typography variant="body2" sx={{ color: 'text.secondary', fontWeight: 500 }}>{label}</Typography>
            <Box sx={{ minWidth: 0 }}>{control}</Box>
        </Box>
    );

    return (
        <ConfigRow
            tabs={[
                {
                    key: 'plugins',
                    label: 'Plugins',
                    content: loading ? (
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, color: 'text.secondary' }}>
                            <CircularProgress size={14} />
                            <Typography variant="body2">Loading…</Typography>
                        </Box>
                    ) : (
                        <>
                            <Button
                                variant="outlined"
                                color="inherit"
                                size="small"
                                endIcon={<IconChevronDown sx={{ fontSize: 18 }} />}
                                onClick={(e) => setPanelAnchor(e.currentTarget)}
                                sx={{ textTransform: 'none', borderColor: 'divider', maxWidth: '100%', py: 0.25, pl: 0.5, justifyContent: 'space-between' }}
                            >
                                <Box component="span" sx={{ display: 'flex', alignItems: 'center', minWidth: 0, overflow: 'hidden' }}>
                                    {summary.map((item, i) => (
                                        <Box
                                            component="span"
                                            key={item.key}
                                            sx={{
                                                display: 'inline-flex', alignItems: 'center', gap: 0.5, px: 1, whiteSpace: 'nowrap',
                                                borderLeft: i > 0 ? 1 : 0, borderColor: 'divider',
                                            }}
                                        >
                                            <Box component="span" sx={{ color: 'text.secondary', fontWeight: 400 }}>{item.label}</Box>
                                            <Box component="span" sx={{ fontWeight: 600, color: item.active ? 'primary.main' : 'text.secondary' }}>{item.value}</Box>
                                        </Box>
                                    ))}
                                </Box>
                            </Button>
                            <Popover
                                open={panelAnchor !== null}
                                anchorEl={panelAnchor}
                                onClose={() => setPanelAnchor(null)}
                                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                                slotProps={{ paper: { sx: { mt: 0.5, p: 2, width: 480, maxWidth: 'calc(100vw - 32px)' } } }}
                            >
                                <Stack spacing={1.75}>
                                    {isChatShaped && panelRow('Thinking', (
                                        <ToggleButtonGroup
                                            exclusive
                                            size="small"
                                            value={effort}
                                            disabled={updating.thinking_effort}
                                            onChange={(_, v: string | null) => v !== null && setEffortLevel(v)}
                                            sx={{ flexWrap: 'wrap' }}
                                        >
                                            {EFFORT_LEVELS.map(level => (
                                                <ToggleButton key={level.value || 'client'} value={level.value} title={level.description}
                                                    sx={{ textTransform: 'none', py: 0.25, px: 1 }}>
                                                    {level.label}
                                                </ToggleButton>
                                            ))}
                                        </ToggleButtonGroup>
                                    ))}
                                    {isChatShaped && panelRow('Vision Proxy', (
                                        <VisionProxyControl
                                            value={visionService}
                                            providers={providers}
                                            disabled={updating.vision_proxy_service || false}
                                            onChange={handleVisionChange}
                                            hideName
                                        />
                                    ))}
                                    {panelRow('Record', (
                                        <ToggleButtonGroup size="small" value={offeredRecordPoints} disabled={updating.recording_v2}>
                                            {RECORDING_POINTS.map(point => (
                                                <ToggleButton key={point.value} value={point.value} title={`${point.label}: ${point.description}`}
                                                    onClick={() => toggleRecordPoint(point.value)}
                                                    sx={{ textTransform: 'none', py: 0.25, px: 1, whiteSpace: 'nowrap' }}>
                                                    {point.label.split(' (')[0]}
                                                </ToggleButton>
                                            ))}
                                        </ToggleButtonGroup>
                                    ))}
                                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                        Apply to every rule of this agent; a rule's own Plugins override them.
                                    </Typography>
                                </Stack>
                            </Popover>
                        </>
                    ),
                },
            ]}
            activeTab="plugins"
            onTabChange={() => {}}
            maxWidth="responsive"
        />
    );
};

export default PluginFeatures;
