import {
    Box,
    FormControlLabel,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import { Psychology } from '@mui/icons-material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';
import { switchControlLabelStyle } from '@/styles/toggleStyles';

const SKILL_FEATURES = [
    { key: 'skill_ide', label: 'IDE Skills', description: 'Enable IDE Skills feature for managing code snippets and skills from IDEs' },
] as const;

const GlobalExperimentalFeatures: React.FC = () => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [loading, setLoading] = useState(true);

    const loadFeatures = async () => {
        try {
            setLoading(true);
            const results = await Promise.all(
                SKILL_FEATURES.map(f => api.getScenarioFlag('_global', f.key))
            );
            const newFeatures: Record<string, boolean> = {};
            SKILL_FEATURES.forEach((f, i) => {
                newFeatures[f.key] = results[i]?.data?.value || false;
            });
            setFeatures(newFeatures);
        } catch (error) {
            console.error('Failed to load global experimental features:', error);
        } finally {
            setLoading(false);
        }
    };

    const setFeature = (featureKey: string, enabled: boolean) => {
        console.log('setGlobalFeature called:', featureKey, enabled);
        api.setScenarioFlag('_global', featureKey, enabled)
            .then((result) => {
                console.log('setScenarioFlag result:', result);
                if (result.success) {
                    setFeatures(prev => ({ ...prev, [featureKey]: enabled }));
                } else {
                    console.error('Failed to set global feature:', result);
                    loadFeatures();
                }
            })
            .catch((err) => {
                console.error('Failed to set global feature:', err);
                loadFeatures();
            });
    };

    useEffect(() => {
        loadFeatures();
    }, []);

    if (loading) {
        return null;
    }

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            {/* Skill Features */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                {/* Label */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Skills
                    </Typography>
                    <Tooltip title="Skill Features - Enable prompt and skill management features" arrow>
                        <Psychology sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    </Tooltip>
                </Box>

                {/* Skill feature toggles using Switch */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                    {SKILL_FEATURES.map((feature) => (
                        <Tooltip key={feature.key} title={feature.description} arrow>
                            <FormControlLabel
                                control={
                                    <Switch
                                        size="small"
                                        checked={features[feature.key] || false}
                                        onChange={(e) => setFeature(feature.key, e.target.checked)}
                                        color="primary"
                                    />
                                }
                                label={feature.label}
                                sx={switchControlLabelStyle}
                            />
                        </Tooltip>
                    ))}
                </Box>
            </Box>
        </Box>
    );
};

export default GlobalExperimentalFeatures;
