import {ArrowBack, Close, ExpandMore} from '@/components/icons';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    Chip,
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
import ApiKeyField from '@/components/provider-form-dialog/ApiKeyField';
import KeyNameField from '@/components/provider-form-dialog/KeyNameField';
import ProtocolSlot, {type ProtocolSlotData, type ProtocolKind} from '@/components/provider-form-dialog/ProtocolSlot';
import ProxyUrlField from '@/components/provider-form-dialog/ProxyUrlField';
import VerificationResultPanel from '@/components/provider-form-dialog/VerificationResultPanel';
import {type VerificationResult, runProviderProbe} from '@/components/provider-form-dialog/probe';
import ProviderIcon from '@/components/ProviderIcon';
import RegionBadge from '@/components/RegionBadge';
import ProviderExportButton from '@/components/ProviderExportButton';

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
    apiBaseOpenAI?: string;
    apiBaseAnthropic?: string;
    createDualProvider?: boolean;
    /** If set, prefer this exact provider ID when resolving the template.
     *  Avoids mismatches when multiple providers share the same base URL. */
    selectedProviderId?: string;
}

interface PresetProviderFormDialogProps {
    open: boolean;
    onClose: () => void;
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
    onNotification?: (message: string, severity: 'success' | 'error') => void;
}

/**
 * Unified provider form — one layout for all API-key provider creation.
 *
 * Protocols are independent, additive slots (OpenAI / Anthropic). Users can
 * add or remove protocol slots at any time. The "dual" concept is gone —
 * a provider simply has 1 or 2 protocol URLs.
 */
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
    onNotification,
}: PresetProviderFormDialogProps) => {
    const {t} = useTranslation();
    const defaultTitle = mode === 'add' ? t('providerDialog.addTitle') : t('providerDialog.editTitle');
    const defaultSubmitText = mode === 'add' ? t('providerDialog.addButton') : t('common.saveChanges');

    // ── Local state ───────────────────────────────────────────────
    const [verifying, setVerifying] = useState(false);
    const [submitting, setSubmitting] = useState(false);
    const [noApiKey, setNoApiKey] = useState(data.noKeyRequired || false);
    const [verificationResult, setVerificationResult] = useState<VerificationResult | null>(null);
    const [selectedProvider, setSelectedProvider] = useState<UniqueProvider | null>(null);
    const [nameIsAutoFilled, setNameIsAutoFilled] = useState(true);
    const [showNameField, setShowNameField] = useState(mode === 'edit');
    const [useGlobalProxy, setUseGlobalProxy] = useState(false);
    const [globalProxyUrl, setGlobalProxyUrl] = useState('');
    const [advancedOpen, setAdvancedOpen] = useState(false);
    const [baseUrlError, setBaseUrlError] = useState(false);

    // ── Protocol slot state (independent from provider selection) ──
    const [slotOpenAI, setSlotOpenAI] = useState<ProtocolSlotData>({url: '', enabled: true});
    const [slotAnthropic, setSlotAnthropic] = useState<ProtocolSlotData>({url: '', enabled: false});

    const allProviders = useProviderTemplates();

    // Stable onChange ref so effects/handlers don't depend on it.
    const onChangeRef = useRef(onChange);
    useEffect(() => { onChangeRef.current = onChange; });

    // ── Fetch global proxy on mount ───────────────────────────────
    useEffect(() => {
        api.getConfig().then((result) => {
            setGlobalProxyUrl(result?.data?.http_transport?.global_proxy_url ?? '');
        });
    }, []);

    // ── URL → provider suggestions ──────────────────────────────────
    const urlCandidates = useMemo(() => {
        if (selectedProvider) return []; // already selected, no need to suggest
        const activeUrl = slotOpenAI.url.trim() || slotAnthropic.url.trim();
        if (!activeUrl) return [];
        let hostname: string;
        try { hostname = new URL(activeUrl).hostname; } catch { return []; }
        if (!hostname) return [];
        const deduped = new Map<string, UniqueProvider>();
        allProviders.forEach(p => {
            if (deduped.has(p.id)) return;
            const ou = (p.baseUrlOpenAI || '').toLowerCase();
            const au = (p.baseUrlAnthropic || '').toLowerCase();
            try {
                if (new URL(ou).hostname === hostname || new URL(au).hostname === hostname) {
                    deduped.set(p.id, p);
                }
            } catch { /* skip malformed URLs */ }
        });
        return Array.from(deduped.values()).slice(0, 5);
    }, [slotOpenAI.url, slotAnthropic.url, selectedProvider, allProviders]);

    const handleSelectProvider = (provider: UniqueProvider) => {
        setSelectedProvider(provider);
        const nextOpenAI: ProtocolSlotData = {
            url: provider.baseUrlOpenAI || '',
            enabled: !!provider.baseUrlOpenAI,
        };
        const nextAnthropic: ProtocolSlotData = {
            url: provider.baseUrlAnthropic || '',
            enabled: !!provider.baseUrlAnthropic,
        };
        setSlotOpenAI(nextOpenAI);
        setSlotAnthropic(nextAnthropic);
        commitProtocolState(nextOpenAI, nextAnthropic);
        onChangeRef.current('providerBaseUrls', {
            openai: provider.baseUrlOpenAI,
            anthropic: provider.baseUrlAnthropic,
        });
        onChangeRef.current('selectedProviderId', provider.id);
        if (nameIsAutoFilled || !data.name) {
            onChangeRef.current('name', provider.alias || provider.name);
            setNameIsAutoFilled(true);
        }
        setVerificationResult(null);
    };

    // ── Init/reset on open ────────────────────────────────────────
    useEffect(() => {
        if (!open) return;

        console.log('[ProviderFormDialog] open mode=%s, data:', mode, {
            selectedProviderId: data.selectedProviderId,
            apiBase: data.apiBase,
            apiBaseOpenAI: data.apiBaseOpenAI,
            apiBaseAnthropic: data.apiBaseAnthropic,
            apiStyle: data.apiStyle,
            providerBaseUrls: data.providerBaseUrls,
        });

        setVerificationResult(null);
        setBaseUrlError(false);
        setAdvancedOpen(mode === 'edit');
        setShowNameField(mode === 'edit');

        const hasDualOpenAI = !!data.apiBaseOpenAI;
        const hasDualAnthropic = !!data.apiBaseAnthropic;

        // Seed protocol slots from data
        const initOpenAI: ProtocolSlotData = {
            url: data.apiBaseOpenAI || (data.apiStyle !== 'anthropic' ? data.apiBase : ''),
            enabled: hasDualOpenAI || !!data.apiBaseOpenAI || data.apiStyle === 'openai' || (!data.apiStyle && !hasDualAnthropic),
        };
        const initAnthropic: ProtocolSlotData = {
            url: data.apiBaseAnthropic || (data.apiStyle === 'anthropic' ? data.apiBase : ''),
            enabled: hasDualAnthropic || data.apiStyle === 'anthropic',
        };
        setSlotOpenAI(initOpenAI);
        setSlotAnthropic(initAnthropic);

        if (mode === 'edit') {
            // Find ALL presets matching the configured URL(s). When multiple
            // URLs exist, require every URL to match — partial matches don't count.
            const urlMatches = allProviders.filter(p => {
                if (hasDualOpenAI && hasDualAnthropic) {
                    // Both protocols configured — both URLs must match
                    return p.baseUrlOpenAI === data.apiBaseOpenAI &&
                           p.baseUrlAnthropic === data.apiBaseAnthropic;
                }
                if (hasDualOpenAI) {
                    return p.baseUrlOpenAI === data.apiBaseOpenAI;
                }
                if (hasDualAnthropic) {
                    return p.baseUrlAnthropic === data.apiBaseAnthropic;
                }
                // Legacy single apiBase
                return p.baseUrlOpenAI === data.apiBase || p.baseUrlAnthropic === data.apiBase;
            });
            console.log('[ProviderFormDialog] edit mode urlMatches count=%d:', urlMatches.length,
                urlMatches.map(p => ({id: p.id, name: p.name, alias: p.alias})));
            // When there's a selectedProviderId and it's among the urlMatches,
            // treat it as unique (the user previously picked this exact preset).
            const idMatch = data.selectedProviderId
                ? urlMatches.find(p => p.id === data.selectedProviderId)
                : null;
            if (idMatch) {
                console.log('[ProviderFormDialog] edit → idMatch:', {id: idMatch.id, name: idMatch.name, alias: idMatch.alias});
                setSelectedProvider(idMatch);
            } else if (urlMatches.length === 1) {
                console.log('[ProviderFormDialog] edit → unique match:', {id: urlMatches[0].id, name: urlMatches[0].name, alias: urlMatches[0].alias});
                setSelectedProvider(urlMatches[0]);
            } else {
                console.log('[ProviderFormDialog] edit → no auto-select (matches=%d)', urlMatches.length);
                // Multiple matches (or none) — don't auto-select.
                // urlCandidates shows them as clickable chips above the slots.
                setSelectedProvider(null);
            }
        } else if (data.selectedProviderId) {
            // Add mode with a preselected provider from screen 1 — strictly
            // follow the clicked provider, don't recalculate from URLs.
            const provider = allProviders.find(p => p.id === data.selectedProviderId);
            console.log('[ProviderFormDialog] add preselected: lookup id=%s → found=%s',
                data.selectedProviderId, provider ? `${provider.id} / ${provider.name} / ${provider.alias}` : 'NOT FOUND');
            if (provider) {
                setSelectedProvider(provider);
                const nextOpenAI: ProtocolSlotData = {
                    url: provider.baseUrlOpenAI || '',
                    enabled: !!provider.baseUrlOpenAI,
                };
                const nextAnthropic: ProtocolSlotData = {
                    url: provider.baseUrlAnthropic || '',
                    enabled: !!provider.baseUrlAnthropic,
                };
                console.log('[ProviderFormDialog] add preselected → slots:', {
                    openAI: nextOpenAI,
                    anthropic: nextAnthropic,
                });
                setSlotOpenAI(nextOpenAI);
                setSlotAnthropic(nextAnthropic);
                commitProtocolState(nextOpenAI, nextAnthropic);
            }
        } else {
            // Add mode without a preselected provider — try URL matching.
            const matchingProvider = allProviders.find(
                p => (data.providerBaseUrls?.openai && p.baseUrlOpenAI === data.providerBaseUrls.openai) ||
                     (data.providerBaseUrls?.anthropic && p.baseUrlAnthropic === data.providerBaseUrls.anthropic) ||
                     (data.apiBase && (p.baseUrlOpenAI === data.apiBase || p.baseUrlAnthropic === data.apiBase))
            ) || null;
            console.log('[ProviderFormDialog] add url-match:', matchingProvider
                ? `found ${matchingProvider.id} / ${matchingProvider.name}`
                : 'no match');
            setSelectedProvider(matchingProvider);
            if (matchingProvider) {
                // Fill slots from template
                if (matchingProvider.baseUrlOpenAI) {
                    setSlotOpenAI({url: matchingProvider.baseUrlOpenAI, enabled: true});
                }
                if (matchingProvider.baseUrlAnthropic) {
                    setSlotAnthropic({url: matchingProvider.baseUrlAnthropic, enabled: true});
                }
            }
        }

        // "Use global proxy" state (add mode only)
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

    // ── Sync protocol slots to parent form data ───────────────────
    const syncProtocolsToParent = useCallback((openAI: ProtocolSlotData, anthropic: ProtocolSlotData) => {
        const cb = onChangeRef.current;
        const protocols: ('openai' | 'anthropic')[] = [];
        if (openAI.enabled) protocols.push('openai');
        if (anthropic.enabled) protocols.push('anthropic');
        cb('protocols', protocols);
        cb('apiStyle', openAI.enabled ? 'openai' : anthropic.enabled ? 'anthropic' : undefined);
        cb('apiBaseOpenAI', openAI.enabled ? openAI.url : '');
        cb('apiBaseAnthropic', anthropic.enabled ? anthropic.url : '');
        cb('apiBase', openAI.enabled ? openAI.url : anthropic.enabled ? anthropic.url : '');
    }, []);

    // Delegate to parent onChange + sync protocol fields
    const commitProtocolState = useCallback((openAI: ProtocolSlotData, anthropic: ProtocolSlotData) => {
        syncProtocolsToParent(openAI, anthropic);
    }, [syncProtocolsToParent]);

    // ── Slot mutation handlers ────────────────────────────────────
    const updateOpenAIUrl = (url: string) => {
        const next = {...slotOpenAI, url};
        setSlotOpenAI(next);
        setVerificationResult(null);
        if (url.trim()) setBaseUrlError(false);
        if (selectedProvider && url !== selectedProvider.baseUrlOpenAI) {
            setSelectedProvider(null);
        }
    };
    const updateAnthropicUrl = (url: string) => {
        const next = {...slotAnthropic, url};
        setSlotAnthropic(next);
        setVerificationResult(null);
        if (url.trim()) setBaseUrlError(false);
        if (selectedProvider && url !== selectedProvider.baseUrlAnthropic) {
            setSelectedProvider(null);
        }
    };
    const commitOpenAI = () => commitProtocolState(slotOpenAI, slotAnthropic);
    const commitAnthropic = () => commitProtocolState(slotOpenAI, slotAnthropic);

    const toggleSlot = (kind: ProtocolKind) => {
        if (kind === 'anthropic') {
            const next = {...slotAnthropic, enabled: !slotAnthropic.enabled};
            setSlotAnthropic(next);
            commitProtocolState(slotOpenAI, next);
        } else {
            const next = {...slotOpenAI, enabled: !slotOpenAI.enabled};
            setSlotOpenAI(next);
            commitProtocolState(next, slotAnthropic);
        }
        setVerificationResult(null);
    };

    // ── Name helpers ──────────────────────────────────────────────
    const computeAutoName = useCallback((): string => {
        if (selectedProvider) return selectedProvider.alias || selectedProvider.name;
        const raw = slotOpenAI.url || slotAnthropic.url || '';
        try {
            const host = new URL(raw).hostname;
            if (host) return host;
        } catch { /* not a URL */ }
        return t('providerDialog.keyName.fallback', {defaultValue: 'Custom Provider'});
    }, [selectedProvider, slotOpenAI.url, slotAnthropic.url, t]);

    const ensureName = (): string => {
        if (data.name && data.name.trim()) return data.name;
        const auto = computeAutoName();
        onChangeRef.current('name', auto);
        return auto;
    };

    // ── Verification ──────────────────────────────────────────────
    const handleVerify = async () => {
        setVerificationResult(null);

        if (noApiKey) return true;

        const probeMessages = {
            failed: t('providerDialog.verification.failed'),
            networkError: t('providerDialog.verification.networkError'),
        };
        const effectiveName = ensureName();
        const token = data.token;

        if (!effectiveName || !token) {
            setVerificationResult({success: false, message: t('providerDialog.verification.missingFields')});
            return false;
        }

        // Collect enabled protocols to probe
        const probes: Array<{style: 'openai' | 'anthropic'; url: string}> = [];
        if (slotOpenAI.enabled) {
            const u = slotOpenAI.url.trim();
            if (!u) { setBaseUrlError(true); return false; }
            probes.push({style: 'openai', url: u});
        }
        if (slotAnthropic.enabled) {
            const u = slotAnthropic.url.trim();
            if (!u) { setBaseUrlError(true); return false; }
            probes.push({style: 'anthropic', url: u});
        }
        if (probes.length === 0) {
            setVerificationResult({success: false, message: 'At least one protocol must be enabled'});
            return false;
        }

        setVerifying(true);

        if (probes.length === 1) {
            const p = probes[0];
            const result = await runProviderProbe(
                {name: effectiveName, apiStyle: p.style, apiBase: p.url, token, authType: data.authType},
                probeMessages,
            );
            setVerificationResult(result);
            setVerifying(false);
            return result.success;
        }

        // Dual-protocol probe: run both, report per-side
        const results = await Promise.all(
            probes.map(p => runProviderProbe(
                {name: effectiveName, apiStyle: p.style, apiBase: p.url, token, authType: data.authType},
                probeMessages,
            ))
        );
        const success = results.every(r => r.success);
        const sideLine = (label: string, r: VerificationResult) =>
            `${r.success ? '✓' : '✗'} ${label}: ${r.message}`;
        setVerificationResult({
            success,
            message: success
                ? 'Both endpoints verified'
                : results.every(r => !r.success)
                    ? 'Both endpoints failed'
                    : `${results[0].success ? 'Anthropic' : 'OpenAI'} endpoint failed`,
            details: probes.map((p, i) => sideLine(p.style === 'openai' ? 'OpenAI' : 'Anthropic', results[i])).join(' • '),
        });
        setVerifying(false);
        return success;
    };

    // ── Submit ────────────────────────────────────────────────────
    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        // Validate: at least one protocol with a URL
        const hasOpenAI = slotOpenAI.enabled && slotOpenAI.url.trim();
        const hasAnthropic = slotAnthropic.enabled && slotAnthropic.url.trim();
        if (!hasOpenAI && !hasAnthropic) {
            setBaseUrlError(true);
            return;
        }

        // Commit final protocol URLs to parent
        commitProtocolState(slotOpenAI, slotAnthropic);

        const resolved: Partial<EnhancedProviderFormData> = {
            apiBaseOpenAI: slotOpenAI.enabled ? slotOpenAI.url.trim() : '',
            apiBaseAnthropic: slotAnthropic.enabled ? slotAnthropic.url.trim() : '',
            apiBase: slotOpenAI.url.trim() || slotAnthropic.url.trim(),
            apiStyle: slotOpenAI.enabled ? 'openai' : 'anthropic',
            name: ensureName(),
            protocols: (() => {
                const p: ('openai' | 'anthropic')[] = [];
                if (slotOpenAI.enabled) p.push('openai');
                if (slotAnthropic.enabled) p.push('anthropic');
                return p;
            })(),
        };

        setSubmitting(true);
        try {
            await onSubmit(e, resolved);
        } finally {
            setSubmitting(false);
        }
    };

    const handleUseGlobalProxyChange = (checked: boolean) => {
        setUseGlobalProxy(checked);
        localStorage.setItem('provider_use_global_proxy', String(checked));
        if (checked && globalProxyUrl) {
            onChange('proxyUrl', globalProxyUrl);
        } else if (!checked) {
            onChange('proxyUrl', '');
        }
    };

    // ── Derived ───────────────────────────────────────────────────
    const hasAnyProtocol = (slotOpenAI.enabled && slotOpenAI.url.trim()) ||
                           (slotAnthropic.enabled && slotAnthropic.url.trim());

    // Persistent /v1 suffix hint: shown on the OpenAI slot when the user is
    // typing a free-form URL (no provider template) that doesn't already end
    // with /v1. Mirrors the old CustomEndpointField's floating tooltip.
    const persistentV1Hint =
        !selectedProvider &&
        slotOpenAI.enabled &&
        slotOpenAI.url.trim().length > 0 &&
        !(/\/v1\/?$/.test(slotOpenAI.url));
    const handleApplyV1Suffix = () => {
        const base = slotOpenAI.url.replace(/\/+$/, '');
        const newUrl = `${base}/v1`;
        updateOpenAIUrl(newUrl);
        commitProtocolState({...slotOpenAI, url: newUrl}, slotAnthropic);
    };

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
                                    <strong>Getting Started</strong><br/>
                                    Add your first API key to enable AI services. You can add more keys later.
                                </Typography>
                            </Alert>
                        )}

                        {/* ── Provider bar: selected + URL-matched suggestions ── */}
                        {(selectedProvider || urlCandidates.length > 0) && (
                            <Box
                                sx={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: 0.75,
                                    px: 1.5,
                                    py: 1,
                                    borderRadius: 1,
                                    bgcolor: 'action.hover',
                                    flexWrap: 'wrap',
                                }}
                            >
                                {selectedProvider && (
                                    <Chip
                                        icon={<ProviderIcon identifier={selectedProvider.icon || selectedProvider.id} size={16}/>}
                                        label={selectedProvider.alias || selectedProvider.name}
                                        size="small"
                                        variant="outlined"
                                        onClick={() => handleSelectProvider(selectedProvider)}
                                        sx={{cursor: 'pointer'}}
                                    />
                                )}
                                {urlCandidates
                                    .filter(p => p.id !== selectedProvider?.id)
                                    .map(p => (
                                        <Chip
                                            key={p.id}
                                            icon={<ProviderIcon identifier={p.icon || p.id} size={14}/>}
                                            label={p.alias || p.name}
                                            size="small"
                                            variant="outlined"
                                            onClick={() => handleSelectProvider(p)}
                                            sx={{cursor: 'pointer'}}
                                        />
                                    ))}
                            </Box>
                        )}

                        {/* ── Protocol Slots ────────────────────── */}
                        <Box>
                            <Typography variant="caption" fontWeight={600} color="text.secondary" sx={{display: 'block', mb: 1.5}}>
                                {t('providerDialog.protocol.label', {defaultValue: 'Protocols'})}
                            </Typography>
                            <Stack spacing={2}>
                                <ProtocolSlot
                                    kind="openai"
                                    slot={slotOpenAI}
                                    onToggle={() => toggleSlot('openai')}
                                    onUrlChange={updateOpenAIUrl}
                                    onUrlBlur={commitOpenAI}
                                    urlError={baseUrlError && !slotOpenAI.url.trim() && !slotAnthropic.url.trim()}
                                    v1Hint={{show: persistentV1Hint, onApply: handleApplyV1Suffix}}
                                    helperText={selectedProvider
                                        ? (slotOpenAI.enabled
                                            ? t('providerDialog.protocol.helperOpenAI', {defaultValue: 'Supports models from OpenAI, Google and many other OpenAI-compatible providers'})
                                            : undefined)
                                        : undefined}
                                />
                                <ProtocolSlot
                                    kind="anthropic"
                                    slot={slotAnthropic}
                                    onToggle={() => toggleSlot('anthropic')}
                                    onUrlChange={updateAnthropicUrl}
                                    onUrlBlur={commitAnthropic}
                                    urlError={baseUrlError && !slotAnthropic.url.trim() && !slotOpenAI.url.trim()}
                                    helperText={selectedProvider
                                        ? (slotAnthropic.enabled
                                            ? t('providerDialog.protocol.helperAnthropic', {defaultValue: 'For Anthropic-compatible AI providers, commonly used with Claude Code'})
                                            : undefined)
                                        : undefined}
                                />
                            </Stack>
                        </Box>

                        {/* ── API Key ─────────────────────────── */}
                        <ApiKeyField
                            mode={mode}
                            token={data.token}
                            onTokenChange={(value) => { onChange('token', value); setVerificationResult(null); }}
                            noApiKey={noApiKey}
                            optionalEditable={optionalEditableToken}
                            onNoApiKeyChange={(checked) => {
                                setNoApiKey(checked);
                                onChange('noKeyRequired', checked);
                                setVerificationResult(null);
                                if (checked && !optionalEditableToken) onChange('token', '');
                            }}
                        />

                        {/* ── Verification result ─────────────── */}
                        {verificationResult && (
                            <VerificationResultPanel
                                result={verificationResult}
                                onClose={() => setVerificationResult(null)}
                                v1Hint={{
                                    show: !verificationResult.success && slotOpenAI.enabled && !(/\/v1\/?$/.test(slotOpenAI.url)),
                                    onApply: () => {
                                        const base = slotOpenAI.url.replace(/\/+$/, '');
                                        const newUrl = `${base}/v1`;
                                        updateOpenAIUrl(newUrl);
                                        commitProtocolState({...slotOpenAI, url: newUrl}, slotAnthropic);
                                    },
                                }}
                            />
                        )}

                        {/* ── Proxy URL ──────────────────────── */}
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

                        {/* ── Advanced accordion ─────────────── */}
                        <Accordion
                            disableGutters elevation={0}
                            expanded={advancedOpen}
                            onChange={(_, expanded) => setAdvancedOpen(expanded)}
                            sx={{
                                border: 0, borderTop: 1, borderColor: 'divider',
                                '&:before': {display: 'none'}, bgcolor: 'transparent',
                            }}
                        >
                            <AccordionSummary
                                expandIcon={<ExpandMore fontSize="small"/>}
                                sx={{px: 0, minHeight: 40, '& .MuiAccordionSummary-content': {my: 0.5}}}
                            >
                                <Typography variant="body2" color="text.secondary" fontWeight={600}>
                                    {t('providerDialog.advanced.label', {defaultValue: 'Advanced — user-agent, name'})}
                                </Typography>
                            </AccordionSummary>
                            <AccordionDetails sx={{px: 0, pb: 1}}>
                                <Stack spacing={2.5}>
                                    <KeyNameField
                                        showField={showNameField}
                                        onShowField={() => {
                                            if (!data.name) onChangeRef.current('name', computeAutoName());
                                            setShowNameField(true);
                                        }}
                                        name={data.name}
                                        autoName={computeAutoName()}
                                        onNameChange={(value) => {
                                            onChange('name', value);
                                            setNameIsAutoFilled(false);
                                        }}
                                    />
                                    <TextField
                                        size="small" fullWidth
                                        label={t('providerDialog.advanced.userAgent.label', {defaultValue: 'User-Agent'})}
                                        placeholder={t('providerDialog.advanced.userAgent.placeholder', {defaultValue: 'Leave empty to use built-in default'})}
                                        value={data.userAgent || ''}
                                        onChange={(e) => onChange('userAgent', e.target.value)}
                                        helperText={t('providerDialog.advanced.userAgent.help', {defaultValue: 'Custom outbound HTTP User-Agent. Empty falls back to the provider\'s built-in UA.'})}
                                    />
                                    {mode === 'edit' && (
                                        <FormControlLabel
                                            control={<Switch size="small" checked={data.enabled || false} onChange={(e) => onChange('enabled', e.target.checked)}/>}
                                            label={t('providerDialog.enabled')}
                                        />
                                    )}
                                </Stack>
                            </AccordionDetails>
                        </Accordion>
                    </Stack>
                </DialogContent>
                <DialogActions sx={{px: 3, pb: 2}}>
                    {mode === 'edit' && data.uuid && (
                        <Box sx={{mr: 'auto'}}>
                            <ProviderExportButton
                                providerUuid={data.uuid}
                                onNotification={onNotification}
                            />
                        </Box>
                    )}
                    {onBack && (
                        <Button
                            type="button" variant="text" size="small"
                            startIcon={<ArrowBack fontSize="small"/>}
                            onClick={() => { onClose(); onBack(); }}
                        >
                            Back
                        </Button>
                    )}
                    <Stack direction="row" spacing={1} sx={{ml: 'auto'}}>
                        <Button
                            type="button" variant="outlined" size="small"
                            disabled={!hasAnyProtocol || verifying || submitting}
                            onClick={handleVerify}
                            title="Test connection using available endpoints"
                            sx={(theme) => ({
                                '&.Mui-disabled': {
                                    color: theme.palette.mode === 'dark' ? 'rgba(255, 255, 255, 0.68)' : theme.palette.text.secondary,
                                },
                            })}
                        >
                            {verifying ? <CircularProgress size={16} thickness={4}/> : 'Test Connection'}
                        </Button>
                        <Button
                            type="submit" variant="contained" size="small"
                            disabled={!hasAnyProtocol || verifying || submitting}
                            sx={(theme) => ({
                                minWidth: verifying || submitting ? '80px' : 'auto',
                                '&.Mui-disabled': {color: theme.palette.primary.contrastText},
                            })}
                        >
                            {submitting ? <CircularProgress size={20} thickness={4}/> : (submitText || defaultSubmitText)}
                        </Button>
                    </Stack>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default ProviderFormDialog;
