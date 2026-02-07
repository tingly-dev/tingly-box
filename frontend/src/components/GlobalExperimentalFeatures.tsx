import {
    Alert,
    Box,
    Tooltip,
    Typography,
    Chip,
} from '@mui/material';
import { Psychology, Cloud } from '@mui/icons-material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';

const SKILL_FEATURES = [
    { key: 'skill_ide', label: 'IDE Skills', description: 'Enable IDE Skills feature for managing code snippets and skills from IDEs' },
] as const;

const GlobalExperimentalFeatures: React.FC = () => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [remoteCoderEnabled, setRemoteCoderEnabled] = useState(false);
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

            // Load Remote Coder flag
            const remoteCoderResult = await api.getScenarioFlag('_global', 'enable_remote_coder');
            setRemoteCoderEnabled(remoteCoderResult?.data?.value || false);
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
                    setFeatures(prev => ({ ...prev, [featureKey]: newValue }));
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

    const toggleRemoteCoder = () => {
        const newValue = !remoteCoderEnabled;
        api.setScenarioFlag('_global', 'enable_remote_coder', newValue)
            .then((result) => {
                if (result.success) {
                    setRemoteCoderEnabled(newValue);
                } else {
                    console.error('Failed to set Remote Coder:', result);
                    loadFeatures();
                }
            })
            .catch((err) => {
                console.error('Failed to set Remote Coder:', err);
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
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
            {/* Skill Features */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                {/* Label */}
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Psychology sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Skills
                    </Typography>
                    <Tooltip title="Skill Features - Enable prompt and skill management features" arrow>
                        <Box />
                    </Tooltip>
                </Box>

                {/* Skill feature toggles as clickable chips */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                    {SKILL_FEATURES.map((feature) => {
                        const isEnabled = features[feature.key] || false;
                        return (
                                                    <Tooltip key={feature.key} title={feature.description + (isEnabled ? ' (enabled)' : ' (disabled) - Click to enable')} arrow>
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
            </Box>

            {/* Remote Coder Section */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                {/* Title with Icon */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Cloud sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                    <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                        Remote Coder
                    </Typography>
                </Box>

                {/* Remote Coder Toggle as clickable chip */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1 }}>
                                        <Tooltip title={"Enable Remote Coder - access sessions remotely through the web UI" + (remoteCoderEnabled ? ' (enabled)' : ' (disabled) - Click to enable')} arrow>
                        <Chip
                            label={`Remote Coder · ${remoteCoderEnabled ? 'On' : 'Off'}`}
                            onClick={toggleRemoteCoder}
                            size="small"
                            sx={chipStyle(remoteCoderEnabled)}
                        />
                    </Tooltip>
                </Box>
            </Box>

            {/* Tip message at the bottom when Remote Coder is enabled */}
            {remoteCoderEnabled && (
                <Alert severity="info" sx={{ mt: 1 }}>
                    <Typography variant="body2">
                        Remote Coder is now enabled! A "Remote Coder" menu item has appeared in the sidebar. Click it to access the remote coder interface.
                    </Typography>
                </Alert>
            )}
        </Box>
    );
};

export default GlobalExperimentalFeatures;
