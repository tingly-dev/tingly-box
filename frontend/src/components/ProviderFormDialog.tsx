import {Close, Edit, InfoOutlined, Visibility, VisibilityOff} from '@mui/icons-material';
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
    Tooltip,
    Typography,
} from '@mui/material';
import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {type UniqueProvider, useProviderTemplates} from '../services/serviceProviders';
import {api} from '../services/api';
import {useFeatureFlags} from '@/contexts/FeatureFlagsContext';
import {Anthropic, OpenAI} from './BrandIcons';
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
    authType?: 'api_key' | 'oauth';
    protocols?: ('openai' | 'anthropic')[];
    providerBaseUrls?: { openai?: string; anthropic?: string };
    // Fusion-mode optional URLs. When both are set, the backend routes per
    // inbound protocol natively. When only one is set, falls back to apiBase.
    apiBaseOpenAI?: string;
    apiBaseAnthropic?: string;
    createFusionProvider?: boolean;
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
    const {t} = useTranslation();
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
    // In add mode the name field is hidden by default — almost everyone wants
    // the auto-generated label and can rename later. Edit mode always shows
    // the field so users can see/edit the existing name.
    const [showNameField, setShowNameField] = useState(mode === 'edit');
    const [providerInputValue, setProviderInputValue] = useState('');
    const [useGlobalProxy, setUseGlobalProxy] = useState(false);
    const [globalProxyUrl, setGlobalProxyUrl] = useState('');
    const [createFusionProvider, setCreateFusionProvider] = useState(false);

    const {enableFusion} = useFeatureFlags();

    const allProviders = useProviderTemplates();

    // Keep onChange in a ref so we can call it from effects/handlers without
    // putting it in dependency arrays (parent passes a fresh function each render).
    const onChangeRef = useRef(onChange);
    useEffect(() => {
        onChangeRef.current = onChange;
    });

    // Mirror the fusion flag into a ref so syncProtocolsToParent can read the
    // current value without being re-created (and thus not re-triggering the
    // open-effect that hydrates state).
    const enableFusionRef = useRef(enableFusion);
    useEffect(() => {
        enableFusionRef.current = enableFusion;
    }, [enableFusion]);

    const openAICapabilities = useMemo(
        () => detectOpenAICapabilities(selectedProvider),
        [selectedProvider]
    );

    // Find the matching template only when the dialog opens. Depend on
    // `open` (a stable boolean transition) rather than `data.apiBase` so that
    // typing in the field doesn't recompute this and re-trigger init.
    // We resolve in both modes so prefilled add-mode data (e.g. picked from
    // onboarding) shows the provider in the Autocomplete instead of blank.
    const matchingProvider = useMemo(() => {
        if (!open) return null;
        if (!data.apiBase) return null;
        // Match by apiBase alone - this handles onboarding prefills where apiStyle is undefined
        return (
            allProviders.find(
                p => p.baseUrlOpenAI === data.apiBase || p.baseUrlAnthropic === data.apiBase
            ) || null
        );
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, allProviders]);

    const ProtocolBaseUrlDisplay: React.FC<{ url: string }> = ({url}) => {
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
        setCreateFusionProvider(!!data.createFusionProvider);
    }, [data.createFusionProvider]);

    // Fetch global proxy URL once on mount
    useEffect(() => {
        api.getConfig().then((result) => {
            const gp = result?.data?.http_transport?.global_proxy_url ?? '';
            setGlobalProxyUrl(gp);
        });
    }, []);

    // Reset / initialise local state when the dialog opens.
    // Only runs on the open transition — never during typing.
    useEffect(() => {
        if (!open) return;

        setVerificationResult(null);
        // Hide the optional name field on each add-mode open; edit-mode keeps
        // it visible so users can review the existing name.
        setShowNameField(mode === 'edit');

        if (mode === 'edit') {
            // Hydrate fusion state: existing fusion URLs imply the protocol
            // is already enabled, regardless of the legacy apiStyle pin.
            const hasFusionOpenAI = !!data.apiBaseOpenAI;
            const hasFusionAnthropic = !!data.apiBaseAnthropic;
            setProtocolOpenAI(hasFusionOpenAI || data.apiStyle === 'openai');
            setProtocolAnthropic(hasFusionAnthropic || data.apiStyle === 'anthropic');
            setSelectedProvider(matchingProvider);
            setProviderInputValue(
                matchingProvider ? matchingProvider.alias || matchingProvider.name : data.apiBase
            );
        } else {
            if (data.protocols && data.protocols.length > 0) {
                setProtocolOpenAI(data.protocols.includes('openai'));
                setProtocolAnthropic(data.protocols.includes('anthropic'));
            } else if (data.apiStyle) {
                setProtocolOpenAI(data.apiStyle === 'openai');
                setProtocolAnthropic(data.apiStyle === 'anthropic');
            } else if (matchingProvider) {
                // When apiBase matches a known provider (e.g., from onboarding),
                // auto-select the provider's supported protocols
                setProtocolOpenAI(!!matchingProvider.baseUrlOpenAI);
                setProtocolAnthropic(!!matchingProvider.baseUrlAnthropic);
            } else {
                setProtocolOpenAI(false);
                setProtocolAnthropic(false);
            }
            // If the parent prefilled apiBase to a known provider (onboarding
            // browse / paste-detect), seed the Autocomplete with it so users
            // see the picked provider rather than a blank field.
            setSelectedProvider(matchingProvider);
            setProviderInputValue(
                matchingProvider
                    ? matchingProvider.alias || matchingProvider.name
                    : data.apiBase || ''
            );
        }

        // Restore "use global proxy" checkbox state from localStorage (add mode only)
        if (mode === 'add') {
            const savedUseGlobal = localStorage.getItem('provider_use_global_proxy') === 'true';
            setUseGlobalProxy(savedUseGlobal);
            if (savedUseGlobal && globalProxyUrl && !data.proxyUrl) {
                onChangeRef.current('proxyUrl', globalProxyUrl);
            }
        } else {
            setUseGlobalProxy(false);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open]);

    // Helper: push protocol-related fields to parent in one batch.
    // Called only from user-driven handlers (not from a render-triggered effect).
    //
    // Fusion-mode rule: when BOTH protocols are checked and we have a known
    // template (provider) with both base URLs, emit `apiBaseOpenAI` +
    // `apiBaseAnthropic` so the backend stores them as fusion fields. The
    // legacy `apiBase`/`apiStyle` still get populated (set to the openai URL
    // by convention) so single-protocol consumers keep working.
    //
    // When only ONE protocol is checked, behavior matches the previous
    // single-protocol UX: pick the matching URL into `apiBase` and clear the
    // other-side fusion field.
    const syncProtocolsToParent = useCallback(
        (
            nextOpenAI: boolean,
            nextAnthropic: boolean,
            provider: UniqueProvider | null
        ) => {
            const protocols: ('openai' | 'anthropic')[] = [];
            if (nextOpenAI) protocols.push('openai');
            if (nextAnthropic) protocols.push('anthropic');

            const cb = onChangeRef.current;
            cb('protocols', protocols);
            cb('apiStyle', protocols.length > 0 ? protocols[0] : undefined);

            if (provider) {
                cb('providerBaseUrls', {
                    openai: provider.baseUrlOpenAI,
                    anthropic: provider.baseUrlAnthropic,
                });

                // Fusion-mode is only available when the global experiment is
                // on. With the flag OFF, picking both protocols falls through
                // to the legacy two-record split handled by the parent submit.
                const fusion = enableFusionRef.current
                    && createFusionProvider
                    && nextOpenAI && nextAnthropic
                    && !!provider.baseUrlOpenAI && !!provider.baseUrlAnthropic;

                if (fusion) {
                    cb('apiBaseOpenAI', provider.baseUrlOpenAI);
                    cb('apiBaseAnthropic', provider.baseUrlAnthropic);
                    // Use the OpenAI URL as the legacy primary so single-
                    // protocol consumers (model probes, model list, etc.)
                    // see a populated apiBase.
                    cb('apiBase', provider.baseUrlOpenAI);
                } else if (nextOpenAI && provider.baseUrlOpenAI) {
                    cb('apiBase', provider.baseUrlOpenAI);
                    cb('apiBaseOpenAI', '');
                    cb('apiBaseAnthropic', '');
                } else if (nextAnthropic && provider.baseUrlAnthropic) {
                    cb('apiBase', provider.baseUrlAnthropic);
                    cb('apiBaseOpenAI', '');
                    cb('apiBaseAnthropic', '');
                } else {
                    cb('apiBaseOpenAI', '');
                    cb('apiBaseAnthropic', '');
                }
            } else {
                // Free-form (no template) — fusion requires a known template.
                cb('apiBaseOpenAI', '');
                cb('apiBaseAnthropic', '');
            }
        },
        [createFusionProvider]
    );

    const handleUseGlobalProxyChange = (checked: boolean) => {
        setUseGlobalProxy(checked);
        localStorage.setItem('provider_use_global_proxy', String(checked));
        if (checked && globalProxyUrl) {
            onChange('proxyUrl', globalProxyUrl);
        } else if (!checked) {
            onChange('proxyUrl', '');
        }
    };

    // OAuth-bound providers are issuer-locked to a single protocol; fusion
    // is api_key only.
    const fusionLocked = data.authType === 'oauth';

    const toggleOpenAIProtocol = () => {
        if (fusionLocked) return;
        if (selectedProvider && !selectedProvider.supportsOpenAI) return;
        const next = !protocolOpenAI;
        setProtocolOpenAI(next);
        setVerificationResult(null);
        syncProtocolsToParent(next, protocolAnthropic, selectedProvider);
    };

    const toggleAnthropicProtocol = () => {
        if (fusionLocked) return;
        if (selectedProvider && !selectedProvider.supportsAnthropic) return;
        const next = !protocolAnthropic;
        setProtocolAnthropic(next);
        setVerificationResult(null);
        syncProtocolsToParent(protocolOpenAI, next, selectedProvider);
    };

    const handleProviderSelect = (newValue: string | UniqueProvider | null) => {
        setVerificationResult(null);
        const cb = onChangeRef.current;

        if (typeof newValue === 'string') {
            // User pressed Enter on a free-typed string
            setSelectedProvider(null);
            cb('apiBase', newValue);
            cb('providerBaseUrls', undefined);
            setProviderInputValue(newValue);
            return;
        }

        if (newValue) {
            // Picked a known provider from the list
            setSelectedProvider(newValue);
            const displayName = newValue.alias || newValue.name;

            const nextOpenAI = newValue.supportsOpenAI;
            const nextAnthropic = newValue.supportsAnthropic;
            setProtocolOpenAI(nextOpenAI);
            setProtocolAnthropic(nextAnthropic);

            const baseUrl = newValue.baseUrlOpenAI || newValue.baseUrlAnthropic || '';
            cb('apiBase', baseUrl);
            cb('providerBaseUrls', {
                openai: newValue.baseUrlOpenAI,
                anthropic: newValue.baseUrlAnthropic,
            });
            // Sync protocols/apiStyle in the same batch
            syncProtocolsToParent(nextOpenAI, nextAnthropic, newValue);

            setProviderInputValue(displayName);

            if (nameIsAutoFilled || !data.name) {
                cb('name', displayName);
                setNameIsAutoFilled(true);
            }
            return;
        }

        // Cleared
        setSelectedProvider(null);
        cb('apiBase', '');
        cb('providerBaseUrls', undefined);
        setProtocolOpenAI(false);
        setProtocolAnthropic(false);
        syncProtocolsToParent(false, false, null);
        setProviderInputValue('');
    };

    // IMPORTANT: do NOT call parent onChange on every keystroke here.
    // We only update the local input state. apiBase gets written:
    //   - when the user picks an option (handleProviderSelect)
    //   - when the user blurs the input (handleProviderInputBlur)
    //   - or implicitly on submit (we read providerInputValue if needed).
    // This is what made typing feel snappy again.
    const handleProviderInputChange = (
        _event: React.SyntheticEvent,
        newValue: string
    ) => {
        setProviderInputValue(newValue);
        // If the user is editing away from the selected provider's display name,
        // detach the selection so the protocol checkboxes become editable again.
        if (
            selectedProvider &&
            newValue !== (selectedProvider.alias || selectedProvider.name)
        ) {
            setSelectedProvider(null);
        }
    };

    const handleProviderInputBlur = () => {
        // Commit free-form input to apiBase if it doesn't match a selected provider.
        if (!selectedProvider) {
            const cb = onChangeRef.current;
            if (data.apiBase !== providerInputValue) {
                cb('apiBase', providerInputValue);
                cb('providerBaseUrls', undefined);
            }
        }
    };

    // Compute a sensible default name when the user leaves the field blank.
    // Uses the provider's display label directly (no " API Key" suffix — the
    // credential is already in the API Keys section, so the suffix is noise).
    // Falls back to apiBase hostname or a generic label.
    const computeAutoName = useCallback((): string => {
        if (selectedProvider) {
            return selectedProvider.alias || selectedProvider.name;
        }
        const raw = data.apiBase || providerInputValue || '';
        try {
            const host = new URL(raw).hostname;
            if (host) return host;
        } catch { /* not a URL */ }
        return t('providerDialog.keyName.fallback', {defaultValue: 'Custom Provider'});
    }, [selectedProvider, data.apiBase, providerInputValue, t]);

    // Ensure a name exists before submit/verify. Writes back to parent so the
    // submit payload carries the generated value.
    const ensureName = (): string => {
        if (data.name && data.name.trim()) return data.name;
        const auto = computeAutoName();
        onChangeRef.current('name', auto);
        return auto;
    };

    const handleVerify = async () => {
        if (noApiKey) {
            setVerificationResult(null);
            return true;
        }

        const apiStyle = protocolOpenAI ? 'openai' : protocolAnthropic ? 'anthropic' : undefined;
        const apiBase =
            protocolOpenAI && selectedProvider?.baseUrlOpenAI
                ? selectedProvider.baseUrlOpenAI
                : protocolAnthropic && selectedProvider?.baseUrlAnthropic
                    ? selectedProvider.baseUrlAnthropic
                    : data.apiBase || providerInputValue;

        const effectiveName = ensureName();

        if (!effectiveName || !apiBase || !data.token || !apiStyle) {
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
                    details: isValid
                        ? t('providerDialog.verification.testResult', {result: result.data.test_result})
                        : undefined,
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

        // Make sure any free-form text in the provider input is committed before submit.
        if (!selectedProvider && data.apiBase !== providerInputValue) {
            onChangeRef.current('apiBase', providerInputValue);
            onChangeRef.current('providerBaseUrls', undefined);
        }

        ensureName();

        const shouldVerify = mode === 'add' ? !noApiKey : data.token !== '' && !noApiKey;

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
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth PaperProps={{sx: {minHeight: 200}}}>
            <DialogTitle>
                <Box sx={{display: 'flex', alignItems: 'center', justifyContent: 'space-between'}}>
                    {title || defaultTitle}
                    <IconButton aria-label="close" onClick={onClose} sx={{ml: 2}} size="small">
                        <Close/>
                    </IconButton>
                </Box>
            </DialogTitle>
            <form onSubmit={handleSubmit}>
                <DialogContent sx={{pt: 1, pb: 1, minHeight: 280}}>
                    {mode === 'add' && (
                        <Typography variant="body2" color="text.secondary" sx={{mb: 2}}>
                            {t('providerDialog.addDescription')}
                        </Typography>
                    )}
                    <Stack spacing={2.5}>
                        {isFirstProvider && mode === 'add' && (
                            <Alert severity="info" sx={{mb: 1}}>
                                <Typography variant="body2">
                                    <strong>Getting Started</strong>
                                    <br/>
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
                                const inputValue = state.inputValue.trim().toLowerCase();
                                if (!inputValue) return options;
                                return options.filter(option => {
                                    const displayName = (option.alias || option.name).toLowerCase();
                                    return (
                                        displayName.includes(inputValue) ||
                                        (option.baseUrlOpenAI || '').toLowerCase().includes(inputValue) ||
                                        (option.baseUrlAnthropic || '').toLowerCase().includes(inputValue)
                                    );
                                });
                            }}
                            getOptionLabel={(option) => {
                                if (typeof option === 'string') return option;
                                return option.alias || option.name;
                            }}
                            isOptionEqualToValue={(option, value) =>
                                typeof option !== 'string' &&
                                typeof value !== 'string' &&
                                option.id === value.id
                            }
                            value={selectedProvider}
                            onChange={(_event, newValue) => handleProviderSelect(newValue)}
                            inputValue={providerInputValue}
                            onInputChange={handleProviderInputChange}
                            onBlur={handleProviderInputBlur}
                            renderInput={(params) => (
                                <TextField
                                    {...params}
                                    label="Provider"
                                    placeholder="Select a provider or enter custom base URL"
                                />
                            )}
                            renderOption={(props, option) => {
                                const {key, ...optionProps} = props;
                                return (
                                    <Box
                                        component="li"
                                        key={key}
                                        {...optionProps}
                                        sx={{display: 'flex', alignItems: 'center', gap: 1}}
                                    >
                                        {option.icon ? <ProviderIcon identifier={option.icon} size={18}/> : null}
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
                            <FormLabel component="legend" sx={{mb: 1}}>
                                {t('providerDialog.apiStyle.label')}
                            </FormLabel>
                            <Stack spacing={1}>
                                <Box
                                    sx={{
                                        borderRadius: 1,
                                        px: 1.5,
                                        py: 1,
                                        cursor: fusionLocked ? 'not-allowed' : 'pointer',
                                        transition: 'all 0.15s',
                                        bgcolor: protocolOpenAI ? 'action.selected' : 'transparent',
                                        '&:hover': {
                                            bgcolor:
                                                fusionLocked
                                                    ? protocolOpenAI
                                                        ? 'action.selected'
                                                        : 'transparent'
                                                    : protocolOpenAI
                                                        ? 'action.selected'
                                                        : 'action.hover',
                                        },
                                    }}
                                    onClick={toggleOpenAIProtocol}
                                >
                                    <Stack direction="row" alignItems="flex-start" spacing={1}>
                                        <OpenAI size={18} sx={{mt: 0.2}}/>
                                        <Box sx={{flex: 1}}>
                                            <Typography variant="body2" fontWeight={500}>
                                                {t('providerDialog.apiStyle.openAI')}
                                            </Typography>
                                            <Typography
                                                variant="caption"
                                                color="text.secondary"
                                                sx={{display: 'block', lineHeight: 1.2}}
                                            >
                                                {openAICapabilities.length > 0
                                                    ? `Supports ${openAICapabilities.join(' + ')}`
                                                    : t('providerDialog.apiStyle.helperOpenAI')}
                                            </Typography>
                                            <Stack
                                                direction="row"
                                                spacing={0.75}
                                                sx={{mt: 0.75, flexWrap: 'wrap', rowGap: 0.75}}
                                            >
                                                {openAICapabilities.length > 0 &&
                                                    openAICapabilities.map(capability => (
                                                        <Chip
                                                            key={capability}
                                                            label={capability}
                                                            size="small"
                                                            variant="outlined"
                                                            color="primary"
                                                        />
                                                    ))}
                                            </Stack>
                                            {selectedProvider?.baseUrlOpenAI && (
                                                <ProtocolBaseUrlDisplay url={selectedProvider.baseUrlOpenAI}/>
                                            )}
                                        </Box>
                                        <Checkbox
                                            size="small"
                                            checked={protocolOpenAI}
                                            disabled={
                                                fusionLocked ||
                                                (selectedProvider ? !selectedProvider.supportsOpenAI : false)
                                            }
                                            sx={{p: 0, mt: -0.5}}
                                            onClick={(e) => e.stopPropagation()}
                                            onChange={toggleOpenAIProtocol}
                                        />
                                    </Stack>
                                </Box>

                                <Box
                                    sx={{
                                        borderRadius: 1,
                                        px: 1.5,
                                        py: 1,
                                        cursor: fusionLocked ? 'not-allowed' : 'pointer',
                                        transition: 'all 0.15s',
                                        bgcolor: protocolAnthropic ? 'action.selected' : 'transparent',
                                        '&:hover': {
                                            bgcolor:
                                                fusionLocked
                                                    ? protocolAnthropic
                                                        ? 'action.selected'
                                                        : 'transparent'
                                                    : protocolAnthropic
                                                        ? 'action.selected'
                                                        : 'action.hover',
                                        },
                                    }}
                                    onClick={toggleAnthropicProtocol}
                                >
                                    <Stack direction="row" alignItems="flex-start" spacing={1}>
                                        <Anthropic size={18} sx={{mt: 0.2}}/>
                                        <Box sx={{flex: 1}}>
                                            <Typography variant="body2" fontWeight={500}>
                                                {t('providerDialog.apiStyle.anthropic')}
                                            </Typography>
                                            <Typography
                                                variant="caption"
                                                color="text.secondary"
                                                sx={{display: 'block', lineHeight: 1.2}}
                                            >
                                                {t('providerDialog.apiStyle.helperAnthropic')}
                                            </Typography>
                                            {selectedProvider?.baseUrlAnthropic && (
                                                <ProtocolBaseUrlDisplay url={selectedProvider.baseUrlAnthropic}/>
                                            )}
                                        </Box>
                                        <Checkbox
                                            size="small"
                                            checked={protocolAnthropic}
                                            disabled={
                                                fusionLocked ||
                                                (selectedProvider ? !selectedProvider.supportsAnthropic : false)
                                            }
                                            sx={{p: 0, mt: -0.5}}
                                            onClick={(e) => e.stopPropagation()}
                                            onChange={toggleAnthropicProtocol}
                                        />
                                    </Stack>
                                </Box>
                            </Stack>
                        </FormControl>
                        {enableFusion && mode === 'add' && protocolOpenAI && protocolAnthropic && (
                            <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: -0.5, pr: 2}}>
                                <FormControlLabel
                                    control={
                                        <Checkbox
                                            size="small"
                                            checked={createFusionProvider}
                                            onChange={(e) => {
                                                const checked = e.target.checked;
                                                setCreateFusionProvider(checked);
                                                onChange('createFusionProvider', checked);
                                                syncProtocolsToParent(protocolOpenAI, protocolAnthropic, selectedProvider);
                                                setVerificationResult(null);
                                            }}
                                        />
                                    }
                                    label={(
                                        <Stack direction="row" spacing={0.75} alignItems="center">
                                            <Typography variant="body2">
                                                {t('providerDialog.fusion.modeLabel')}
                                            </Typography>
                                            <Tooltip
                                                arrow
                                                placement="top"
                                                slotProps={{
                                                    tooltip: {
                                                        sx: (theme) => ({
                                                            maxWidth: 360,
                                                            bgcolor: 'background.paper',
                                                            color: 'text.primary',
                                                            border: `1px solid ${theme.palette.divider}`,
                                                            boxShadow: theme.shadows[6],
                                                            p: 1.25,
                                                            '& .MuiTypography-caption': {
                                                                color: 'text.secondary',
                                                                lineHeight: 1.45,
                                                            },
                                                        }),
                                                    },
                                                    arrow: {
                                                        sx: (theme) => ({
                                                            color: theme.palette.background.paper,
                                                            '&:before': {
                                                                border: `1px solid ${theme.palette.divider}`,
                                                            },
                                                        }),
                                                    },
                                                }}
                                                title={
                                                    <Box>
                                                        <Typography variant="body2" sx={{fontWeight: 600, mb: 0.5}}>
                                                            {t('providerDialog.fusion.tooltipTitle')}
                                                        </Typography>
                                                        <Typography variant="caption" sx={{display: 'block'}}>
                                                            {t('providerDialog.fusion.normalModeDesc')}
                                                        </Typography>
                                                        <Typography variant="caption" sx={{display: 'block', mt: 0.75}}>
                                                            {t('providerDialog.fusion.fusionModeDesc')}
                                                        </Typography>
                                                    </Box>
                                                }
                                            >
                                                <InfoOutlined sx={{fontSize: 16, color: 'text.secondary'}} />
                                            </Tooltip>
                                        </Stack>
                                    )}
                                    labelPlacement="start"
                                />
                            </Box>
                        )}

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
                                placeholder={
                                    mode === 'add'
                                        ? t('providerDialog.apiKey.placeholderAdd')
                                        : t('providerDialog.apiKey.placeholderEdit')
                                }
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
                                                    {showApiKey ? (
                                                        <VisibilityOff fontSize="small"/>
                                                    ) : (
                                                        <Visibility fontSize="small"/>
                                                    )}
                                                </IconButton>
                                            </InputAdornment>
                                        ),
                                    },
                                }}
                            />
                            <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: 0.5, pr: 2}}>
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

                        {showNameField ? (
                            <TextField
                                size="small"
                                fullWidth
                                autoFocus
                                label={t('providerDialog.keyName.label')}
                                value={data.name}
                                onChange={(e) => {
                                    onChange('name', e.target.value);
                                    setVerificationResult(null);
                                    setNameIsAutoFilled(false);
                                }}
                                placeholder={computeAutoName()}
                                helperText={t('providerDialog.keyName.helper', {
                                    defaultValue: 'Leave blank to use the auto-generated name. You can rename later.',
                                })}
                            />
                        ) : (
                            <Box
                                sx={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: 1,
                                    px: 1.5,
                                    py: 0.75,
                                    borderRadius: 1,
                                    bgcolor: 'background.default',
                                    border: 1,
                                    borderColor: 'divider',
                                }}
                            >
                                <Typography variant="caption" color="text.secondary">
                                    {t('providerDialog.keyName.label')}
                                </Typography>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        flex: 1,
                                        color: 'text.primary',
                                        overflow: 'hidden',
                                        textOverflow: 'ellipsis',
                                        whiteSpace: 'nowrap',
                                    }}
                                >
                                    {data.name || computeAutoName()}
                                </Typography>
                                <Tooltip
                                    title={t('providerDialog.keyName.editAction', {
                                        defaultValue: 'Edit name',
                                    })}
                                    arrow
                                >
                                    <IconButton
                                        size="small"
                                        onClick={() => {
                                            if (!data.name) {
                                                onChangeRef.current('name', computeAutoName());
                                            }
                                            setShowNameField(true);
                                        }}
                                        sx={{color: 'text.secondary'}}
                                    >
                                        <Edit fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        )}

                        <Box>
                            <TextField
                                size="small"
                                fullWidth
                                label={t('providerDialog.advanced.proxyUrl.label')}
                                placeholder={t('providerDialog.advanced.proxyUrl.placeholder')}
                                value={data.proxyUrl || ''}
                                onChange={(e) => {
                                    onChange('proxyUrl', e.target.value);
                                    if (useGlobalProxy && e.target.value !== globalProxyUrl) {
                                        setUseGlobalProxy(false);
                                        localStorage.setItem('provider_use_global_proxy', 'false');
                                    }
                                }}
                            />
                            {mode === 'add' && (
                                <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: 0.5, pr: 2}}>
                                    <FormControlLabel
                                        control={
                                            <Checkbox
                                                size="small"
                                                checked={useGlobalProxy}
                                                disabled={!globalProxyUrl}
                                                onChange={(e) => handleUseGlobalProxyChange(e.target.checked)}
                                            />
                                        }
                                        label={
                                            <Typography variant="body2" color={globalProxyUrl ? 'text.secondary' : 'text.disabled'}>
                                                {globalProxyUrl
                                                    ? t('providerDialog.advanced.proxyUrl.useGlobal', {url: globalProxyUrl})
                                                    : t('providerDialog.advanced.proxyUrl.useGlobalNotSet')}
                                            </Typography>
                                        }
                                        labelPlacement="start"
                                    />
                                </Box>
                            )}
                        </Box>

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
                                sx={{mt: 1}}
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
                                        <Typography
                                            variant="body2"
                                            display="block"
                                            sx={{mt: 1, color: 'text.secondary'}}
                                        >
                                            {t('providerDialog.verification.failureHint')}
                                        </Typography>
                                    )}
                                    {verificationResult.responseTime && (
                                        <Typography variant="caption" display="block">
                                            {t('providerDialog.verification.responseTime', {
                                                time: verificationResult.responseTime,
                                            })}
                                            {verificationResult.modelsCount &&
                                                ` • ${t('providerDialog.verification.modelsAvailable', {
                                                    count: verificationResult.modelsCount,
                                                })}`}
                                        </Typography>
                                    )}
                                </Box>
                            </Alert>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions sx={{px: 3, pb: 2, gap: 1, justifyContent: 'flex-end'}}>
                    <Button
                        type="button"
                        variant="outlined"
                        color="warning"
                        size="small"
                        disabled={!hasAnyProtocol || verifying}
                        onClick={async () => {
                            // Make sure any free-form text in the provider input is committed
                            if (!selectedProvider && data.apiBase !== providerInputValue) {
                                onChangeRef.current('apiBase', providerInputValue);
                                onChangeRef.current('providerBaseUrls', undefined);
                            }
                            ensureName();
                            onClose();
                            await onForceAdd?.();
                        }}
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
                            <CircularProgress size={20} thickness={4}/>
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
