import {useFeatureFlags} from '@/contexts/FeatureFlagsContext';
import { Security } from '@mui/icons-material';
import {Alert, Box, Chip, Tooltip, Typography,} from '@mui/material';
import React, {useEffect, useState} from 'react';
import {api} from '../services/api';

const GlobalExperimentalFeatures: React.FC = () => {
    const [guardrailsEnabled, setGuardrailsEnabled] = useState(false);
    const [loading, setLoading] = useState(true);
    const {refresh} = useFeatureFlags();

    const loadFeatures = async () => {
        try {
            setLoading(true);
            const guardrailsResult = await api.getScenarioFlag('_global', 'guardrails');
            setGuardrailsEnabled(guardrailsResult?.data?.value || false);

        } catch (error) {
            console.error('Failed to load global experimental features:', error);
        } finally {
            setLoading(false);
        }
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
            {/* Guardrails Section */}
            <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                    <Security sx={{ fontSize: '1rem', color: 'text.secondary' }} />
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
