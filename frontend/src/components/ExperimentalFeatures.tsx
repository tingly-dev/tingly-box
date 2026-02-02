import {
    Box,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { Science } from '@mui/icons-material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';
import {ToggleButtonGroupStyle, ToggleButtonStyle} from "@/styles/style.tsx";

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

    const setFeature = (featureKey: string, enabled: boolean) => {
        console.log('setFeature called:', featureKey, enabled);
        api.setScenarioFlag(scenario, featureKey, enabled)
            .then((result) => {
                console.log('setScenarioFlag result:', result);
                if (result.success) {
                    setFeatures(prev => ({ ...prev, [featureKey]: enabled }));
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
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                    Experimental
                </Typography>
                <Tooltip title="Experimental Features Control" arrow>
                    <Science sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                </Tooltip>
            </Box>

            {/* Feature toggles */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                {FEATURES.map((feature) => (
                    <Tooltip key={feature.key} title={feature.description} arrow>
                        <ToggleButtonGroup
                            size="small"
                            sx={ToggleButtonGroupStyle}
                            exclusive
                            value={features[feature.key] ? 'on' : 'off'}
                            onChange={() => setFeature(feature.key, !features[feature.key])}
                        >
                            <ToggleButton value="off" sx={ToggleButtonStyle}>
                                Off
                            </ToggleButton>
                            <ToggleButton value="on" sx={ToggleButtonStyle}>
                                {feature.label}
                            </ToggleButton>
                        </ToggleButtonGroup>
                    </Tooltip>
                ))}
            </Box>
        </Box>
    );
};

export default ExperimentalFeatures;
