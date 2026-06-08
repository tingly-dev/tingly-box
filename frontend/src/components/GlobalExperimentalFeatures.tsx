import {useFeatureFlags} from '@/contexts/FeatureFlagsContext';
import { Psychology as IconBrain, Shield as IconShield, SettingsApplications } from '@/components/icons';
import {Alert, Box, Chip, Tooltip, Typography,} from '@mui/material';
import React, {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {api} from '../services/api';
import {isFullEdition} from "@/utils/edition.ts";

const SKILL_FEATURES = [
    {
        key: 'skill_ide',
        labelKey: 'system.experimentalFeatures.skills',
        descriptionKey: 'system.experimentalFeatures.enableIdeSkills',
    },
] as const;

const GlobalExperimentalFeatures: React.FC = () => {
    const {t} = useTranslation();
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [guardrailsEnabled, setGuardrailsEnabled] = useState(false);
    const [mcpEnabled, setMCPEnabled] = useState(false);
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

            // Load MCP flag
            const mcpResult = await api.getScenarioFlag('_global', 'mcp');
            setMCPEnabled(mcpResult?.data?.value || false);

        } catch (error) {
            console.error('Failed to load global experimental features:', error);
        } finally {
            setLoading(false);
        }
    };

    const toggleFeature = (featureKey: string) => {
        const newValue = !features[featureKey];
        api.setScenarioFlag('_global', featureKey, newValue)
            .then((result) => {
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

    const toggleMCP = () => {
        const newValue = !mcpEnabled;
        api.setScenarioFlag('_global', 'mcp', newValue)
            .then((result) => {
                if (result.success) {
                    setMCPEnabled(newValue);
                    refresh();
                } else {
                    console.error('Failed to set MCP:', result);
                    loadFeatures();
                }
            })
            .catch((err) => {
                console.error('Failed to set MCP:', err);
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
                        <IconBrain sx={{ fontSize: 16, color: 'text.secondary' }} />
                        <Typography variant="subtitle2" sx={{color: 'text.secondary'}}>
                            {t('system.experimentalFeatures.skills')}
                        </Typography>
                        <Tooltip title={t('system.experimentalFeatures.enableIdeSkills')} arrow>
                            <Box/>
                        </Tooltip>
                    </Box>

                    {/* Skill feature toggles as clickable chips */}
                    <Box sx={{display: 'flex', alignItems: 'center', gap: 2, flex: 1}}>
                        {SKILL_FEATURES.map((feature) => {
                            const isEnabled = features[feature.key] || false;
                            return (
                                <Tooltip key={feature.key}
                                         title={t(feature.descriptionKey) + (isEnabled ? ` (${t('system.experimentalFeatures.enabled')})` : ` (${t('system.experimentalFeatures.disabled')})`)}
                                         arrow>
                                    <Chip
                                        label={`${t(feature.labelKey)} · ${isEnabled ? t('common.on') : t('common.off')}`}
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
                    <IconShield sx={{ fontSize: 16, color: 'text.secondary' }} />
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        {t('system.experimentalFeatures.guardrails')}
                    </Typography>
                </Box>

                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                    <Tooltip title={t('system.experimentalFeatures.enableGuardrails') + (guardrailsEnabled ? ` (${t('system.experimentalFeatures.enabled')})` : ` (${t('system.experimentalFeatures.disabled')})`)} arrow>
                        <Chip
                            label={`${t('system.experimentalFeatures.guardrails')} · ${guardrailsEnabled ? t('common.on') : t('common.off')}`}
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
                        {t('system.experimentalFeatures.guardrailsEnabledInfo')}
                    </Typography>
                </Alert>
            )}

            {/* MCP Section */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <SettingsApplications sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        {t('system.experimentalFeatures.mcp')}
                    </Typography>
                </Box>

                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                    <Tooltip title={t('system.experimentalFeatures.enableMCP') + (mcpEnabled ? ` (${t('system.experimentalFeatures.enabled')})` : ` (${t('system.experimentalFeatures.disabled')})`)} arrow>
                        <Chip
                            label={`${t('system.experimentalFeatures.mcp')} Tools · ${mcpEnabled ? t('common.on') : t('common.off')}`}
                            onClick={toggleMCP}
                            size="small"
                            sx={{ ...chipStyle(mcpEnabled), cursor: 'pointer', pointerEvents: 'auto' }}
                        />
                    </Tooltip>
                </Box>
            </Box>

            {mcpEnabled && (
                <Alert severity="info" sx={{ mt: 1 }}>
                    <Typography variant="body2">
                        {t('system.experimentalFeatures.mcpEnabledInfo')}
                    </Typography>
                </Alert>
            )}

        </Box>
    );
};

export default GlobalExperimentalFeatures;
