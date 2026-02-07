import {
    Box,
    Tooltip,
    Typography,
    Chip,
} from '@mui/material';
import { Science } from '@mui/icons-material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';

export interface ExperimentalFeaturesProps {
    scenario: string;
}

const FEATURES = [
    { key: 'smart_compact', label: 'Smart Compact', description: 'Remove thinking blocks from conversation history to reduce context' },
    { key: 'recording', label: 'Recording', description: 'Record scenario-level request/response traffic for debugging' },
] as const;

const ExperimentalFeatures: React.FC<ExperimentalFeaturesProps> = ({ scenario }) => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [loading, setLoading] = useState(true);

    const loadFeatures = async () => {
        try {
            setLoading(true);
            const results = await Promise.all(
                FEATURES.map(f => api.getScenarioFlag(scenario, f.key))
            );
            const newFeatures: Record<string, boolean> = {};
            FEATURES.forEach((f, i) => {
                newFeatures[f.key] = results[i]?.data?.value || false;
            });
            setFeatures(newFeatures);
        } catch (error) {
            console.error('Failed to load experimental features:', error);
        } finally {
            setLoading(false);
        }
    };

    const toggleFeature = (featureKey: string) => {
        const newValue = !features[featureKey];
        console.log('toggleFeature called:', featureKey, newValue);
        api.setScenarioFlag(scenario, featureKey, newValue)
            .then((result) => {
                console.log('setScenarioFlag result:', result);
                if (result.success) {
                    setFeatures(prev => ({ ...prev, [featureKey]: newValue }));
                } else {
                    console.error('Failed to set feature:', result);
                    loadFeatures();
                }
            })
            .catch((err) => {
                console.error('Failed to set feature:', err);
                loadFeatures();
            });
    };

    useEffect(() => {
        loadFeatures();
    }, [scenario]);

    if (loading) {
        return null;
    }

    return (
        <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
            {/* Label */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                    Experimental
                </Typography>
                <Tooltip title="Experimental Features Control" arrow>
                    <Science sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                </Tooltip>
            </Box>

            {/* Feature toggles as clickable chips */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                {FEATURES.map((feature) => {
                    const isEnabled = features[feature.key] || false;
                    return (
                                                <Tooltip key={feature.key} title={feature.description + (isEnabled ? ' (enabled)' : ' (disabled) - Click to enable')} arrow>
                            <Chip
                                label={`${feature.label} · ${isEnabled ? 'On' : 'Off'}`}
                                onClick={() => toggleFeature(feature.key)}
                                size="small"
                                sx={{
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
    );
};

export default ExperimentalFeatures;
