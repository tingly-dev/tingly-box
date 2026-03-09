import {
    Box,
    CircularProgress,
    Tooltip,
    Typography,
    ToggleButton,
    ToggleButtonGroup,
    Chip,
} from '@mui/material';
import { Science } from '@mui/icons-material';
import Psychology from '@mui/icons-material/Psychology';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';
import { toggleButtonGroupStyle, toggleButtonStyle } from '../styles/toggleStyles';

export interface PluginFeaturesProps {
    scenario: string;
}

const PLUGIN_FEATURES = [
    { key: 'smart_compact', label: 'Smart Compact', description: 'Remove thinking blocks from conversation history to reduce context' },
    { key: 'recording', label: 'Recording', description: 'Record scenario-level request/response traffic for debugging' },
    { key: 'clean_header', label: 'Clean Header', description: 'Remove Claude Code billing header from system messages', scenarios: ['claude_code'] },
] as const;

const EFFORT_LEVELS = [
    { value: '', label: 'Default', description: 'Use model default' },
    { value: 'low', label: 'Low', description: '~1K tokens - Fast' },
    { value: 'medium', label: 'Med', description: '~5K tokens - Balanced' },
    { value: 'high', label: 'High', description: '~20K tokens - Deep' },
    { value: 'max', label: 'Max', description: '~32K tokens - Max quality' },
] as const;

const PluginFeatures: React.FC<PluginFeaturesProps> = ({ scenario }) => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [effort, setEffort] = useState<string>('');
    const [loading, setLoading] = useState(true);
    const [updating, setUpdating] = useState<Record<string, boolean>>({});

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

    const toggleFeature = (featureKey: string) => {
        if (updating[featureKey]) return; // Prevent rapid clicks

        const newValue = !features[featureKey];
        setUpdating(prev => ({ ...prev, [featureKey]: true }));

        api.setScenarioFlag(scenario, featureKey, newValue)
            .then((result) => {
                if (result.success) {
                    setFeatures(prev => ({ ...prev, [featureKey]: newValue }));
                } else {
                    console.error('Failed to update feature:', result.error);
                    loadData(); // Reload to show actual state
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

    useEffect(() => {
        loadData();
    }, [scenario, visibleFeatures]);

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
            {/* Thinking Effort Row */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 3 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Tooltip title="Thinking effort level for extended reasoning" arrow>
                        <Psychology sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    </Tooltip>
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Thinking Effort
                    </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', flex: 1 }}>
                    <ToggleButtonGroup
                        value={effort}
                        exclusive
                        size="small"
                        disabled={updating.effort}
                        onChange={(_, value) => value !== null && setEffortLevel(value)}
                        sx={toggleButtonGroupStyle}
                    >
                        {EFFORT_LEVELS.map((level) => (
                            <Tooltip key={level.value} title={level.description} arrow>
                                <ToggleButton value={level.value} sx={toggleButtonStyle}>
                                    {level.label}
                                </ToggleButton>
                            </Tooltip>
                        ))}
                    </ToggleButtonGroup>
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
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    {visibleFeatures.map((feature) => {
                        const isEnabled = features[feature.key] || false;
                        const isUpdating = updating[feature.key] || false;
                        return (
                            <Tooltip key={feature.key} title={feature.description} arrow>
                                <Chip
                                    label={`${feature.label} · ${isEnabled ? 'On' : 'Off'}`}
                                    onClick={() => !isUpdating && toggleFeature(feature.key)}
                                    disabled={isUpdating}
                                    sx={{
                                        height: 32,
                                        bgcolor: isEnabled ? 'primary.main' : 'action.hover',
                                        color: isEnabled ? 'primary.contrastText' : 'text.primary',
                                        fontWeight: isEnabled ? 600 : 400,
                                        border: isEnabled ? 'none' : '1px solid',
                                        borderColor: 'divider',
                                        opacity: isUpdating ? 0.6 : 1,
                                        '&:hover': {
                                            bgcolor: isEnabled ? 'primary.dark' : 'action.selected',
                                        },
                                    }}
                                />
                            </Tooltip>
                        );
                    })}
                </Box>
            </Box>
        </Box>
    );
};

export default PluginFeatures;
