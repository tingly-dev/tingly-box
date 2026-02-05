import {
    Alert,
    Box,
    FormControlLabel,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import { Psychology, Cloud } from '@mui/icons-material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';
import { switchControlLabelStyle } from '@/styles/toggleStyles';

const SKILL_FEATURES = [
    { key: 'skill_ide', label: 'IDE Skills', description: 'Enable IDE Skills feature for managing code snippets and skills from IDEs' },
] as const;

const GlobalExperimentalFeatures: React.FC = () => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [remoteCCEnabled, setRemoteCCEnabled] = useState(false);
    const [loading, setLoading] = useState(true);

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

            // Load Remote CC flag
            const remoteCCResult = await api.getScenarioFlag('_global', 'skill_remote_cc');
            setRemoteCCEnabled(remoteCCResult?.data?.value || false);
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

    const setRemoteCC = (enabled: boolean) => {
        api.setScenarioFlag('_global', 'skill_remote_cc', enabled)
            .then((result) => {
                if (result.success) {
                    setRemoteCCEnabled(enabled);
                } else {
                    console.error('Failed to set Remote CC:', result);
                    loadFeatures();
                }
            })
            .catch((err) => {
                console.error('Failed to set Remote CC:', err);
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

            {/* Remote Claude Code Section */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                {/* Title with Icon */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Cloud sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Remote Claude Code
                    </Typography>
                </Box>

                {/* Remote CC Switch - on the same line */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                    <Tooltip title="Enable Remote Claude Code - access Claude Code sessions remotely through the web UI" arrow>
                        <FormControlLabel
                            control={
                                <Switch
                                    size="small"
                                    checked={remoteCCEnabled}
                                    onChange={(e) => setRemoteCC(e.target.checked)}
                                    color="primary"
                                />
                            }
                            label="Remote CC"
                            sx={switchControlLabelStyle}
                        />
                    </Tooltip>
                </Box>
            </Box>

            {/* Tip message at the bottom when Remote CC is enabled */}
            {remoteCCEnabled && (
                <Alert severity="info" sx={{ mt: 1 }}>
                    <Typography variant="body2">
                        Remote CC is now enabled! A "Remote CC" menu item has appeared in the sidebar. Click it to access the remote Claude Code interface.
                    </Typography>
                </Alert>
            )}
        </Box>
    );
};

export default GlobalExperimentalFeatures;
