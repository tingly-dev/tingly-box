import { WarningAmber, Close } from '@mui/icons-material';
import {
    Alert,
    Autocomplete,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    MenuItem,
    Stack,
    Switch,
    TextField,
    Typography,
    Checkbox,
    Grid,
} from '@mui/material';
import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { getProvidersByStyle } from '../services/serviceProviders';
import api from '../services/api';
import { OpenAI } from '@lobehub/icons';
import { Anthropic } from '@lobehub/icons';

export interface EnhancedProviderFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic' | undefined;
    token: string;
    noKeyRequired?: boolean;
    enabled?: boolean;
}

interface PresetProviderFormDialogProps {
    open: boolean;
    onClose: () => void;
    onSubmit: (e: React.FormEvent) => void;
    data: EnhancedProviderFormData;
    onChange: (field: keyof EnhancedProviderFormData, value: any) => void;
    mode: 'add' | 'edit';
    title?: string;
    submitText?: string;
    isFirstProvider?: boolean;
}

const ProviderFormDialog = ({
    open,
    onClose,
    onSubmit,
    data,
    onChange,
    mode,
    title,
    submitText,
    isFirstProvider = false,
}: PresetProviderFormDialogProps) => {
    const { t } = useTranslation();
    const defaultTitle = mode === 'add' ? t('providerDialog.addTitle') : t('providerDialog.editTitle');
    const defaultSubmitText = mode === 'add' ? t('providerDialog.addButton') : t('common.saveChanges');

    const [verifying, setVerifying] = useState(false);
    const [noApiKey, setNoApiKey] = useState(data.noKeyRequired || false);
    const [isCustomUrl, setIsCustomUrl] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{
        success: boolean;
        message: string;
        details?: string;
        responseTime?: number;
        modelsCount?: number;
    } | null>(null);
    const [styleChangedWarning, setStyleChangedWarning] = useState(false);

    // Sync noApiKey state with data.noKeyRequired prop
    useEffect(() => {
        setNoApiKey(data.noKeyRequired || false);
    }, [data.noKeyRequired]);

    // Determine if current URL is custom (not from presets)
    useEffect(() => {
        if (data.apiBase && data.apiStyle) {
            const providers = getCurrentProviders();
            const isPreset = providers.some(p => p.baseUrl === data.apiBase);
            setIsCustomUrl(!isPreset);
        } else {
            setIsCustomUrl(false);
        }
    }, [data.apiBase, data.apiStyle]);

    // Reset noApiKey when switching between preset/custom
    useEffect(() => {
        if (!isCustomUrl && noApiKey) {
            setNoApiKey(false);
            onChange('noKeyRequired', false);
        }
    }, [isCustomUrl]);

    const openaiProviders = getProvidersByStyle('openai');
    const anthropicProviders = getProvidersByStyle('anthropic');

    // Get current provider options based on apiStyle
    const getCurrentProviders = () => {
        if (data.apiStyle === 'openai') return openaiProviders;
        if (data.apiStyle === 'anthropic') return anthropicProviders;
        return [];
    };

    // Handle provider/baseurl selection
    const handleProviderOrBaseUrlSelect = (newValue: string | { title: string; value: string; baseUrl: string; api_style: string } | null) => {
        setVerificationResult(null);

        if (typeof newValue === 'string') {
            // Custom input - only update apiBase
            onChange('apiBase', newValue);
            setIsCustomUrl(true);
        } else if (newValue && newValue.baseUrl) {
            // Preset selected - update apiBase
            onChange('apiBase', newValue.baseUrl);
            setIsCustomUrl(false);
            // If name is empty and token is empty, set default name
            if (!data.name && !data.token) {
                onChange('name', t('providerDialog.keyName.autoFill', { title: newValue.title }));
            }
        } else if (newValue === null) {
            // Clear selection - only clear apiBase, preserve name
            onChange('apiBase', '');
            setIsCustomUrl(true);
        }
    };

    // Handle verification
    const handleVerify = async () => {
        // Skip verification if no API key mode is enabled
        if (noApiKey) {
            setVerificationResult(null);
            return true;
        }

        if (!data.name || !data.apiBase || !data.token || !data.apiStyle) {
            setVerificationResult({
                success: false,
                message: t('providerDialog.verification.missingFields'),
            });
            return false;
        }

        setVerifying(true);
        setVerificationResult(null);

        try {
            const result = await api.probeProvider(
                data.apiStyle,
                data.apiBase,
                data.token,
            )

            if (result.success && result.data) {
                setVerificationResult({
                    success: true,
                    message: result.data.message,
                    details: t('providerDialog.verification.testResult', { result: result.data.test_result }),
                    responseTime: result.data.response_time_ms,
                    modelsCount: result.data.models_count,
                });
                return true;
            } else {
                setVerificationResult({
                    success: false,
                    message: result.error?.message || t('providerDialog.verification.failed'),
                    details: result.error?.type,
                });
                return false;
            }
        } catch (error) {
            setVerificationResult({
                success: false,
                message: t('providerDialog.verification.networkError'),
                details: error instanceof Error ? error.message : 'Unknown error',
            });
            return false;
        } finally {
            setVerifying(false);
        }
    };

    // Wrapped submit handler that includes verification
    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        // Skip verification for edit mode with empty token, or if no API key is checked
        const shouldVerify = mode === 'add' ? !noApiKey : (data.token !== '' && !noApiKey);

        if (shouldVerify) {
            const verified = await handleVerify();
            if (!verified) {
                // Verification failed, don't submit
                return;
            }
        }

        // Call the original onSubmit
        onSubmit(e);
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    {title || defaultTitle}
                    <IconButton
                        aria-label="close"
                        onClick={onClose}
                        sx={{ ml: 2 }}
                        size="small"
                    >
                        <Close />
                    </IconButton>
                </Box>
            </DialogTitle>
            <form onSubmit={handleSubmit}>
                <DialogContent sx={{ pb: 1 }}>
                    <Stack spacing={2.5}>
                        {/* First Provider Welcome Message */}
                        {isFirstProvider && mode === 'add' && (
                            <Alert severity="info" sx={{ mb: 1 }}>
                                <Typography variant="body2">
                                    <strong>Getting Started</strong><br />
                                    Add your first API key to enable AI services. You can add more keys later.
                                </Typography>
                            </Alert>
                        )}
                        {/* API Style Selection - Form Field Style */}
                        <TextField
                            select
                            fullWidth
                            size="small"
                            label={t('providerDialog.apiStyle.label')}
                            value={data.apiStyle || ''}
                            onChange={(e) => {
                                const newStyle = e.target.value as 'openai' | 'anthropic' | '';
                                const oldStyle = data.apiStyle;

                                onChange('apiStyle', newStyle);
                                setVerificationResult(null);

                                // Reset apiBase and name when switching styles
                                if (oldStyle && newStyle && oldStyle !== newStyle) {
                                    onChange('apiBase', '');
                                    onChange('name', '');
                                    setIsCustomUrl(false);
                                    // Show warning only in edit mode
                                    if (mode === 'edit') {
                                        setStyleChangedWarning(true);
                                        setTimeout(() => setStyleChangedWarning(false), 4000);
                                    }
                                }
                            }}
                            sx={{
                                '& .MuiOutlinedInput-notchedOutline': {
                                    borderColor: 'divider',
                                },
                                '&:hover .MuiOutlinedInput-notchedOutline': {
                                    borderColor: 'primary.main',
                                },
                                '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
                                    borderColor: 'primary.main',
                                    borderWidth: 2,
                                },
                            }}
                            helperText={
                                data.apiStyle === 'openai'
                                    ? t('providerDialog.apiStyle.helperOpenAI')
                                    : data.apiStyle === 'anthropic'
                                        ? t('providerDialog.apiStyle.helperAnthropic')
                                        : t('providerDialog.apiStyle.placeholder')
                            }
                            required={mode === 'add'}
                        >
                            <MenuItem value="">
                                <em>{t('providerDialog.apiStyle.placeholder')}</em>
                            </MenuItem>
                            <MenuItem value="openai">
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                    <OpenAI size={16} />
                                    {t('providerDialog.apiStyle.openAI')}
                                </Box>
                            </MenuItem>
                            <MenuItem value="anthropic">
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                    <Anthropic size={16} />
                                    {t('providerDialog.apiStyle.anthropic')}
                                </Box>
                            </MenuItem>
                        </TextField>

                        {/* Style change warning alert */}
                        {styleChangedWarning && (
                            <Alert
                                severity="warning"
                                icon={<WarningAmber fontSize="inherit" />}
                                sx={{ mt: 1 }}
                                action={
                                    <IconButton
                                        aria-label="close"
                                        color="inherit"
                                        size="small"
                                        onClick={() => setStyleChangedWarning(false)}
                                    >
                                        ×
                                    </IconButton>
                                }
                            >
                                <Typography variant="body2">
                                    {t('providerDialog.apiStyle.switchWarning', { defaultValue: 'API style changed. Base URL has been reset. Please select a compatible provider.' })}
                                </Typography>
                            </Alert>
                        )}

                        {/* Show other fields only after API Style is selected */}
                        {data.apiStyle && (
                            <>
                                {/* API Key Name */}
                                <TextField
                                    size="small"
                                    fullWidth
                                    label={t('providerDialog.keyName.label')}
                                    value={data.name}
                                    onChange={(e) => {
                                        onChange('name', e.target.value);
                                        setVerificationResult(null);
                                    }}
                                    required
                                    placeholder={t('providerDialog.keyName.placeholder')}
                                />

                                {/* Merged Provider Preset and Base URL Input */}
                                <Autocomplete
                                    freeSolo
                                    autoSelect
                                    size="small"
                                    options={getCurrentProviders()}
                                    getOptionLabel={(option) => {
                                        if (typeof option === 'string') return option;
                                        return `${option.title} - ${option.baseUrl}`;
                                    }}
                                    value={data.apiBase}
                                    onChange={(_event, newValue) => {
                                        handleProviderOrBaseUrlSelect(newValue);
                                    }}
                                    onInputChange={(_event, newInputValue) => {
                                        // Allow custom input
                                        onChange('apiBase', newInputValue);
                                        setIsCustomUrl(true);
                                        setVerificationResult(null);
                                    }}
                                    renderOption={(props, option) => (
                                        <Box component="li" {...props} sx={{ fontSize: '0.875rem' }}>
                                            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                                <Typography variant="body2" fontWeight="medium">
                                                    {option.title}
                                                </Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    {option.baseUrl}
                                                </Typography>
                                            </Box>
                                        </Box>
                                    )}
                                    renderInput={(params) => (
                                        <TextField
                                            {...params}
                                            label={t('providerDialog.providerOrUrl.label')}
                                            placeholder={t('providerDialog.providerOrUrl.placeholder')}
                                        />
                                    )}
                                    isOptionEqualToValue={(option, value) => {
                                        if (typeof value === 'string') {
                                            return option.baseUrl === value;
                                        }
                                        return option.value === value.value;
                                    }}
                                />

                                {/* API Key Field with No Key Required switch */}
                                <TextField
                                    size="small"
                                    fullWidth
                                    label={t('providerDialog.apiKey.label')}
                                    type="password"
                                    value={data.token}
                                    onChange={(e) => {
                                        onChange('token', e.target.value);
                                        // Clear verification result when token changes
                                        setVerificationResult(null);
                                    }}
                                    required={mode === 'add' && !noApiKey}
                                    placeholder={mode === 'add' ? t('providerDialog.apiKey.placeholderAdd') : t('providerDialog.apiKey.placeholderEdit')}
                                    helperText={mode === 'edit' && t('providerDialog.apiKey.helperEdit')}
                                    disabled={noApiKey}
                                />
                                {isCustomUrl && (
                                    <FormControlLabel
                                        control={
                                            <Switch
                                                size="small"
                                                checked={noApiKey}
                                                onChange={(e) => {
                                                    setNoApiKey(e.target.checked);
                                                    onChange('noKeyRequired', e.target.checked);
                                                    setVerificationResult(null);
                                                    if (e.target.checked) {
                                                        onChange('token', '');
                                                    }
                                                }}
                                            />
                                        }
                                        label="No Key Required"
                                    />
                                )}

                                {/* Verification Result */}
                                {verificationResult && (
                                    <Alert
                                        severity={verificationResult.success ? 'success' : 'error'}
                                        sx={{ mt: 1 }}
                                        action={
                                            <IconButton
                                                aria-label="close"
                                                color="inherit"
                                                size="small"
                                                onClick={() => setVerificationResult(null)}
                                            >
                                                ×
                                            </IconButton>
                                        }
                                    >
                                        <Box>
                                            <Typography variant="body2" fontWeight="bold">
                                                {verificationResult.message}
                                            </Typography>
                                            {verificationResult.details && (
                                                <Typography variant="caption" display="block">
                                                    {verificationResult.details}
                                                </Typography>
                                            )}
                                            {verificationResult.responseTime && (
                                                <Typography variant="caption" display="block">
                                                    {t('providerDialog.verification.responseTime', { time: verificationResult.responseTime })}
                                                    {verificationResult.modelsCount && ` • ${t('providerDialog.verification.modelsAvailable', { count: verificationResult.modelsCount })}`}
                                                </Typography>
                                            )}
                                        </Box>
                                    </Alert>
                                )}

                                {/* Enabled Toggle (Edit mode only) */}
                                {mode === 'edit' && (
                                    <FormControlLabel
                                        control={
                                            <Switch
                                                size="small"
                                                checked={data.enabled || false}
                                                onChange={(e) => onChange('enabled', e.target.checked)}
                                            />
                                        }
                                        label={t('providerDialog.enabled')}
                                    />
                                )}
                            </>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button type="submit" variant="contained" size="small" disabled={verifying}>
                        {verifying ? (
                            <>
                                <CircularProgress size={16} sx={{ mr: 1 }} />
                                {mode === 'add' ? 'Adding...' : 'Saving...'}
                            </>
                        ) : (
                            submitText || defaultSubmitText
                        )}
                    </Button>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default ProviderFormDialog;