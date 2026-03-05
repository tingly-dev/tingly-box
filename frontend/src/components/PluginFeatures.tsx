import {
    Box,
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

    const loadData = async () => {
        try {
            setLoading(true);
            // Load effort level first (will be displayed first)
            const effortResult = await api.getScenarioStringFlag(scenario, 'thinking_effort');
            setEffort(effortResult?.data?.value || '');

            // Load plugin features
            const featureResults = await Promise.all(
                PLUGIN_FEATURES.map(f => api.getScenarioFlag(scenario, f.key))
            );
            const newFeatures: Record<string, boolean> = {};
            PLUGIN_FEATURES.forEach((f, i) => {
                newFeatures[f.key] = featureResults[i]?.data?.value || false;
            });
            setFeatures(newFeatures);
        } catch (error) {
            console.error('Failed to load scenario features:', error);
        } finally {
            setLoading(false);
        }
    };

    const toggleFeature = (featureKey: string) => {
        const newValue = !features[featureKey];
        api.setScenarioFlag(scenario, featureKey, newValue)
            .then((result) => {
                if (result.success) {
                    setFeatures(prev => ({ ...prev, [featureKey]: newValue }));
                } else {
                    loadData();
                }
            })
            .catch(() => loadData());
    };

    const setEffortLevel = (level: string) => {
        api.setScenarioStringFlag(scenario, 'thinking_effort', level)
            .then((result) => {
                if (result.success) {
                    setEffort(level);
                } else {
                    loadData();
                }
            })
            .catch(() => loadData());
    };

    useEffect(() => {
        loadData();
    }, [scenario]);

    if (loading) {
        return null;
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
                    {PLUGIN_FEATURES.map((feature) => {
                        const isEnabled = features[feature.key] || false;
                        return (
                            <Tooltip key={feature.key} title={feature.description} arrow>
                                <Chip
                                    label={`${feature.label} · ${isEnabled ? 'On' : 'Off'}`}
                                    onClick={() => toggleFeature(feature.key)}
                                    sx={{
                                        height: 32,
                                        bgcolor: isEnabled ? 'primary.main' : 'action.hover',
                                        color: isEnabled ? 'primary.contrastText' : 'text.primary',
                                        fontWeight: isEnabled ? 600 : 400,
                                        border: isEnabled ? 'none' : '1px solid',
                                        borderColor: 'divider',
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
