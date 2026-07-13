import {useFeatureFlags} from '@/contexts/FeatureFlagsContext';
import { Psychology as IconBrain, Shield as IconShield, SettingsApplications } from '@/components/icons';
import {Alert, Box, Chip, Typography,} from '@mui/material';
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

    // One row per feature: name + always-visible description, with a plain
    // On/Off chip on the right. The description used to live only in a hover
    // tooltip (and the chip repeated the feature name).
    const featureRow = (
        icon: React.ReactNode,
        name: string,
        description: string,
        enabled: boolean,
        onToggle: () => void,
    ) => (
        <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                {icon}
                <Typography variant="subtitle2">{name}</Typography>
            </Box>
            <Typography variant="body2" sx={{ color: 'text.secondary', flex: 1 }}>
                {description}
            </Typography>
            <Chip
                label={enabled ? t('common.on') : t('common.off')}
                onClick={onToggle}
                size="small"
                sx={{ ...chipStyle(enabled), minWidth: 52 }}
            />
        </Box>
    );

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', gap: 0}}>
            {/* Skill Features - Only in full edition */}
            {isFullEdition && SKILL_FEATURES.map((feature) =>
                <React.Fragment key={feature.key}>
                    {featureRow(
                        <IconBrain sx={{ fontSize: 16, color: 'text.secondary' }} />,
                        t(feature.labelKey),
                        t(feature.descriptionKey),
                        features[feature.key] || false,
                        () => toggleFeature(feature.key),
                    )}
                </React.Fragment>
            )}

            {/* Guardrails Section */}
            {featureRow(
                <IconShield sx={{ fontSize: 16, color: 'text.secondary' }} />,
                t('system.experimentalFeatures.guardrails'),
                t('system.experimentalFeatures.enableGuardrails'),
                guardrailsEnabled,
                toggleGuardrails,
            )}

            {guardrailsEnabled && (
                <Alert severity="info" sx={{ mt: 1 }}>
                    <Typography variant="body2">
                        {t('system.experimentalFeatures.guardrailsEnabledInfo')}
                    </Typography>
                </Alert>
            )}

            {/* MCP Section */}
            {featureRow(
                <SettingsApplications sx={{ fontSize: '1rem', color: 'text.secondary' }} />,
                `${t('system.experimentalFeatures.mcp')} Tools`,
                t('system.experimentalFeatures.enableMCP'),
                mcpEnabled,
                toggleMCP,
            )}

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
