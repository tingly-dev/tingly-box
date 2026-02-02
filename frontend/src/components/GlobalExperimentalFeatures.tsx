import {
    Box,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { Public, Psychology } from '@mui/icons-material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';
import {ToggleButtonGroupStyle, ToggleButtonStyle} from "@/styles/style.tsx";

const FEATURES = [
    { key: 'smart_compact', label: 'Smart Compact', description: 'Remove thinking blocks from conversation history to reduce context (applies to all scenarios)', icon: 'science' as const },
    { key: 'recording', label: 'Recording', description: 'Record scenario-level request/response traffic for debugging (applies to all scenarios)', icon: 'science' as const },
] as const;

const SKILL_FEATURES = [
    { key: 'skill_user', label: 'User Prompt', description: 'Enable User Prompt feature for managing user recordings and templates', icon: 'skill' as const },
    { key: 'skill_ide', label: 'IDE Skills', description: 'Enable IDE Skills feature for managing code snippets and skills from IDEs', icon: 'skill' as const },
] as const;

const GlobalExperimentalFeatures: React.FC = () => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [loading, setLoading] = useState(true);

    const loadFeatures = async () => {
        try {
            setLoading(true);
            const allFeatures = [...FEATURES, ...SKILL_FEATURES];
            const results = await Promise.all(
                allFeatures.map(f => api.getScenarioFlag('_global', f.key))
            );
            const newFeatures: Record<string, boolean> = {};
            allFeatures.forEach((f, i) => {
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
            {/* Experimental Features */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                {/* Label */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                        Experimental
                    </Typography>
                    <Tooltip title="Global Experimental Features - Apply to All Scenarios" arrow>
                        <Public sx={{ fontSize: '1rem', color: 'text.secondary' }} />
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

            {/* Skill Features */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                {/* Label */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                        Skills
                    </Typography>
                    <Tooltip title="Skill Features - Enable prompt and skill management features" arrow>
                        <Psychology sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    </Tooltip>
                </Box>

                {/* Skill feature toggles */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                    {SKILL_FEATURES.map((feature) => (
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
        </Box>
    );
};

export default GlobalExperimentalFeatures;
