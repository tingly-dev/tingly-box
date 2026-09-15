import {useFeatureFlags} from '@/contexts/FeatureFlagsContext';
import type {ExperimentalFeature} from '@/components/ExperimentalFeatureGate';
import { Psychology as IconBrain, Shield as IconShield, SettingsApplications, Terminal as IconTerminal } from '@/components/icons';
import {Alert, Box, Chip, Typography,} from '@mui/material';
import {alpha} from '@mui/material/styles';
import React, {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate} from 'react-router-dom';
import {api} from '../services/api';
import {isFullEdition} from "@/utils/edition.ts";

const SKILL_FEATURES = [
    {
        key: 'skill_user',
        labelKey: 'system.experimentalFeatures.userPrompts',
        descriptionKey: 'system.experimentalFeatures.enableUserPrompts',
    },
    {
        key: 'skill_ide',
        labelKey: 'system.experimentalFeatures.skills',
        descriptionKey: 'system.experimentalFeatures.enableIdeSkills',
    },
] as const;

interface GlobalExperimentalFeaturesProps {
    requestedFeature?: ExperimentalFeature;
    returnTo?: string;
}

const GlobalExperimentalFeatures: React.FC<GlobalExperimentalFeaturesProps> = ({ requestedFeature, returnTo }) => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [guardrailsEnabled, setGuardrailsEnabled] = useState(false);
    const [mcpEnabled, setMCPEnabled] = useState(false);
    const [managedAgentEnabled, setManagedAgentEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const [updatingFeature, setUpdatingFeature] = useState<ExperimentalFeature>();
    const [actionError, setActionError] = useState(false);
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

            // Load Managed Agent (Tasks) flag
            const managedAgentResult = await api.getScenarioFlag('_global', 'managed_agent');
            setManagedAgentEnabled(managedAgentResult?.data?.value || false);

        } catch (error) {
            console.error('Failed to load global experimental features:', error);
        } finally {
            setLoading(false);
        }
    };

    const finishUpdate = async (
        featureKey: ExperimentalFeature,
        newValue: boolean,
        updateLocalState: () => void,
    ) => {
        setActionError(false);
        setUpdatingFeature(featureKey);
        try {
            const result = await api.setScenarioFlag('_global', featureKey, newValue);
            if (!result.success) throw new Error('Feature update was rejected');

            updateLocalState();
            await refresh();
            if (newValue && requestedFeature === featureKey && returnTo) {
                navigate(returnTo, {replace: true});
            }
        } catch (error) {
            console.error(`Failed to set ${featureKey}:`, error);
            setActionError(true);
            loadFeatures();
        } finally {
            setUpdatingFeature(undefined);
        }
    };

    const toggleFeature = (featureKey: 'skill_user' | 'skill_ide') => {
        const newValue = !features[featureKey];
        return finishUpdate(
            featureKey,
            newValue,
            () => setFeatures(prev => ({...prev, [featureKey]: newValue})),
        );
    };

    const toggleGuardrails = () => {
        const newValue = !guardrailsEnabled;
        return finishUpdate('guardrails', newValue, () => setGuardrailsEnabled(newValue));
    };

    const toggleMCP = () => {
        const newValue = !mcpEnabled;
        return finishUpdate('mcp', newValue, () => setMCPEnabled(newValue));
    };

    const toggleManagedAgent = () => {
        const newValue = !managedAgentEnabled;
        return finishUpdate('managed_agent', newValue, () => setManagedAgentEnabled(newValue));
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
        featureKey: ExperimentalFeature,
        icon: React.ReactNode,
        name: string,
        description: string,
        enabled: boolean,
        onToggle: () => void,
    ) => (
        <Box
            id={`experimental-feature-${featureKey}`}
            sx={{
                display: 'flex',
                alignItems: 'center',
                py: 2,
                px: 1.5,
                mx: -1.5,
                gap: 3,
                borderRadius: 1,
                borderLeft: '3px solid',
                borderLeftColor: requestedFeature === featureKey ? 'primary.main' : 'transparent',
                bgcolor: requestedFeature === featureKey
                    ? theme => alpha(theme.palette.primary.main, 0.06)
                    : 'transparent',
            }}
        >
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
                disabled={updatingFeature !== undefined}
                size="small"
                sx={{ ...chipStyle(enabled), minWidth: 52 }}
            />
        </Box>
    );

    const requestedFeatureName = requestedFeature
        ? {
            skill_user: t('system.experimentalFeatures.userPrompts'),
            skill_ide: t('system.experimentalFeatures.skills'),
            guardrails: t('system.experimentalFeatures.guardrails'),
            mcp: `${t('system.experimentalFeatures.mcp')} Tools`,
            managed_agent: t('system.experimentalFeatures.managedAgent'),
        }[requestedFeature]
        : undefined;

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', gap: 0}}>
            {requestedFeature && (
                <Alert
                    severity="info"
                    sx={{
                        mb: 1.5,
                        bgcolor: theme => alpha(theme.palette.primary.main, 0.06),
                        color: 'text.primary',
                        '& .MuiAlert-icon': {color: 'primary.main'},
                    }}
                >
                    {t('system.experimentalFeatures.requiredMessage', {feature: requestedFeatureName})}
                </Alert>
            )}

            {actionError && (
                <Alert severity="error" sx={{mb: 1.5}}>
                    {t('system.experimentalFeatures.enableFailed')}
                </Alert>
            )}

            {/* Skill Features - Only in full edition */}
            {isFullEdition && SKILL_FEATURES.map((feature) =>
                <React.Fragment key={feature.key}>
                    {featureRow(
                        feature.key,
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
                'guardrails',
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
                'mcp',
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

            {/* Managed Agent (Tasks) Section — full edition only: it runs the
                Claude Code CLI on the host through agentboot. */}
            {isFullEdition && featureRow(
                'managed_agent',
                <IconTerminal sx={{ fontSize: 16, color: 'text.secondary' }} />,
                t('system.experimentalFeatures.managedAgent'),
                t('system.experimentalFeatures.enableManagedAgent'),
                managedAgentEnabled,
                toggleManagedAgent,
            )}

            {isFullEdition && managedAgentEnabled && (
                <Alert severity="info" sx={{ mt: 1 }}>
                    <Typography variant="body2">
                        {t('system.experimentalFeatures.managedAgentEnabledInfo')}
                    </Typography>
                </Alert>
            )}

        </Box>
    );
};

export default GlobalExperimentalFeatures;
