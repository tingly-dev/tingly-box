import {ArrowBack, Close, ExpandMore, InfoOutlined} from '@mui/icons-material';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';
import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {type UniqueProvider, useProviderTemplates} from '../services/serviceProviders';
import {api} from '../services/api';
import {useFeatureFlags} from '@/contexts/FeatureFlagsContext';
import ApiKeyField from './providerFormDialog/ApiKeyField';
import FusionToggle from './providerFormDialog/FusionToggle';
import KeyNameField from './providerFormDialog/KeyNameField';
import ProtocolSelector from './providerFormDialog/ProtocolSelector';
import ProviderAutocomplete from './providerFormDialog/ProviderAutocomplete';
import ProxyUrlField from './providerFormDialog/ProxyUrlField';
import VerificationResultPanel from './providerFormDialog/VerificationResultPanel';
import {detectOpenAICapabilities} from './providerFormDialog/helpers';
import {type VerificationResult, runProviderProbe} from './providerFormDialog/probe';

export interface EnhancedProviderFormData {
    uuid?: string;
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic' | undefined;
    token: string;
    noKeyRequired?: boolean;
    enabled?: boolean;
    proxyUrl?: string;
    userAgent?: string;
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
    // `resolved` carries fields the dialog finalises at submit time (free-typed
    // apiBase, the auto-generated name, etc). Parents must merge it over their
    // form state because those values are committed via async onChange and are
    // not yet visible in state when this fires.
    onSubmit: (e: React.FormEvent, resolved?: Partial<EnhancedProviderFormData>) => void | Promise<void>;
    onForceAdd?: () => void;
    onBack?: () => void;
    data: EnhancedProviderFormData;
    onChange: (field: keyof EnhancedProviderFormData, value: any) => void;
    mode: 'add' | 'edit';
    title?: string;
    submitText?: string;
    isFirstProvider?: boolean;
    /** Pass true for local providers: token field stays editable but is not required. */
    optionalEditableToken?: boolean;
}

const ProviderFormDialog = ({
                                open,
                                onClose,
                                onSubmit,
                                onBack,
                                data,
                                onChange,
                                mode,
                                title,
                                submitText,
                                isFirstProvider = false,
                                optionalEditableToken = false,
                            }: PresetProviderFormDialogProps) => {
    const {t} = useTranslation();
    const defaultTitle = mode === 'add' ? t('providerDialog.addTitle') : t('providerDialog.editTitle');
    const defaultSubmitText = mode === 'add' ? t('providerDialog.addButton') : t('common.saveChanges');

    const [verifying, setVerifying] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [noApiKey, setNoApiKey] = useState(data.noKeyRequired || false);
    const [verificationResult, setVerificationResult] = useState<VerificationResult | null>(null);
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
    const [advancedOpen, setAdvancedOpen] = useState(false);
    const [baseUrlError, setBaseUrlError] = useState(false);

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
        setBaseUrlError(false);
        // Edit mode opens the advanced panel so users can see/change existing settings.
        setAdvancedOpen(mode === 'edit');
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
        setBaseUrlError(false);
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
        if (newValue.trim()) setBaseUrlError(false);
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

        const result = await runProviderProbe(
            {name: effectiveName, apiStyle, apiBase, token: data.token, authType: data.authType},
            {
                failed: t('providerDialog.verification.failed'),
                networkError: t('providerDialog.verification.networkError'),
            },
        );
        setVerificationResult(result);
        setVerifying(false);
        return result.success;
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const effectiveBase = data.apiBase || providerInputValue;
        if (!effectiveBase.trim()) {
            setBaseUrlError(true);
            return;
        }

        // Collect the values we finalise here so the parent can use them
        // directly. onChange writes to parent state asynchronously, so the
        // parent's submit closure would otherwise read stale values — that's
        // why a free-typed provider could not be added without first clicking
        // "Test Connection" (which triggered extra renders that flushed state).
        const resolved: Partial<EnhancedProviderFormData> = {};

        // Make sure any free-form text in the provider input is committed before submit.
        if (!selectedProvider && data.apiBase !== providerInputValue) {
            onChangeRef.current('apiBase', providerInputValue);
            onChangeRef.current('providerBaseUrls', undefined);
            resolved.apiBase = providerInputValue;
            (resolved as any).providerBaseUrls = undefined;
        }

        resolved.name = ensureName();

        // NO MANDATORY VERIFICATION - allow adding keys without testing
        // Verification is optional via the "Test Connection" button.
        //
        // Do NOT close the dialog here: the parent's submit handler is async
        // and closes the dialog itself only after the add/update succeeds.
        // Closing eagerly dismissed the dialog even when the request failed,
        // making it look like the key was saved when it was not.
        //
        // Await so the button can show a spinner while the request is in
        // flight. On success the parent unmounts this dialog; on failure it
        // stays open and the spinner clears so the user can retry.
        setSubmitting(true);
        try {
            await onSubmit(e, resolved);
        } finally {
            setSubmitting(false);
        }
    };

    const hasAnyProtocol = protocolOpenAI || protocolAnthropic;
    const showFusionToggle = enableFusion && mode === 'add' && protocolOpenAI && protocolAnthropic;

    // When both protocols are checked on a template that exposes two base URLs,
    // the outcome ("merge into one" vs "create two") is otherwise invisible.
    // Surface it as a one-line hint that tracks the fusion toggle.
    const hasBothBaseUrls = !!selectedProvider?.baseUrlOpenAI && !!selectedProvider?.baseUrlAnthropic;
    const showTopologyHint = mode === 'add' && protocolOpenAI && protocolAnthropic && hasBothBaseUrls;
    const willMergeBaseUrls = enableFusion && createFusionProvider;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth
            PaperProps={{sx: {maxHeight: '88vh', display: 'flex', flexDirection: 'column'}}}>
            <DialogTitle sx={{flexShrink: 0}}>
                <Box sx={{display: 'flex', alignItems: 'center', justifyContent: 'space-between'}}>
                    {title || defaultTitle}
                    <IconButton aria-label="close" onClick={onClose} sx={{ml: 2}} size="small">
                        <Close/>
                    </IconButton>
                </Box>
            </DialogTitle>
            <form onSubmit={handleSubmit} style={{display: 'flex', flexDirection: 'column', flex: 1, overflow: 'hidden'}}>
                <DialogContent sx={{pt: 1, pb: 1, overflowY: 'auto', flex: 1}}>
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

                        <ProviderAutocomplete
                            options={allProviders}
                            value={selectedProvider}
                            inputValue={providerInputValue}
                            onChange={handleProviderSelect}
                            onInputChange={handleProviderInputChange}
                            onBlur={handleProviderInputBlur}
                            required
                            error={baseUrlError}
                            helperText={baseUrlError ? t('providerDialog.provider.required', {defaultValue: 'Base URL is required'}) : undefined}
                        />

                        <ApiKeyField
                            mode={mode}
                            token={data.token}
                            onTokenChange={(value) => {
                                onChange('token', value);
                                setVerificationResult(null);
                            }}
                            noApiKey={noApiKey}
                            optionalEditable={optionalEditableToken}
                            onNoApiKeyChange={(checked) => {
                                setNoApiKey(checked);
                                onChange('noKeyRequired', checked);
                                setVerificationResult(null);
                                if (checked && !optionalEditableToken) {
                                    onChange('token', '');
                                }
                            }}
                        />

                        <ProtocolSelector
                            selectedProvider={selectedProvider}
                            protocolOpenAI={protocolOpenAI}
                            protocolAnthropic={protocolAnthropic}
                            fusionLocked={fusionLocked}
                            openAICapabilities={openAICapabilities}
                            onToggleOpenAI={toggleOpenAIProtocol}
                            onToggleAnthropic={toggleAnthropicProtocol}
                        />

                        {showFusionToggle && (
                            <FusionToggle
                                checked={createFusionProvider}
                                onChange={(checked) => {
                                    setCreateFusionProvider(checked);
                                    onChange('createFusionProvider', checked);
                                    syncProtocolsToParent(protocolOpenAI, protocolAnthropic, selectedProvider);
                                    setVerificationResult(null);
                                }}
                            />
                        )}

                        {showTopologyHint && (
                            <Stack
                                direction="row"
                                spacing={1}
                                alignItems="flex-start"
                                sx={{
                                    mt: -1,
                                    px: 1.5,
                                    py: 1,
                                    borderRadius: 1,
                                    bgcolor: 'action.hover',
                                }}
                            >
                                <InfoOutlined sx={{fontSize: 16, mt: 0.2, color: 'text.secondary'}}/>
                                <Typography variant="caption" color="text.secondary" sx={{lineHeight: 1.4}}>
                                    {willMergeBaseUrls
                                        ? t('providerDialog.fusion.outcomeMerged')
                                        : t('providerDialog.fusion.outcomeSplit')}
                                </Typography>
                            </Stack>
                        )}

                        {verificationResult && (
                            <VerificationResultPanel
                                result={verificationResult}
                                onClose={() => setVerificationResult(null)}
                            />
                        )}

                        {/* Advanced accordion — name, proxy, user-agent, enabled */}
                        <Accordion
                            disableGutters
                            elevation={0}
                            expanded={advancedOpen}
                            onChange={(_, expanded) => setAdvancedOpen(expanded)}
                            sx={{
                                border: 0,
                                borderTop: 1,
                                borderColor: 'divider',
                                '&:before': {display: 'none'},
                                bgcolor: 'transparent',
                            }}
                        >
                            <AccordionSummary
                                expandIcon={<ExpandMore fontSize="small"/>}
                                sx={{
                                    px: 0,
                                    minHeight: 40,
                                    '& .MuiAccordionSummary-content': {my: 0.5},
                                }}
                            >
                                <Typography variant="body2" color="text.secondary" fontWeight={600}>
                                    {t('providerDialog.advanced.label', {defaultValue: 'Advanced — proxy, user-agent, name'})}
                                </Typography>
                            </AccordionSummary>
                            <AccordionDetails sx={{px: 0, pb: 1}}>
                                <Stack spacing={2.5}>
                                    <KeyNameField
                                        showField={showNameField}
                                        onShowField={() => {
                                            if (!data.name) {
                                                onChangeRef.current('name', computeAutoName());
                                            }
                                            setShowNameField(true);
                                        }}
                                        name={data.name}
                                        autoName={computeAutoName()}
                                        onNameChange={(value) => {
                                            onChange('name', value);
                                            setVerificationResult(null);
                                            setNameIsAutoFilled(false);
                                        }}
                                    />

                                    <ProxyUrlField
                                        mode={mode}
                                        proxyUrl={data.proxyUrl || ''}
                                        onProxyUrlChange={(value) => {
                                            onChange('proxyUrl', value);
                                            if (useGlobalProxy && value !== globalProxyUrl) {
                                                setUseGlobalProxy(false);
                                                localStorage.setItem('provider_use_global_proxy', 'false');
                                            }
                                        }}
                                        globalProxyUrl={globalProxyUrl}
                                        useGlobalProxy={useGlobalProxy}
                                        onUseGlobalProxyChange={handleUseGlobalProxyChange}
                                    />

                                    <TextField
                                        size="small"
                                        fullWidth
                                        label={t('providerDialog.advanced.userAgent.label', {defaultValue: 'User-Agent'})}
                                        placeholder={t('providerDialog.advanced.userAgent.placeholder', {defaultValue: 'Leave empty to use built-in default'})}
                                        value={data.userAgent || ''}
                                        onChange={(e) => onChange('userAgent', e.target.value)}
                                        helperText={t('providerDialog.advanced.userAgent.help', {
                                            defaultValue: 'Custom outbound HTTP User-Agent. Empty falls back to the provider\'s built-in UA.',
                                        })}
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
                                </Stack>
                            </AccordionDetails>
                        </Accordion>
                    </Stack>
                </DialogContent>
                <DialogActions sx={{px: 3, pb: 2}}>
                    {onBack && (
                        <Button
                            type="button"
                            variant="text"
                            size="small"
                            startIcon={<ArrowBack fontSize="small"/>}
                            onClick={() => { onClose(); onBack(); }}
                        >
                            Back
                        </Button>
                    )}
                    <Stack direction="row" spacing={1} sx={{ml: 'auto'}}>
                        <Button
                            type="button"
                            variant="outlined"
                            size="small"
                            disabled={!hasAnyProtocol || verifying || submitting}
                            onClick={handleVerify}
                            title="Test connection using available endpoints (optional check)"
                            sx={(theme) => ({
                                '&.Mui-disabled': {
                                    color: theme.palette.mode === 'dark'
                                        ? 'rgba(255, 255, 255, 0.68)'
                                        : theme.palette.text.secondary,
                                },
                            })}
                        >
                            {verifying ? (
                                <CircularProgress size={16} thickness={4}/>
                            ) : (
                                'Test Connection'
                            )}
                        </Button>
                        <Button
                            type="submit"
                            variant="contained"
                            size="small"
                            disabled={!hasAnyProtocol || verifying || submitting}
                            sx={(theme) => ({
                                minWidth: verifying || submitting ? '80px' : 'auto',
                                '&.Mui-disabled': {
                                    color: theme.palette.primary.contrastText,
                                },
                            })}
                        >
                            {submitting ? (
                                <CircularProgress size={20} thickness={4}/>
                            ) : (
                                submitText || defaultSubmitText
                            )}
                        </Button>
                    </Stack>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default ProviderFormDialog;
