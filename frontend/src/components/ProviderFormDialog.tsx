import { Close, Visibility, VisibilityOff } from '@mui/icons-material';
import {
    Alert,
    Autocomplete,
    Box,
    Button,
    Checkbox,
    Chip,
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
import { useProviderTemplates, type UniqueProvider } from '../services/serviceProviders';
import { api } from '../services/api';
import { OpenAI, Anthropic } from './BrandIcons';
import ProviderIcon from './ProviderIcon';

export interface EnhancedProviderFormData {
    uuid?: string;
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic' | undefined;
    token: string;
    noKeyRequired?: boolean;
    enabled?: boolean;
    proxyUrl?: string;
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

const OPENAI_RESPONSE_HINTS = ['responses', 'response'];
const OPENAI_CHAT_HINTS = ['chat/completions', '/chat', 'chat'];

const detectOpenAICapabilities = (provider: UniqueProvider | null) => {
    if (!provider?.baseUrlOpenAI) {
        return [] as string[];
    }

    const haystacks = [
        provider.baseUrlOpenAI,
        provider.apiDoc || '',
        provider.name,
        provider.alias || '',
    ].map(value => value.toLowerCase());

    const supportsResponses = OPENAI_RESPONSE_HINTS.some(hint => haystacks.some(text => text.includes(hint)));
    const supportsChat = OPENAI_CHAT_HINTS.some(hint => haystacks.some(text => text.includes(hint)));

    const capabilities: string[] = [];
    if (supportsChat) capabilities.push('Chat');
    if (supportsResponses) capabilities.push('Responses');
    return capabilities;
};

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
    const [selectedProvider, setSelectedProvider] = useState<UniqueProvider | null>(null);
    const [protocolOpenAI, setProtocolOpenAI] = useState(false);
    const [protocolAnthropic, setProtocolAnthropic] = useState(false);
    const [nameIsAutoFilled, setNameIsAutoFilled] = useState(true);

    const allProviders = useProviderTemplates();

    const openAICapabilities = useMemo(() => detectOpenAICapabilities(selectedProvider), [selectedProvider]);

    const ProtocolBaseUrlDisplay: React.FC<{ url: string }> = ({ url }) => {
        if (!url) return null;
        return (
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 0.5,
                    mt: 0.75,
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

    useEffect(() => {
        setNoApiKey(data.noKeyRequired || false);
    }, [data.noKeyRequired]);

    useEffect(() => {
        if (data.name && !nameIsAutoFilled) {
            return;
        }
    }, [data.name, nameIsAutoFilled]);

    useEffect(() => {
        if (open) {
            setVerificationResult(null);

            if (mode === 'edit') {
                setProtocolOpenAI(data.apiStyle === 'openai');
                setProtocolAnthropic(data.apiStyle === 'anthropic');

                const matchingProvider = allProviders.find(p =>
                    (p.baseUrlOpenAI === data.apiBase && data.apiStyle === 'openai') ||
                    (p.baseUrlAnthropic === data.apiBase && data.apiStyle === 'anthropic')
                );
                setSelectedProvider(matchingProvider || null);
            } else {
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
    }, [open, mode, data.apiBase, data.apiStyle, data.protocols, allProviders]);

    useEffect(() => {
        const protocols: ('openai' | 'anthropic')[] = [];
        if (protocolOpenAI) protocols.push('openai');
        if (protocolAnthropic) protocols.push('anthropic');
        onChange('protocols', protocols);

        if (protocols.length > 0) {
            onChange('apiStyle', protocols[0]);
        } else {
            onChange('apiStyle', undefined);
        }

        if (selectedProvider) {
            onChange('providerBaseUrls', {
                openai: selectedProvider.baseUrlOpenAI,
                anthropic: selectedProvider.baseUrlAnthropic,
            });
            if (protocolOpenAI && selectedProvider.baseUrlOpenAI) {
                onChange('apiBase', selectedProvider.baseUrlOpenAI);
            } else if (protocolAnthropic && selectedProvider.baseUrlAnthropic) {
                onChange('apiBase', selectedProvider.baseUrlAnthropic);
            }
        }
    }, [protocolOpenAI, protocolAnthropic, selectedProvider, onChange]);

    const handleProviderSelect = (newValue: string | UniqueProvider | null) => {
        setVerificationResult(null);

        if (typeof newValue === 'string') {
            setSelectedProvider(null);
            onChange('apiBase', newValue);
            onChange('providerBaseUrls', undefined);
        } else if (newValue) {
            setSelectedProvider(newValue);
            const displayName = newValue.alias || newValue.name;

            setProtocolOpenAI(newValue.supportsOpenAI);
            setProtocolAnthropic(newValue.supportsAnthropic);

            const baseUrl = newValue.baseUrlOpenAI || newValue.baseUrlAnthropic || '';
            onChange('apiBase', baseUrl);
            onChange('providerBaseUrls', {
                openai: newValue.baseUrlOpenAI,
                anthropic: newValue.baseUrlAnthropic,
            });

            if (nameIsAutoFilled || !data.name) {
                const autoName = t('providerDialog.keyName.autoFill', { title: displayName });
                onChange('name', autoName);
                setNameIsAutoFilled(true);
            }
        } else {
            setSelectedProvider(null);
            onChange('apiBase', '');
            onChange('providerBaseUrls', undefined);
            setProtocolOpenAI(false);
            setProtocolAnthropic(false);
        }
    };

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
            }

            setVerificationResult({
                success: false,
                message: result.error?.message || t('providerDialog.verification.failed'),
            });
            return false;
        } catch (_error) {
            setVerificationResult({
                success: false,
                message: t('providerDialog.verification.networkError'),
            });
            return false;
        } finally {
            setVerifying(false);
        }
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const shouldVerify = mode === 'add' ? !noApiKey : (data.token !== '' && !noApiKey);

        if (shouldVerify) {
            const verified = await handleVerify();
            if (!verified) {
                return;
            }
        }

        onClose();
        onSubmit(e);
    };

    const hasAnyProtocol = protocolOpenAI || protocolAnthropic;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth PaperProps={{ sx: { minHeight: 200 } }}>
            <DialogTitle>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    {title || defaultTitle}
                    <IconButton aria-label="close" onClick={onClose} sx={{ ml: 2 }} size="small">
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
                        {isFirstProvider && mode === 'add' && (
                            <Alert severity="info" sx={{ mb: 1 }}>
                                <Typography variant="body2">
                                    <strong>Getting Started</strong><br />
                                    Add your first API key to enable AI services. You can add more keys later.
                                </Typography>
                            </Alert>
                        )}

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
                            inputValue={selectedProvider ? (selectedProvider.alias || selectedProvider.name) : data.apiBase}
                            renderInput={(params) => (
                                <TextField
                                    {...params}
                                    label="Provider"
                                    placeholder="Select a provider or enter custom base URL"
                                />
                            )}
                            renderOption={(props, option) => {
                                const { key, ...optionProps } = props;
                                return (
                                    <Box component="li" key={key} {...optionProps} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                        {option.icon ? <ProviderIcon identifier={option.icon} size={18} /> : null}
                                        <Box>
                                            <Typography variant="body2">{option.alias || option.name}</Typography>
                                            <Typography variant="caption" color="text.secondary">
                                                {option.baseUrlOpenAI || option.baseUrlAnthropic}
                                            </Typography>
                                        </Box>
                                    </Box>
                                );
                            }}
                        />

                        <FormControl component="fieldset">
                            <FormLabel component="legend" sx={{ mb: 1 }}>
                                {t('providerDialog.apiStyle.label')}
                            </FormLabel>
                            <Stack spacing={1}>
                                <Box
                                    sx={{
                                        borderRadius: 1,
                                        px: 1.5,
                                        py: 1,
                                        cursor: mode === 'edit' ? 'not-allowed' : 'pointer',
                                        transition: 'all 0.15s',
                                        bgcolor: protocolOpenAI ? 'action.selected' : 'transparent',
                                        '&:hover': {
                                            bgcolor: mode === 'edit' ? (protocolOpenAI ? 'action.selected' : 'transparent') : (protocolOpenAI ? 'action.selected' : 'action.hover'),
                                        },
                                    }}
                                    onClick={() => {
                                        if (mode === 'edit') return;
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
                                                {openAICapabilities.length > 0
                                                    ? `Supports ${openAICapabilities.join(' + ')}`
                                                    : t('providerDialog.apiStyle.helperOpenAI')}
                                            </Typography>
                                            <Stack direction="row" spacing={0.75} sx={{ mt: 0.75, flexWrap: 'wrap', rowGap: 0.75 }}>
                                                {openAICapabilities.length > 0 ? (
                                                    openAICapabilities.map(capability => (
                                                        <Chip key={capability} label={capability} size="small" variant="outlined" color="primary" />
                                                    ))
                                                ) : (
                                                    <Chip label="OpenAI style" size="small" variant="outlined" color="primary" />
                                                )}
                                            </Stack>
                                            {selectedProvider?.baseUrlOpenAI && (
                                                <ProtocolBaseUrlDisplay url={selectedProvider.baseUrlOpenAI} />
                                            )}
                                        </Box>
                                        <Checkbox
                                            size="small"
                                            checked={protocolOpenAI}
                                            disabled={mode === 'edit' || (selectedProvider ? !selectedProvider.supportsOpenAI : false)}
                                            sx={{ p: 0, mt: -0.5 }}
                                        />
                                    </Stack>
                                </Box>

                                <Box
                                    sx={{
                                        borderRadius: 1,
                                        px: 1.5,
                                        py: 1,
                                        cursor: mode === 'edit' ? 'not-allowed' : 'pointer',
                                        transition: 'all 0.15s',
                                        bgcolor: protocolAnthropic ? 'action.selected' : 'transparent',
                                        '&:hover': {
                                            bgcolor: mode === 'edit' ? (protocolAnthropic ? 'action.selected' : 'transparent') : (protocolAnthropic ? 'action.selected' : 'action.hover'),
                                        },
                                    }}
                                    onClick={() => {
                                        if (mode === 'edit') return;
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
                                            disabled={mode === 'edit' || (selectedProvider ? !selectedProvider.supportsAnthropic : false)}
                                            sx={{ p: 0, mt: -0.5 }}
                                        />
                                    </Stack>
                                </Box>
                            </Stack>
                        </FormControl>

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

                        <TextField
                            size="small"
                            fullWidth
                            label={t('providerDialog.keyName.label')}
                            value={data.name}
                            onChange={(e) => {
                                onChange('name', e.target.value);
                                setVerificationResult(null);
                                setNameIsAutoFilled(false);
                            }}
                            required
                            placeholder={t('providerDialog.keyName.placeholder')}
                        />

                        <TextField
                            size="small"
                            fullWidth
                            label={t('providerDialog.advanced.proxyUrl.label')}
                            placeholder={t('providerDialog.advanced.proxyUrl.placeholder')}
                            value={data.proxyUrl || ''}
                            onChange={(e) => onChange('proxyUrl', e.target.value)}
                        />

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
