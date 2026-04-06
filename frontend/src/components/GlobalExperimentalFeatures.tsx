import {useFeatureFlags} from '@/contexts/FeatureFlagsContext';
import { Cloud } from '@mui/icons-material';
import { IconBrain, IconShield } from '@tabler/icons-react';
import {Alert, Box, Chip, Tooltip, Typography,} from '@mui/material';
import React, {useEffect, useState} from 'react';
import {api} from '../services/api';
import {isFullEdition} from "@/utils/edition.ts";

const SKILL_FEATURES = [
    {
        key: 'skill_ide',
        label: 'IDE Skills',
        description: 'Enable IDE Skills feature for managing code snippets and skills from IDEs'
    },
] as const;

const GlobalExperimentalFeatures: React.FC = () => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [guardrailsEnabled, setGuardrailsEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const {refresh} = useFeatureFlags();

    const loadFeatures = async () => {
        try {
            setLoading(true);
            // Load skill features
            const results = await Promise.all(
                SKILL_FEATURES.map(f => api.getScenarioFlag('_global', f.key))
            );
            const newFeatures: Record<string, boolean> = {};
            SKILL_FEATURES.forEach((f, i) => {
                newFeatures[f.key] = results[i]?.data?.value || false;
            });
            setFeatures(newFeatures);

            // Load Guardrails flag
            const guardrailsResult = await api.getScenarioFlag('_global', 'guardrails');
            setGuardrailsEnabled(guardrailsResult?.data?.value || false);

        } catch (error) {
            console.error('Failed to load global experimental features:', error);
        } finally {
            setLoading(false);
        }
    };

    const toggleFeature = (featureKey: string) => {
        const newValue = !features[featureKey];
        console.log('toggleGlobalFeature called:', featureKey, newValue);
        api.setScenarioFlag('_global', featureKey, newValue)
            .then((result) => {
                console.log('setScenarioFlag result:', result);
                if (result.success) {
                    setFeatures(prev => ({...prev, [featureKey]: newValue}));
                    refresh()
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

    const toggleGuardrails = () => {
        const newValue = !guardrailsEnabled;
        api.setScenarioFlag('_global', 'guardrails', newValue)
            .then((result) => {
                if (result.success) {
                    setGuardrailsEnabled(newValue);
                    refresh();
                } else {
                    console.error('Failed to set Guardrails:', result);
                    loadFeatures();
                }
            })
            .catch((err) => {
                console.error('Failed to set Guardrails:', err);
                loadFeatures();
            });
    };

    useEffect(() => {
        loadFeatures();
    }, []);

    if (loading) {
        return null;
    }

    const chipStyle = (isEnabled: boolean) => ({
        bgcolor: isEnabled ? 'primary.main' : 'action.hover',
        color: isEnabled ? 'primary.contrastText' : 'text.primary',
        fontWeight: isEnabled ? 600 : 400,
        border: isEnabled ? 'none' : '1px solid',
        borderColor: 'divider',
        '&:hover': {
            bgcolor: isEnabled ? 'primary.dark' : 'action.selected',
        },
    });

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', gap: 0}}>
            {/* Skill Features - Only in full edition */}
            {isFullEdition && (
                <Box sx={{display: 'flex', alignItems: 'center', py: 2, gap: 3}}>
                    {/* Label */}
                    <Box sx={{display: 'flex', alignItems: 'center', gap: 1, minWidth: 180}}>
                        <IconBrain size={16} style={{ color: 'var(--mui-palette-text-secondary)' }}/>
                        <Typography variant="subtitle2" sx={{color: 'text.secondary'}}>
                            Skills
                        </Typography>
                        <Tooltip title="Skill Features - Enable prompt and skill management features" arrow>
                            <Box/>
                        </Tooltip>
                    </Box>

                    {/* Skill feature toggles as clickable chips */}
                    <Box sx={{display: 'flex', alignItems: 'center', gap: 2, flex: 1}}>
                        {SKILL_FEATURES.map((feature) => {
                            const isEnabled = features[feature.key] || false;
                            return (
                                <Tooltip key={feature.key}
                                         title={feature.description + (isEnabled ? ' (enabled)' : ' (disabled) - Click to enable')}
                                         arrow>
                                    <Chip
                                        label={`${feature.label} · ${isEnabled ? 'On' : 'Off'}`}
                                        onClick={() => toggleFeature(feature.key)}
                                        size="small"
                                        sx={chipStyle(isEnabled)}
                                    />
                                </Tooltip>
                            );
                        })}
                    </Box>
                </Box>)
            }

            {/* Guardrails Section */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <IconShield size={16} style={{ color: 'var(--mui-palette-text-secondary)' }} />
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Guardrails
                    </Typography>
                </Box>

                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                    <Tooltip title={"Enable Guardrails - block risky tool calls and filter sensitive outputs" + (guardrailsEnabled ? ' (enabled)' : ' (disabled) - Click to enable')} arrow>
                        <Chip
                            label={`Guardrails · ${guardrailsEnabled ? 'On' : 'Off'}`}
                            onClick={toggleGuardrails}
                            size="small"
                            sx={chipStyle(guardrailsEnabled)}
                        />
                    </Tooltip>
                </Box>
            </Box>

            {guardrailsEnabled && (
                <Alert severity="info" sx={{ mt: 1 }}>
                    <Typography variant="body2">
                        Guardrails is enabled. A "Guardrails" page is available in the sidebar for rule management.
                    </Typography>
                </Alert>
            )}
        </Box>
    );
};

export default GlobalExperimentalFeatures;
