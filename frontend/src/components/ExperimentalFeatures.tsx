import {
    Box,
    Button,
    ButtonGroup,
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
        <Box sx={{ display: 'flex', justifyContent: 'flex-start', alignItems: 'center', py: 2, gap: 2 }}>
            {/* Label */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Science sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                    Experimental
                </Typography>
            </Box>

            {/* Feature toggles */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                {FEATURES.map((feature) => (
                    <Tooltip key={feature.key} title={feature.description} arrow>
                        <ButtonGroup
                            size="small"
                            sx={ToggleButtonGroupStyle}
                        >
                        <Button
                            variant={features[feature.key] ? 'outlined' : 'contained'}
                            onClick={() => setFeature(feature.key, false)}
                            sx={{
                                ...(features[feature.key] === false && ToggleButtonStyle),
                            }}
                        >
                            Off
                        </Button>
                        <Button
                            variant={features[feature.key] ? 'contained' : 'outlined'}
                            onClick={() => setFeature(feature.key, true)}
                            sx={{
                                ...(features[feature.key] === true && ToggleButtonStyle),
                            }}
                        >
                            {feature.label}
                        </Button>
                    </ButtonGroup>
                    </Tooltip>
                ))}
            </Box>
        </Box>
    );
};

export default ExperimentalFeatures;
