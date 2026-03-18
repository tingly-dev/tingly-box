import {
    Box,
    Tooltip,
    Typography,
    Chip,
    FormControl,
    Select,
    MenuItem,
    SelectChangeEvent,
} from '@mui/material';
import { Science, FiberManualRecord } from '@mui/icons-material';
import React, { useEffect, useState } from 'react';
import { api } from '../services/api';

export interface ExperimentalFeaturesProps {
    scenario: string;
}

interface FeatureConfig {
    key: string;
    label: string;
    description: string;
    scenarios?: string[];
}

const FEATURES: FeatureConfig[] = [
    { key: 'smart_compact', label: 'Smart Compact', description: 'Remove thinking blocks from conversation history to reduce context' },
    { key: 'recording', label: 'Recording', description: 'Record scenario-level request/response traffic for debugging (Legacy)' },
    { key: 'clean_header', label: 'Clean Header', description: 'Remove Claude Code billing header from system messages (Claude Code only)', scenarios: ['claude_code'] },
];

// Record V2 modes
const RECORD_V2_MODES = [
    { value: '', label: 'Off', description: 'Recording disabled' },
    { value: 'request', label: 'Request', description: 'Record request only' },
    { value: 'response', label: 'Response', description: 'Record response only' },
    { value: 'request_response', label: 'Both', description: 'Record both request and response' },
] as const;

const ExperimentalFeatures: React.FC<ExperimentalFeaturesProps> = ({ scenario }) => {
    const [features, setFeatures] = useState<Record<string, boolean>>({});
    const [recordV2Mode, setRecordV2Mode] = useState<string>('');
    const [loading, setLoading] = useState(true);

    // Filter features based on scenario (if scenarios are specified, only show for those scenarios)
    const visibleFeatures = FEATURES.filter(f => !f.scenarios || f.scenarios.includes(scenario as any));

    const loadFeatures = async () => {
        try {
            setLoading(true);
            const results = await Promise.all(
                visibleFeatures.map(f => api.getScenarioFlag(scenario, f.key))
            );
            const newFeatures: Record<string, boolean> = {};
            visibleFeatures.forEach((f, i) => {
                newFeatures[f.key] = results[i]?.data?.value || false;
            });
            setFeatures(newFeatures);

            // Load Record V2 mode (string flag)
            const recordV2Result = await api.getScenarioStringFlag(scenario, 'record_v2');
            setRecordV2Mode(recordV2Result?.data?.value || '');
        } catch (error) {
            console.error('Failed to load experimental features:', error);
        } finally {
            setLoading(false);
        }
    };

    const toggleFeature = (featureKey: string) => {
        const newValue = !features[featureKey];
        console.log('toggleFeature called:', featureKey, newValue);
        api.setScenarioFlag(scenario, featureKey, newValue)
            .then((result) => {
                console.log('setScenarioFlag result:', result);
                if (result.success) {
                    setFeatures(prev => ({ ...prev, [featureKey]: newValue }));
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

    const handleRecordV2Change = (event: SelectChangeEvent<string>) => {
        const newMode = event.target.value;
        console.log('Record V2 mode changed:', newMode);
        api.setScenarioStringFlag(scenario, 'record_v2', newMode)
            .then((result) => {
                if (result.success) {
                    setRecordV2Mode(newMode);
                } else {
                    console.error('Failed to set record_v2 mode:', result);
                    loadFeatures();
                }
            })
            .catch((err) => {
                console.error('Failed to set record_v2 mode:', err);
                loadFeatures();
            });
    };

    useEffect(() => {
        loadFeatures();
    }, [scenario]);

    if (loading) {
        return null;
    }

    const currentRecordMode = RECORD_V2_MODES.find(m => m.value === recordV2Mode);
    const isRecordV2Enabled = recordV2Mode !== '';

    return (
        <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
            {/* Label */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                    Experimental
                </Typography>
                <Tooltip title="Experimental Features Control" arrow>
                    <Science sx={{ fontSize: '1rem', color: 'text.secondary' }} />
                </Tooltip>
            </Box>

            {/* Feature toggles as clickable chips */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1, flexWrap: 'wrap' }}>
                {visibleFeatures.map((feature) => {
                    const isEnabled = features[feature.key] || false;
                    return (
                        <Tooltip key={feature.key} title={feature.description + (isEnabled ? ' (enabled)' : ' (disabled) - Click to enable')} arrow>
                            <Chip
                                label={`${feature.label} · ${isEnabled ? 'On' : 'Off'}`}
                                onClick={() => toggleFeature(feature.key)}
                                size="small"
                                sx={{
                                    bgcolor: isEnabled ? 'primary.main' : 'action.hover',
                                    color: isEnabled ? 'primary.contrastText' : 'text.primary',
                                    fontWeight: isEnabled ? 600 : 400,
                                    border: isEnabled ? 'none' : '1px solid',
                                    borderColor: 'divider',
                                    '&:hover': {
                                        bgcolor: isEnabled ? 'primary.dark' : 'action.selected',
                                    },
                                }}
                            />
                        </Tooltip>
                    );
                })}

                {/* Record V2 Dropdown */}
                <Tooltip
                    title={`Recording V2: ${currentRecordMode?.description || 'Disabled'}${isRecordV2Enabled ? ' (enabled)' : ' (disabled)'}`}
                    arrow
                >
                    <Box sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                        bgcolor: isRecordV2Enabled ? 'error.main' : 'action.hover',
                        color: isRecordV2Enabled ? 'error.contrastText' : 'text.primary',
                        borderRadius: 1,
                        px: 1,
                        py: 0.5,
                        border: isRecordV2Enabled ? 'none' : '1px solid',
                        borderColor: 'divider',
                        '&:hover': {
                            bgcolor: isRecordV2Enabled ? 'error.dark' : 'action.selected',
                        },
                    }}>
                        <FiberManualRecord sx={{ fontSize: '0.875rem' }} />
                        <Typography variant="caption" sx={{ fontWeight: isRecordV2Enabled ? 600 : 400, mr: 1 }}>
                            Record V2
                        </Typography>
                        <FormControl size="small" sx={{ minWidth: 100 }}>
                            <Select
                                value={recordV2Mode}
                                onChange={handleRecordV2Change}
                                variant="standard"
                                disableUnderline
                                sx={{
                                    color: 'inherit',
                                    fontSize: '0.75rem',
                                    '& .MuiSelect-select': {
                                        py: 0,
                                        pr: '20px !important',
                                    },
                                    '& .MuiSvgIcon-root': {
                                        color: 'inherit',
                                    },
                                }}
                            >
                                {RECORD_V2_MODES.map((mode) => (
                                    <MenuItem key={mode.value} value={mode.value} sx={{ fontSize: '0.875rem' }}>
                                        {mode.label}
                                    </MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                    </Box>
                </Tooltip>
            </Box>
        </Box>
    );
};

export default ExperimentalFeatures;
