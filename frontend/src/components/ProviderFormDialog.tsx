import { WarningAmber, Close, Visibility, VisibilityOff } from '@mui/icons-material';
import {
    Alert,
    Autocomplete,
    Box,
    Button,
    Checkbox,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    FormControlLabel,
    FormLabel,
    IconButton,
    InputAdornment,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';
import React, { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { getAllUniqueProviders, type UniqueProvider } from '../services/serviceProviders';
import { api } from '../services/api';
import { OpenAI, Anthropic } from './BrandIcons';

export interface EnhancedProviderFormData {
    uuid?: string;
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic' | undefined;
    token: string;
    noKeyRequired?: boolean;
    enabled?: boolean;
    proxyUrl?: string;
    // New fields for multi-protocol support
    protocols?: ('openai' | 'anthropic')[];
    providerBaseUrls?: { openai?: string; anthropic?: string };
}

interface PresetProviderFormDialogProps {
    open: boolean;
    onClose: () => void;
    onSubmit: (e: React.FormEvent) => void;
    onForceAdd?: () => void;
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
    onForceAdd,
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
    const [showApiKey, setShowApiKey] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{
        success: boolean;
        message: string;
        details?: string;
        responseTime?: number;
        modelsCount?: number;
    } | null>(null);

    // Selected provider object (null for custom URL)
    const [selectedProvider, setSelectedProvider] = useState<UniqueProvider | null>(null);

    // Protocol checkboxes state
    const [protocolOpenAI, setProtocolOpenAI] = useState(false);
    const [protocolAnthropic, setProtocolAnthropic] = useState(false);

    // All unique providers
    const allProviders = useMemo(() => getAllUniqueProviders(), []);

    // Helper component for displaying base URL
    const ProtocolBaseUrlDisplay: React.FC<{ url: string }> = ({ url }) => {
        if (!url) return null;
        return (
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 0.5,
                    mt: 0.5,
                    px: 1,
                    py: 0.5,
                    bgcolor: 'background.default',
                    borderRadius: 0.75,
                }}
            >
                <Typography
                    variant="caption"
                    sx={{
                        fontFamily: 'monospace',
                        color: 'primary.main',
                        fontSize: '0.7rem',
                        wordBreak: 'break-all',
                    }}
                >
                    {url}
                </Typography>
            </Box>
        );
    };

    // Sync noApiKey state with data.noKeyRequired prop
    useEffect(() => {
        setNoApiKey(data.noKeyRequired || false);
    }, [data.noKeyRequired]);

    // Initialize state when dialog opens or mode changes
    useEffect(() => {
        if (open) {
            setVerificationResult(null);

            if (mode === 'edit') {
                // In edit mode, set protocol based on existing apiStyle
                setProtocolOpenAI(data.apiStyle === 'openai');
                setProtocolAnthropic(data.apiStyle === 'anthropic');

                // Try to find matching provider
                const matchingProvider = allProviders.find(p =>
                    (p.baseUrlOpenAI === data.apiBase && data.apiStyle === 'openai') ||
                    (p.baseUrlAnthropic === data.apiBase && data.apiStyle === 'anthropic')
                );
                setSelectedProvider(matchingProvider || null);
            } else {
                // In add mode, check if protocols were pre-set
                if (data.protocols && data.protocols.length > 0) {
                    setProtocolOpenAI(data.protocols.includes('openai'));
                    setProtocolAnthropic(data.protocols.includes('anthropic'));
                } else if (data.apiStyle) {
                    setProtocolOpenAI(data.apiStyle === 'openai');
                    setProtocolAnthropic(data.apiStyle === 'anthropic');
                } else {
                    setProtocolOpenAI(false);
                    setProtocolAnthropic(false);
                }
                setSelectedProvider(null);
            }
        }
    }, [open, mode]);

    // Sync protocol state back to parent data
    useEffect(() => {
        const protocols: ('openai' | 'anthropic')[] = [];
        if (protocolOpenAI) protocols.push('openai');
        if (protocolAnthropic) protocols.push('anthropic');
        onChange('protocols', protocols);

        // Set apiStyle to the first selected protocol (for backward compatibility)
        if (protocols.length > 0) {
            onChange('apiStyle', protocols[0]);
        } else {
            onChange('apiStyle', undefined);
        }

        // Update providerBaseUrls
        if (selectedProvider) {
            onChange('providerBaseUrls', {
                openai: selectedProvider.baseUrlOpenAI,
                anthropic: selectedProvider.baseUrlAnthropic,
            });
            // Set apiBase to match the first selected protocol
            if (protocolOpenAI && selectedProvider.baseUrlOpenAI) {
                onChange('apiBase', selectedProvider.baseUrlOpenAI);
            } else if (protocolAnthropic && selectedProvider.baseUrlAnthropic) {
                onChange('apiBase', selectedProvider.baseUrlAnthropic);
            }
        }
    }, [protocolOpenAI, protocolAnthropic]);

    // Handle provider selection from autocomplete
    const handleProviderSelect = (newValue: string | UniqueProvider | null) => {
        setVerificationResult(null);

        if (typeof newValue === 'string') {
            // Custom URL input
            setSelectedProvider(null);
            onChange('apiBase', newValue);
            onChange('providerBaseUrls', undefined);
        } else if (newValue) {
            // Preset provider selected
            setSelectedProvider(newValue);
            const displayName = newValue.alias || newValue.name;

            // Auto-select all supported protocols
            setProtocolOpenAI(newValue.supportsOpenAI);
            setProtocolAnthropic(newValue.supportsAnthropic);

            // Set base URL (prefer OpenAI if both supported)
            const baseUrl = newValue.baseUrlOpenAI || newValue.baseUrlAnthropic || '';
            onChange('apiBase', baseUrl);
            onChange('providerBaseUrls', {
                openai: newValue.baseUrlOpenAI,
                anthropic: newValue.baseUrlAnthropic,
            });

            // Auto-fill name when provider changes
            const autoName = t('providerDialog.keyName.autoFill', { title: displayName });
            onChange('name', autoName);
        } else {
            // Cleared
            setSelectedProvider(null);
            onChange('apiBase', '');
            onChange('providerBaseUrls', undefined);
            setProtocolOpenAI(false);
            setProtocolAnthropic(false);
        }
    };

    // Handle verification
    const handleVerify = async () => {
        if (noApiKey) {
            setVerificationResult(null);
            return true;
        }

        const apiStyle = protocolOpenAI ? 'openai' : protocolAnthropic ? 'anthropic' : undefined;
        const apiBase = protocolOpenAI && selectedProvider?.baseUrlOpenAI
            ? selectedProvider.baseUrlOpenAI
            : protocolAnthropic && selectedProvider?.baseUrlAnthropic
                ? selectedProvider.baseUrlAnthropic
                : data.apiBase;

        if (!data.name || !apiBase || !data.token || !apiStyle) {
            setVerificationResult({
                success: false,
                message: t('providerDialog.verification.missingFields'),
            });
            return false;
        }

        setVerifying(true);
        setVerificationResult(null);

        try {
            const result = await api.probeProvider(apiStyle, apiBase, data.token);

            if (result.success && result.data) {
                const isValid = result.data.valid !== false;
                setVerificationResult({
                    success: isValid,
                    message: result.data.message,
                    details: isValid ? t('providerDialog.verification.testResult', { result: result.data.test_result }) : undefined,
                    responseTime: result.data.response_time_ms,
                    modelsCount: result.data.models_count,
                });
                return isValid;
            } else {
                setVerificationResult({
                    success: false,
                    message: result.error?.message || t('providerDialog.verification.failed'),
                });
                return false;
            }
        } catch (error) {
            setVerificationResult({
                success: false,
                message: t('providerDialog.verification.networkError'),
            });
            return false;
        } finally {
            setVerifying(false);
        }
    };

    // Wrapped submit handler
    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const shouldVerify = mode === 'add' ? !noApiKey : (data.token !== '' && !noApiKey);

        if (shouldVerify) {
            const verified = await handleVerify();
            if (!verified) {
                return;
            }
        }

        // Close dialog immediately for better UX
        // The parent onSubmit will handle the actual API operation
        onClose();
        onSubmit(e);
    };

    const hasAnyProtocol = protocolOpenAI || protocolAnthropic;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth PaperProps={{ sx: { minHeight: 200 } }}>
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
                <DialogContent sx={{ pt: 1, pb: 1, minHeight: 280 }}>
                    {mode === 'add' && (
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                            {t('providerDialog.addDescription')}
                        </Typography>
                    )}
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

                        {/* ① Provider Selection */}
                        <Autocomplete
                            freeSolo
                            autoHighlight
                            openOnFocus
                            selectOnFocus
                            handleHomeEndKeys
                            size="small"
                            options={allProviders}
                            filterOptions={(options, state) => {
                                const inputValue = state.inputValue.toLowerCase();
                                // If input matches a selected provider's display, show all
                                const isSelectedFormat = selectedProvider &&
                                    (selectedProvider.alias || selectedProvider.name).toLowerCase() === inputValue;
                                if (isSelectedFormat) return options;

                                return options.filter(option => {
                                    const displayName = (option.alias || option.name).toLowerCase();
                                    return displayName.includes(inputValue) ||
                                        (option.baseUrlOpenAI || '').toLowerCase().includes(inputValue) ||
                                        (option.baseUrlAnthropic || '').toLowerCase().includes(inputValue);
                                });
                            }}
                            getOptionLabel={(option) => {
                                if (typeof option === 'string') return option;
                                return option.alias || option.name;
                            }}
                            value={selectedProvider}
                            onChange={(_event, newValue) => {
                                handleProviderSelect(newValue);
                            }}
                            inputValue={(() => {
                                if (selectedProvider) {
                                    return selectedProvider.alias || selectedProvider.name;
                                }
                                return data.apiBase;
                            })()}
                            onInputChange={(_event, newInputValue, reason) => {
                                if (reason === 'input') {
                                    // If typing, treat as custom URL when no provider matches
                                    const matchingProvider = allProviders.find(p =>
                                        (p.alias || p.name).toLowerCase() === newInputValue.toLowerCase()
                                    );
                                    if (!matchingProvider) {
                                        setSelectedProvider(null);
                                        onChange('apiBase', newInputValue);
                                        setVerificationResult(null);
                                        // Clear protocols when input is cleared
                                        if (!newInputValue) {
                                            setProtocolOpenAI(false);
                                            setProtocolAnthropic(false);
                                        }
                                    }
                                } else if (reason === 'clear') {
                                    // Handle clear button click
                                    setSelectedProvider(null);
                                    onChange('apiBase', '');
                                    onChange('providerBaseUrls', undefined);
                                    setProtocolOpenAI(false);
                                    setProtocolAnthropic(false);
                                    setVerificationResult(null);
                                }
                            }}
                            renderOption={(props, option) => (
                                <Box component="li" {...props} sx={{ fontSize: '0.875rem' }}>
                                    <Typography variant="body2" fontWeight="medium">
                                        {option.alias || option.name}
                                    </Typography>
                                </Box>
                            )}
                            renderInput={(params) => (
                                <TextField
                                    {...params}
                                    label={t('providerDialog.provider.label')}
                                    required
                                    placeholder={t('providerDialog.provider.placeholder')}
                                />
                            )}
                            isOptionEqualToValue={(option, value) => {
                                if (typeof value === 'string') return false;
                                return option.id === value?.id;
                            }}
                        />

                        {/* ② Protocol Selection (Checkboxes) */}
                        <FormControl
                            required
                            sx={{
                                position: 'relative',
                                border: 1,
                                borderColor: 'text.primary',
                                borderWidth: 0.5,
                                // opacity: 0.23,
                                borderRadius: 1,
                                p: 0.5,
                                m: 0,
                            }}
                        >
                            <FormLabel
                                sx={{
                                    position: 'absolute',
                                    top: -10,
                                    left: 12,
                                    px: 0.5,
                                    bgcolor: 'background.paper',
                                    fontSize: '0.75rem',
                                    color: 'text.secondary',
                                    '&.Mui-focused': {
                                        color: 'text.secondary',
                                    },
                                }}
                            >
                                {t('providerDialog.protocol.label')}
                            </FormLabel>
                            <Stack spacing={0.5} sx={{ mt: 0.5 }}>
                                {/* OpenAI Protocol */}
                                <Box
                                    sx={{
                                        px: 1.5,
                                        py: 1,
                                        borderRadius: 1,
                                        cursor: 'pointer',
                                        transition: 'all 0.15s',
                                        bgcolor: protocolOpenAI ? 'action.selected' : 'transparent',
                                        '&:hover': {
                                            bgcolor: protocolOpenAI ? 'action.selected' : 'action.hover',
                                        },
                                    }}
                                    onClick={() => {
                                        if (selectedProvider && !selectedProvider.supportsOpenAI) return;
                                        setProtocolOpenAI(!protocolOpenAI);
                                        setVerificationResult(null);
                                    }}
                                >
                                    <Stack direction="row" alignItems="flex-start" spacing={1}>
                                        <OpenAI size={18} sx={{ mt: 0.2 }} />
                                        <Box sx={{ flex: 1 }}>
                                            <Typography variant="body2" fontWeight={500}>
                                                {t('providerDialog.apiStyle.openAI')}
                                            </Typography>
                                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.2 }}>
                                                {t('providerDialog.apiStyle.helperOpenAI')}
                                            </Typography>
                                            {selectedProvider?.baseUrlOpenAI && (
                                                <ProtocolBaseUrlDisplay url={selectedProvider.baseUrlOpenAI} />
                                            )}
                                        </Box>
                                        <Checkbox
                                            size="small"
                                            checked={protocolOpenAI}
                                            disabled={selectedProvider ? !selectedProvider.supportsOpenAI : false}
                                            sx={{ p: 0, mt: -0.5 }}
                                        />
                                    </Stack>
                                </Box>
                                {/* Anthropic Protocol */}
                                <Box
                                    sx={{
                                        borderRadius: 1,
                                        px: 1.5,
                                        py: 1,
                                        cursor: 'pointer',
                                        transition: 'all 0.15s',
                                        bgcolor: protocolAnthropic ? 'action.selected' : 'transparent',
                                        '&:hover': {
                                            bgcolor: protocolAnthropic ? 'action.selected' : 'action.hover',
                                        },
                                    }}
                                    onClick={() => {
                                        if (selectedProvider && !selectedProvider.supportsAnthropic) return;
                                        setProtocolAnthropic(!protocolAnthropic);
                                        setVerificationResult(null);
                                    }}
                                >
                                    <Stack direction="row" alignItems="flex-start" spacing={1}>
                                        <Anthropic size={18} sx={{ mt: 0.2 }} />
                                        <Box sx={{ flex: 1 }}>
                                            <Typography variant="body2" fontWeight={500}>
                                                {t('providerDialog.apiStyle.anthropic')}
                                            </Typography>
                                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.2 }}>
                                                {t('providerDialog.apiStyle.helperAnthropic')}
                                            </Typography>
                                            {selectedProvider?.baseUrlAnthropic && (
                                                <ProtocolBaseUrlDisplay url={selectedProvider.baseUrlAnthropic} />
                                            )}
                                        </Box>
                                        <Checkbox
                                            size="small"
                                            checked={protocolAnthropic}
                                            disabled={selectedProvider ? !selectedProvider.supportsAnthropic : false}
                                            sx={{ p: 0, mt: -0.5 }}
                                        />
                                    </Stack>
                                </Box>
                            </Stack>
                        </FormControl>

                        {/* ③ API Key Field */}
                        <Box>
                            <TextField
                                size="small"
                                fullWidth
                                label={noApiKey ? 'API Key (Not Required)' : t('providerDialog.apiKey.label')}
                                type={showApiKey ? 'text' : 'password'}
                                value={data.token}
                                onChange={(e) => {
                                    onChange('token', e.target.value);
                                    setVerificationResult(null);
                                }}
                                required={!noApiKey}
                                placeholder={mode === 'add' ? t('providerDialog.apiKey.placeholderAdd') : t('providerDialog.apiKey.placeholderEdit')}
                                helperText={mode === 'edit' && t('providerDialog.apiKey.helperEdit')}
                                disabled={noApiKey}
                                slotProps={{
                                    input: {
                                        sx: {
                                            '& input': {
                                                textOverflow: 'ellipsis',
                                            },
                                        },
                                        endAdornment: (
                                            <InputAdornment position="end">
                                                <IconButton
                                                    size="small"
                                                    onClick={() => setShowApiKey(!showApiKey)}
                                                    edge="end"
                                                    disabled={noApiKey}
                                                >
                                                    {showApiKey ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                                </IconButton>
                                            </InputAdornment>
                                        ),
                                    },
                                }}
                            />
                            <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 0.5, pr: 2 }}>
                                <FormControlLabel
                                    control={
                                        <Checkbox
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
                                    label="No API Key Required"
                                    labelPlacement="start"
                                />
                            </Box>
                        </Box>

                        {/* ④ Name Field */}
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

                        {/* Proxy URL Field */}
                        <TextField
                            size="small"
                            fullWidth
                            label={t('providerDialog.advanced.proxyUrl.label')}
                            placeholder={t('providerDialog.advanced.proxyUrl.placeholder')}
                            value={data.proxyUrl || ''}
                            onChange={(e) => onChange('proxyUrl', e.target.value)}
                        />

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

                        {/* Verification Result */}
                        {verificationResult && (
                            <Alert
                                severity={verificationResult.success ? 'success' : 'warning'}
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
                                    {!verificationResult.success && (
                                        <Typography variant="body2" display="block" sx={{ mt: 1, color: 'text.secondary' }}>
                                            {t('providerDialog.verification.failureHint')}
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
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                    {/* Add/Save Anyway button - skip verification */}
                    <Button
                        type="button"
                        variant="outlined"
                        color="warning"
                        size="small"
                        disabled={!hasAnyProtocol}
                        onClick={() => onForceAdd?.()}
                        title="Skip connectivity check and save anyway. The provider may not work correctly if the connection fails."
                        sx={{
                            '&.Mui-disabled': {
                                color: 'text.disabled',
                                borderColor: 'action.disabledBackground',
                            },
                        }}
                    >
                        {mode === 'add' ? 'Add Anyway' : 'Save Anyway'}
                    </Button>
                    <Button
                        type="submit"
                        variant="contained"
                        size="small"
                        disabled={verifying || !hasAnyProtocol}
                        sx={{
                            minWidth: verifying ? '80px' : 'auto',
                        }}
                    >
                        {verifying ? (
                            <CircularProgress size={20} thickness={4} />
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
