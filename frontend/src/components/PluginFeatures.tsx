import { Check as CheckIcon, Science, SettingsSuggest, AutoMode, FlashOn, KeyboardArrowDown } from '@mui/icons-material';
import Psychology from '@mui/icons-material/Psychology';
import {
    Box,
    Button,
    CircularProgress,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';

export interface PluginFeaturesProps {
    scenario: string;
}

const PLUGIN_FEATURES = [
    { key: 'smart_compact', label: 'Smart Compact', description: 'Remove thinking blocks from conversation history to reduce context' },
    { key: 'recording', label: 'Recording', description: 'Record scenario-level request/response traffic for debugging' },
    { key: 'clean_header', label: 'Clean Header', description: 'Remove Claude Code billing header from system messages', scenarios: ['claude_code'] as const },
    { key: 'anthropic_beta', label: 'Beta', description: 'Enable Anthropic beta features (e.g. extended thinking)', scenarios: ['claude_code'] as const },
] as const;

const EFFORT_LEVELS = [
    { value: '', label: 'Default', description: 'Use model default' },
    { value: 'low', label: 'Low', description: '~1K tokens - Fast' },
    { value: 'medium', label: 'Medium', description: '~5K tokens - Balanced' },
    { value: 'high', label: 'High', description: '~20K tokens - Deep' },
    { value: 'max', label: 'Max', description: '~32K tokens - Max quality' },
] as const;

const THINKING_MODES = [
    { value: 'default', label: 'Default', description: 'Use client default config', icon: SettingsSuggest },
    { value: 'adaptive', label: 'Adaptive', description: 'Model decides when to use extended thinking', icon: AutoMode },
    { value: 'force', label: 'Force', description: 'Always use extended thinking if possible', icon: FlashOn },
] as const;

const PluginFeatures: React.FC<PluginFeaturesProps> = ({ scenario }) => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [effort, setEffort] = useState<string>('');
    const [thinkingMode, setThinkingMode] = useState<string>('default');
    const [loading, setLoading] = useState(true);
    const [updating, setUpdating] = useState<Record<string, boolean>>({});
    const [menuAnchor, setMenuAnchor] = useState<Record<string, HTMLElement | null>>({});

    // Filter features based on scenario (if scenarios are specified, only show for those scenarios)
    const visibleFeatures = PLUGIN_FEATURES.filter(f => !f.scenarios || f.scenarios.includes(scenario as any));

    const loadData = async () => {
        try {
            setLoading(true);
            // Load effort level first (will be displayed first)
            const effortResult = await api.getScenarioStringFlag(scenario, 'thinking_effort');
            if (effortResult?.success && effortResult?.data?.value !== undefined) {
                setEffort(effortResult.data.value);
            }

            // Load thinking mode (for claude_code scenario)
            if (scenario === 'claude_code') {
                const thinkingModeResult = await api.getScenarioStringFlag(scenario, 'thinking_mode');
                if (thinkingModeResult?.success && thinkingModeResult?.data?.value !== undefined) {
                    setThinkingMode(thinkingModeResult.data.value);
                }
            }

            // Load plugin features (only visible ones)
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
                    console.error('Failed to update feature:', result.error);
                    loadData();
                }
            })
            .catch((error) => {
                console.error('Failed to update feature:', error);
                loadData();
            })
            .finally(() => {
                setUpdating(prev => ({ ...prev, [featureKey]: false }));
            });
    };

    const handleMenuOpen = (featureKey: string, event: React.MouseEvent<HTMLElement>) => {
        setMenuAnchor(prev => ({ ...prev, [featureKey]: event.currentTarget }));
    };

    const handleMenuClose = (featureKey: string) => {
        setMenuAnchor(prev => ({ ...prev, [featureKey]: null }));
    };

    const setEffortLevel = (level: string) => {
        if (updating.effort || level === effort) return; // Prevent rapid clicks or no-ops

        setUpdating(prev => ({ ...prev, effort: true }));

        api.setScenarioStringFlag(scenario, 'thinking_effort', level)
            .then((result) => {
                if (result.success) {
                    setEffort(level);
                } else {
                    console.error('Failed to update effort level:', result.error);
                    loadData();
                }
            })
            .catch((error) => {
                console.error('Failed to update effort level:', error);
                loadData();
            })
            .finally(() => {
                setUpdating(prev => ({ ...prev, effort: false }));
            });
    };

    const updateThinkingMode = (mode: string) => {
        if (updating.thinkingMode || mode === thinkingMode) return;

        setUpdating(prev => ({ ...prev, thinkingMode: true }));

        api.setScenarioStringFlag(scenario, 'thinking_mode', mode)
            .then((result) => {
                if (result.success) {
                    setThinkingMode(mode);
                } else {
                    console.error('Failed to update thinking mode:', result.error);
                    loadData();
                }
            })
            .catch((error) => {
                console.error('Failed to update thinking mode:', error);
                loadData();
            })
            .finally(() => {
                setUpdating(prev => ({ ...prev, thinkingMode: false }));
            });
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
        <Box sx={{ display: 'flex', flexDirection: 'column', py: 2, gap: 2 }}>
            {/* Thinking Row */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 3 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Tooltip title="Thinking configuration" arrow>
                        <Psychology sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    </Tooltip>
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Thinking
                    </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flex: 1 }}>
                    {/* Thinking Effort */}
                    <Tooltip title={`Effort: ${EFFORT_LEVELS.find(l => l.value === effort)?.label || 'Default'}`} placement="right" arrow>
                        <Button
                            size="small"
                            variant="outlined"
                            onClick={(e) => !updating.effort && handleMenuOpen('effort', e)}
                            disabled={updating.effort}
                            endIcon={<KeyboardArrowDown />}
                            sx={{
                                minWidth: 110,
                                textTransform: 'none',
                                bgcolor: effort && effort !== '' ? 'primary.main' : 'transparent',
                                color: effort && effort !== '' ? 'primary.contrastText' : 'text.primary',
                                border: effort && effort !== '' ? 'none' : '1px solid',
                                borderColor: 'divider',
                                opacity: updating.effort ? 0.6 : 1,
                                '&:hover': {
                                    bgcolor: effort && effort !== '' ? 'primary.dark' : 'action.selected',
                                },
                            }}
                        >
                            Effort: {EFFORT_LEVELS.find(l => l.value === effort)?.label || 'Default'}
                        </Button>
                    </Tooltip>
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
                                onClick={() => {
                                    setEffortLevel(level.value);
                                    handleMenuClose('effort');
                                }}
                            >
                                <Tooltip title={level.description} placement="right" arrow>
                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                                        <ListItemText>{level.label}</ListItemText>
                                        {level.value === effort && <CheckIcon />}
                                    </Box>
                                </Tooltip>
                            </MenuItem>
                        ))}
                    </Menu>

                    {/* Thinking Mode (claude_code only) */}
                    {scenario === 'claude_code' && (
                        <>
                            <Tooltip title={`Mode: ${THINKING_MODES.find(m => m.value === thinkingMode)?.label || 'Default'}`} placement="right" arrow>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    onClick={(e) => !updating.thinkingMode && handleMenuOpen('thinkingMode', e)}
                                    disabled={updating.thinkingMode}
                                    endIcon={<KeyboardArrowDown />}
                                    sx={{
                                        minWidth: 110,
                                        textTransform: 'none',
                                        bgcolor: thinkingMode && thinkingMode !== 'default' ? 'primary.main' : 'transparent',
                                        color: thinkingMode && thinkingMode !== 'default' ? 'primary.contrastText' : 'text.primary',
                                        border: thinkingMode && thinkingMode !== 'default' ? 'none' : '1px solid',
                                        borderColor: 'divider',
                                        opacity: updating.thinkingMode ? 0.6 : 1,
                                        '&:hover': {
                                            bgcolor: thinkingMode && thinkingMode !== 'default' ? 'primary.dark' : 'action.selected',
                                        },
                                    }}
                                >
                                    Mode: {THINKING_MODES.find(m => m.value === thinkingMode)?.label || 'Default'}
                                </Button>
                            </Tooltip>
                            <Menu
                                anchorEl={menuAnchor['thinkingMode']}
                                open={Boolean(menuAnchor['thinkingMode'])}
                                onClose={() => handleMenuClose('thinkingMode')}
                                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                            >
                                {THINKING_MODES.map((mode) => {
                                    const Icon = mode.icon;
                                    return (
                                        <MenuItem
                                            key={mode.value}
                                            selected={mode.value === thinkingMode}
                                            onClick={() => {
                                                updateThinkingMode(mode.value);
                                                handleMenuClose('thinkingMode');
                                            }}
                                        >
                                            <Tooltip title={mode.description} placement="right" arrow>
                                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                                                    <ListItemIcon sx={{ mr: -1 }}>
                                                        <Icon sx={{ fontSize: '1rem' }} />
                                                    </ListItemIcon>
                                                    <ListItemText>{mode.label}</ListItemText>
                                                    {mode.value === thinkingMode && <CheckIcon />}
                                                </Box>
                                            </Tooltip>
                                        </MenuItem>
                                    );
                                })}
                            </Menu>
                        </>
                    )}
                </Box>
            </Box>

            {/* Plugin Features Row */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 3 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Tooltip title="Plugin Features Control" arrow>
                        <Science sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    </Tooltip>
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Plugin
                    </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flex: 1 }}>
                    {visibleFeatures.map((feature) => {
                        const isEnabled = features[feature.key] || false;
                        const isUpdating = updating[feature.key] || false;
                        const anchorEl = menuAnchor[feature.key];
                        return (
                            <Box key={feature.key}>
                                <Tooltip title={`${feature.label}: ${isEnabled ? 'On' : 'Off'}`} placement="right" arrow>
                                    <Button
                                        size="small"
                                        variant="outlined"
                                        onClick={(e) => !isUpdating && handleMenuOpen(feature.key, e)}
                                        disabled={isUpdating}
                                        endIcon={<KeyboardArrowDown />}
                                        sx={{
                                            minWidth: 100,
                                            textTransform: 'none',
                                            bgcolor: isEnabled ? 'primary.main' : 'transparent',
                                            color: isEnabled ? 'primary.contrastText' : 'text.primary',
                                            fontWeight: isEnabled ? 600 : 400,
                                            border: isEnabled ? 'none' : '1px solid',
                                            borderColor: 'divider',
                                            opacity: isUpdating ? 0.6 : 1,
                                            '&:hover': {
                                                bgcolor: isEnabled ? 'primary.dark' : 'action.selected',
                                            },
                                        }}
                                    >
                                        {feature.label}: {isEnabled ? 'On' : 'Off'}
                                    </Button>
                                </Tooltip>
                                <Menu
                                    anchorEl={anchorEl}
                                    open={Boolean(anchorEl)}
                                    onClose={() => handleMenuClose(feature.key)}
                                    anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                                    transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                                >
                                    <MenuItem
                                        selected={isEnabled}
                                        onClick={() => {
                                            setFeature(feature.key, true);
                                            handleMenuClose(feature.key);
                                        }}
                                    >
                                        <ListItemText>On</ListItemText>
                                        {isEnabled && <CheckIcon />}
                                    </MenuItem>
                                    <MenuItem
                                        selected={!isEnabled}
                                        onClick={() => {
                                            setFeature(feature.key, false);
                                            handleMenuClose(feature.key);
                                        }}
                                    >
                                        <ListItemText>Off</ListItemText>
                                        {!isEnabled && <CheckIcon />}
                                    </MenuItem>
                                </Menu>
                            </Box>
                        );
                    })}
                </Box>
            </Box>
        </Box>
    );
};

export default PluginFeatures;
