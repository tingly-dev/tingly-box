import {
    IconCheck,
    IconChevronDown,
    IconCircleFilled,
} from '@tabler/icons-react';
import {
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    Typography,
} from '@mui/material';
import type { SelectChangeEvent } from '@mui/material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';
import { ConfigRow } from './ConfigRow';
import ModelSelectDialog from './ModelSelectDialog';
import type { ProviderSelectTabOption } from './ModelSelectDialog';
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

const PLUGIN_FEATURES: PluginFeatureConfig[] = [
    { key: 'smart_compact', label: 'Smart Compact', description: 'Remove thinking blocks from conversation history to reduce context' },
    { key: 'clean_header', label: 'Clean Header', description: 'Remove Claude Code billing header from system messages', scenarios: ['claude_code'] as const },
    // Vision Proxy is not in this list — see renderVisionProxyButton.
    // { key: 'anthropic_beta', label: 'Beta', description: 'Enable Anthropic beta features (e.g. extended thinking)', scenarios: ['claude_code'] as const },
];

const VISION_PROXY_SERVICE_KEY = 'vision_proxy_service';

const EFFORT_LEVELS = [
    { value: '', label: 'By Client', description: 'Pass the client\'s thinking config through unchanged' },
    { value: 'off', label: 'Off', description: 'Force extended thinking disabled' },
    { value: 'low', label: 'Low', description: '~1K tokens — Fast' },
    { value: 'medium', label: 'Medium', description: '~5K tokens — Balanced' },
    { value: 'high', label: 'High', description: '~20K tokens — Deep' },
    { value: 'max', label: 'Max', description: '~32K tokens — Max quality' },
] as const;

const RECORD_V2_MODES = [
    { value: '', label: 'Off', description: 'Recording disabled' },
    { value: 'request', label: 'Request Only', description: 'Record the final outbound request only' },
    { value: 'request_response', label: 'Request + Response', description: 'Record the final outbound request and final response' },
    { value: 'staged_request_response', label: 'Request + Transform + Response', description: 'Record original request, transformed request, and final response' },
] as const;

const PluginFeatures: React.FC<PluginFeaturesProps> = ({ scenario }) => {
    const baseScenario = scenario.includes(':') ? scenario.split(':')[0] : scenario;

    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [effort, setEffort] = useState<string>('');
    const [recordV2Mode, setRecordV2Mode] = useState<string>('');
    const [loading, setLoading] = useState(true);
    const [updating, setUpdating] = useState<Record<string, boolean>>({});
    const [menuAnchor, setMenuAnchor] = useState<Record<string, HTMLElement | null>>({});

    const [visionService, setVisionService] = useState<{ provider: string; model: string } | null>(null);
    const [providers, setProviders] = useState<Provider[]>([]);
    const [visionPickerOpen, setVisionPickerOpen] = useState(false);

    const providerNameFor = (uuid: string) => providers.find(p => p.uuid === uuid)?.name || uuid;

    const visibleFeatures = PLUGIN_FEATURES.filter(f => !f.scenarios || f.scenarios.includes(baseScenario as any));

    const loadData = async () => {
        try {
            setLoading(true);
            const effortResult = await api.getScenarioStringFlag(scenario, 'thinking_effort');
            if (effortResult?.success && effortResult?.data?.value !== undefined) {
                setEffort(effortResult.data.value);
            }

            const featureResults = await Promise.all(
                visibleFeatures.map(f => api.getScenarioFlag(scenario, f.key))
            );
            const newFeatures: Record<string, boolean> = {};
            visibleFeatures.forEach((f, i) => {
                if (featureResults[i]?.success && featureResults[i]?.data?.value !== undefined) {
                    newFeatures[f.key] = featureResults[i].data.value;
                } else {
                    newFeatures[f.key] = false;
                }
            });
            setFeatures(newFeatures);

            const recordV2Result = await api.getScenarioStringFlag(scenario, 'recording_v2');
            if (recordV2Result?.success && recordV2Result?.data?.value !== undefined) {
                setRecordV2Mode(recordV2Result.data.value);
            }

            // Vision proxy service (provider + model) lives in ScenarioConfig.Extensions
            const cfgResult = await api.getScenarioConfig(scenario);
            const ext = cfgResult?.data?.extensions || cfgResult?.data?.Extensions;
            const svc = ext?.[VISION_PROXY_SERVICE_KEY];
            if (svc && svc.provider && svc.model) {
                setVisionService({ provider: svc.provider, model: svc.model });
            } else {
                setVisionService(null);
            }

            const providersResult = await api.getProviders();
            if (providersResult?.success && Array.isArray(providersResult.data)) {
                setProviders(providersResult.data);
            }
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
            .then((result) => {
                if (result.success) {
                    setFeatures(prev => ({ ...prev, [featureKey]: value }));
                } else {
                    loadData();
                }
            })
            .catch(() => loadData())
            .finally(() => setUpdating(prev => ({ ...prev, [featureKey]: false })));
    };

    const persistVisionService = async (next: { provider: string; model: string } | null) => {
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
            const updated = { ...cfg, scenario, extensions };
            const result = await api.setScenarioConfig(scenario, updated);
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

    const handleMenuOpen = (key: string, event: React.MouseEvent<HTMLElement>) => {
        setMenuAnchor(prev => ({ ...prev, [key]: event.currentTarget }));
    };

    const handleMenuClose = (key: string) => {
        setMenuAnchor(prev => ({ ...prev, [key]: null }));
    };

    const setEffortLevel = (level: string) => {
        if (updating.effort || level === effort) return;
        setUpdating(prev => ({ ...prev, effort: true }));
        api.setScenarioStringFlag(scenario, 'thinking_effort', level)
            .then((result) => {
                if (result.success) {
                    setEffort(level);
                } else {
                    loadData();
                }
            })
            .catch(() => loadData())
            .finally(() => setUpdating(prev => ({ ...prev, effort: false })));
    };

    const handleRecordV2Change = (event: SelectChangeEvent<string>) => {
        const newMode = event.target.value;
        if (updating.recordV2 || newMode === recordV2Mode) return;
        setUpdating(prev => ({ ...prev, recordV2: true }));
        api.setScenarioStringFlag(scenario, 'recording_v2', newMode)
            .then((result) => {
                if (result.success) {
                    setRecordV2Mode(newMode);
                } else {
                    loadData();
                }
            })
            .catch(() => loadData())
            .finally(() => setUpdating(prev => ({ ...prev, recordV2: false })));
    };

    useEffect(() => {
        loadData();
    }, [scenario]);

    const renderEffortButton = () => {
        const currentLevel = EFFORT_LEVELS.find(l => l.value === effort);
        const isActive = effort !== '';
        return (
            <Tooltip title={`Thinking: ${currentLevel?.description || 'By Client'}`} placement="right" arrow>
                <Button
                    size="small"
                    variant="outlined"
                    onClick={(e) => !updating.effort && handleMenuOpen('effort', e)}
                    disabled={updating.effort}
                    endIcon={<IconChevronDown size={18} />}
                    sx={{
                        minWidth: 110,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        bgcolor: isActive ? 'primary.main' : 'transparent',
                        color: isActive ? 'primary.contrastText' : 'text.primary',
                        border: isActive ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: updating.effort ? 0.6 : 1,
                        '&:hover': { bgcolor: isActive ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    Thinking: {currentLevel?.label || 'By Client'}
                </Button>
            </Tooltip>
        );
    };

    // Custom button instead of a PLUGIN_FEATURES entry: enable and model
    // pick are the same act here (picking a model IS the on state), so the
    // generic On/Off toggle would add a redundant control.
    const renderVisionProxyButton = () => {
        const isUpdating = updating.vision_proxy_service || false;
        const isEnabled = !!visionService;
        const label = isEnabled ? visionService!.model : 'Off';
        const tooltip = isEnabled
            ? `Vision Proxy: images described by ${providerNameFor(visionService!.provider)} / ${visionService!.model}`
            : 'Vision Proxy: describe images via a vision-capable model so text-only downstreams can read them';
        return (
            <Tooltip title={tooltip} placement="right" arrow>
                <Button
                    size="small"
                    variant="outlined"
                    onClick={(e) => !isUpdating && handleMenuOpen('vision_proxy', e)}
                    disabled={isUpdating}
                    endIcon={<IconChevronDown size={18} />}
                    sx={{
                        minWidth: 100,
                        maxWidth: 260,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        '& .MuiButton-endIcon': { flexShrink: 0 },
                        bgcolor: isEnabled ? 'primary.main' : 'transparent',
                        color: isEnabled ? 'primary.contrastText' : 'text.primary',
                        fontWeight: isEnabled ? 600 : 400,
                        border: isEnabled ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: isUpdating ? 0.6 : 1,
                        '&:hover': { bgcolor: isEnabled ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                        Vision Proxy: {label}
                    </Box>
                </Button>
            </Tooltip>
        );
    };

    const renderPluginButtons = () => (
        <>
            {visibleFeatures.map((feature) => {
                const isEnabled = features[feature.key] || false;
                const isUpdating = updating[feature.key] || false;
                return (
                    <Tooltip key={feature.key} title={`${feature.label}: ${feature.description} (${isEnabled ? 'On' : 'Off'})`} placement="right" arrow>
                        <Button
                            size="small"
                            variant="outlined"
                            onClick={(e) => !isUpdating && handleMenuOpen(feature.key, e)}
                            disabled={isUpdating}
                            endIcon={<IconChevronDown size={18} />}
                            sx={{
                                minWidth: 100,
                                textTransform: 'none',
                                whiteSpace: 'nowrap',
                                bgcolor: isEnabled ? 'primary.main' : 'transparent',
                                color: isEnabled ? 'primary.contrastText' : 'text.primary',
                                fontWeight: isEnabled ? 600 : 400,
                                border: isEnabled ? 'none' : '1px solid',
                                borderColor: 'divider',
                                opacity: isUpdating ? 0.6 : 1,
                                '&:hover': { bgcolor: isEnabled ? 'primary.dark' : 'action.selected' },
                            }}
                        >
                            {feature.label}: {isEnabled ? 'On' : 'Off'}
                        </Button>
                    </Tooltip>
                );
            })}
        </>
    );

    const renderRecordV2Button = () => {
        const currentRecordMode = RECORD_V2_MODES.find(m => m.value === recordV2Mode);
        const isRecordV2Enabled = recordV2Mode !== '';
        const isUpdatingRecordV2 = updating.recordV2 || false;
        return (
            <Tooltip
                title={`Recording V2: ${currentRecordMode?.description || 'Disabled'}${isRecordV2Enabled ? ' (enabled)' : ' (disabled)'}`}
                placement="right"
                arrow
            >
                <Button
                    size="small"
                    variant="outlined"
                    onClick={(e) => !isUpdatingRecordV2 && handleMenuOpen('recordV2', e)}
                    disabled={isUpdatingRecordV2}
                    endIcon={<IconChevronDown size={18} />}
                    sx={{
                        minWidth: 110,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        bgcolor: isRecordV2Enabled ? 'primary.main' : 'transparent',
                        color: isRecordV2Enabled ? 'primary.contrastText' : 'text.primary',
                        fontWeight: isRecordV2Enabled ? 600 : 400,
                        border: isRecordV2Enabled ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: isUpdatingRecordV2 ? 0.6 : 1,
                        '&:hover': { bgcolor: isRecordV2Enabled ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    <IconCircleFilled size={14} style={{ marginRight: '4px' }} />
                    Record: {currentRecordMode?.label || 'Off'}
                </Button>
            </Tooltip>
        );
    };

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
            {/* Single Plugin row: thinking controls first, then plugin features */}
            <ConfigRow
                tabs={[
                    {
                        key: 'plugin',
                        label: 'Plugin',
                        content: (
                            <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 1.5, rowGap: 1 }}>
                                {renderEffortButton()}
                                {renderPluginButtons()}
                                {renderVisionProxyButton()}
                                {renderRecordV2Button()}
                            </Box>
                        ),
                    },
                ]}
                activeTab="plugin"
                onTabChange={() => {}}
            />

            {/* Effort Menu */}
            <Menu
                anchorEl={menuAnchor['effort']}
                open={Boolean(menuAnchor['effort'])}
                onClose={() => handleMenuClose('effort')}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                {EFFORT_LEVELS.map((level) => (
                    <MenuItem
                        key={level.value}
                        selected={level.value === effort}
                        onClick={() => { setEffortLevel(level.value); handleMenuClose('effort'); }}
                        title={level.description}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                            <ListItemText primary={level.label} primaryTypographyProps={{ variant: 'body2' }} />
                            {level.value === effort && <IconCheck size={16} />}
                        </Box>
                    </MenuItem>
                ))}
            </Menu>

            {/* Plugin Feature Menus */}
            {visibleFeatures.map((feature) => {
                const isEnabled = features[feature.key] || false;
                return (
                    <Menu
                        key={feature.key}
                        anchorEl={menuAnchor[feature.key]}
                        open={Boolean(menuAnchor[feature.key])}
                        onClose={() => handleMenuClose(feature.key)}
                        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                    >
                        <MenuItem
                            selected={isEnabled}
                            onClick={() => { setFeature(feature.key, true); handleMenuClose(feature.key); }}
                            title={feature.description}
                        >
                            <ListItemText primary="On" primaryTypographyProps={{ variant: 'body2' }} />
                            {isEnabled && <IconCheck size={16} />}
                        </MenuItem>
                        <MenuItem
                            selected={!isEnabled}
                            onClick={() => { setFeature(feature.key, false); handleMenuClose(feature.key); }}
                            title={feature.description}
                        >
                            <ListItemText primary="Off" primaryTypographyProps={{ variant: 'body2' }} />
                            {!isEnabled && <IconCheck size={16} />}
                        </MenuItem>
                    </Menu>
                );
            })}

            {/* Vision Proxy Menu: Off / On(->model picker) */}
            <Menu
                anchorEl={menuAnchor['vision_proxy']}
                open={Boolean(menuAnchor['vision_proxy'])}
                onClose={() => handleMenuClose('vision_proxy')}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                <MenuItem
                    selected={!visionService}
                    onClick={() => { handleMenuClose('vision_proxy'); if (visionService) persistVisionService(null); }}
                >
                    <ListItemText primary="Off" primaryTypographyProps={{ variant: 'body2' }} />
                    {!visionService && <IconCheck size={16} />}
                </MenuItem>
                <MenuItem
                    selected={!!visionService}
                    onClick={() => { handleMenuClose('vision_proxy'); setVisionPickerOpen(true); }}
                >
                    <ListItemText
                        primary={visionService ? `On — ${visionService.model}` : 'On — pick a model…'}
                        secondary={visionService ? providerNameFor(visionService.provider) : 'Choose a vision-capable model'}
                        primaryTypographyProps={{ variant: 'body2' }}
                        secondaryTypographyProps={{ variant: 'caption' }}
                    />
                    {!!visionService && <IconCheck size={16} />}
                </MenuItem>
            </Menu>

            {/* Vision Proxy Service Picker */}
            <Dialog
                open={visionPickerOpen}
                onClose={() => setVisionPickerOpen(false)}
                maxWidth="lg"
                fullWidth
                PaperProps={{ sx: { height: '80vh' } }}
            >
                <DialogTitle sx={{ textAlign: 'center' }}>
                    <Typography variant="h6">Pick Vision Proxy Model</Typography>
                </DialogTitle>
                <DialogContent>
                    <ModelSelectDialog
                        providers={providers}
                        selectedProvider={visionService?.provider}
                        selectedModel={visionService?.model}
                        onSelected={async (option: ProviderSelectTabOption) => {
                            await persistVisionService({ provider: option.provider.uuid, model: option.model });
                            setVisionPickerOpen(false);
                        }}
                        onSelectionClear={async () => {
                            await persistVisionService(null);
                            setVisionPickerOpen(false);
                        }}
                    />
                </DialogContent>
            </Dialog>

            {/* Record V2 Menu */}
            <Menu
                anchorEl={menuAnchor['recordV2']}
                open={Boolean(menuAnchor['recordV2'])}
                onClose={() => handleMenuClose('recordV2')}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                {RECORD_V2_MODES.map((mode) => (
                    <MenuItem
                        key={mode.value}
                        selected={mode.value === recordV2Mode}
                        onClick={() => {
                            handleRecordV2Change({ target: { value: mode.value } } as SelectChangeEvent<string>);
                            handleMenuClose('recordV2');
                        }}
                        title={mode.description}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                            <ListItemText primary={mode.label} primaryTypographyProps={{ variant: 'body2' }} />
                            {mode.value === recordV2Mode && <IconCheck size={16} />}
                        </Box>
                    </MenuItem>
                ))}
            </Menu>
        </Box>
    );
};

export default PluginFeatures;
