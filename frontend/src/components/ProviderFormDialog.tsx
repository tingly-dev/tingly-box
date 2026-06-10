import {ArrowBack, Close, ExpandMore, InfoOutlined} from '@/components/icons';
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
    Link,
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
    /** When true, the user entered via "Custom endpoint" — hide the provider
     *  dropdown and show a plain URL text field instead. Custom endpoints are
     *  strictly single-protocol. */
    customMode?: boolean;
    /** When true, the user entered via "Fusion endpoint" — show two URL fields
     *  (OpenAI + Anthropic) under one key and always save a single fused record.
     *  No protocol selector, no topology toggle. */
    fusionMode?: boolean;
    /** Edit-mode upgrade: convert a single custom provider into a fusion one. */
    onConvertToFusion?: () => void;
    /** Edit-mode downgrade: convert a fusion provider back to a single endpoint. */
    onConvertToSingle?: () => void;
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
                                customMode = false,
                                fusionMode = false,
                                onConvertToFusion,
                                onConvertToSingle,
                            }: PresetProviderFormDialogProps) => {
    const {t} = useTranslation();
    const defaultTitle = fusionMode
        ? (mode === 'add'
            ? t('providerDialog.fusionForm.title', {defaultValue: 'Add Fusion endpoint'})
            : t('providerDialog.fusionForm.editTitle', {defaultValue: 'Edit Fusion endpoint'}))
        : (mode === 'add' ? t('providerDialog.addTitle') : t('providerDialog.editTitle'));
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
    // Fusion-form local mirrors for the two URL fields (snappy typing; committed
    // to data.apiBaseOpenAI / data.apiBaseAnthropic on blur / submit).
    const [fusOpenAIUrl, setFusOpenAIUrl] = useState('');
    const [fusAnthropicUrl, setFusAnthropicUrl] = useState('');
    // Whether the provider being edited already has both fusion URLs stored.
    const [isExistingFusion, setIsExistingFusion] = useState(false);
    // Snapshot of fusion URLs captured on dialog open, used for downgrade/revert.
    const initialFusionRef = useRef<{ openAI: string; anthropic: string; apiBase: string; apiStyle: string }>({
        openAI: '', anthropic: '', apiBase: '', apiStyle: 'openai',
    });

    const allProviders = useProviderTemplates();

    // Keep onChange in a ref so we can call it from effects/handlers without
    // putting it in dependency arrays (parent passes a fresh function each render).
    const onChangeRef = useRef(onChange);
    useEffect(() => {
        onChangeRef.current = onChange;
    });

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
        // Seed the fusion-form URL mirrors. By convention apiBase carries the
        // OpenAI URL, so fall back to it when apiBaseOpenAI is empty.
        setFusOpenAIUrl(data.apiBaseOpenAI || data.apiBase || '');
        setFusAnthropicUrl(data.apiBaseAnthropic || '');
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
            const existingFusion = hasFusionOpenAI && hasFusionAnthropic;
            setIsExistingFusion(existingFusion);
            // Snapshot for downgrade/revert operations.
            initialFusionRef.current = {
                openAI: data.apiBaseOpenAI || '',
                anthropic: data.apiBaseAnthropic || '',
                apiBase: data.apiBase,
                apiStyle: data.apiStyle || 'openai',
            };
            setCreateFusionProvider(false);
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
            } else if (customMode) {
                // Custom endpoints are OpenAI-compatible in the vast majority of
                // cases — pre-select OpenAI so users aren't stuck with nothing
                // chosen, and let the recommendation hint steer the rare exception.
                setProtocolOpenAI(true);
                setProtocolAnthropic(false);
            } else {
                setProtocolOpenAI(false);
                setProtocolAnthropic(false);
            }
            setIsExistingFusion(false);
            setCreateFusionProvider(false);
            // If the parent prefilled apiBase to a known provider (onboarding
            // browse / paste-detect), seed the Autocomplete with it so users see
            // the picked provider rather than a blank field. In custom mode
            // (including self-hosted) the field is a free-text URL input, so seed
            // the URL itself — never the provider's display name/alias.
            const seedProvider = customMode ? null : matchingProvider;
            setSelectedProvider(seedProvider);
            setProviderInputValue(
                seedProvider
                    ? seedProvider.alias || seedProvider.name
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

                const fusion = createFusionProvider
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
                // Free-form / custom single endpoint — no fusion fields here.
                // (Two-endpoint fusion is configured via the dedicated Fusion form.)
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

    // OAuth-bound providers are issuer-locked to a single protocol.
    // In edit mode, non-fusion providers are also locked — protocol changes
    // require upgrading to fusion (via the fusion toggle) or creating a new entry.
    const fusionLocked = data.authType === 'oauth';
    const protocolLocked = mode === 'edit' && !isExistingFusion;
    const effectiveLocked = fusionLocked || protocolLocked;

    // For an existing fusion provider being edited: deselecting one side
    // downgrades it to a single-protocol provider.
    const handleFusionDowngrade = (nextOpenAI: boolean, nextAnthropic: boolean) => {
        const cb = onChangeRef.current;
        const snap = initialFusionRef.current;
        if (nextOpenAI && !nextAnthropic) {
            cb('apiBase', snap.openAI || snap.apiBase);
            cb('apiStyle', 'openai');
            cb('apiBaseOpenAI', '');
            cb('apiBaseAnthropic', '');
        } else if (!nextOpenAI && nextAnthropic) {
            cb('apiBase', snap.anthropic);
            cb('apiStyle', 'anthropic');
            cb('apiBaseOpenAI', '');
            cb('apiBaseAnthropic', '');
        }
        // Disallowing both-false: ignore the toggle if it would leave no protocol.
    };

    // Custom endpoints are strictly single-protocol (radio semantics):
    // selecting one clears the other; the active option can't be unselected.
    const selectCustomProtocol = (openai: boolean) => {
        setProtocolOpenAI(openai);
        setProtocolAnthropic(!openai);
        setVerificationResult(null);
        syncProtocolsToParent(openai, !openai, selectedProvider);
    };

    const toggleOpenAIProtocol = () => {
        if (effectiveLocked) return;
        if (selectedProvider && !selectedProvider.supportsOpenAI) return;
        if (customMode) { if (!protocolOpenAI) selectCustomProtocol(true); return; }
        const next = !protocolOpenAI;
        if (isExistingFusion && !next && !protocolAnthropic) return;
        setProtocolOpenAI(next);
        setVerificationResult(null);
        if (isExistingFusion) handleFusionDowngrade(next, protocolAnthropic);
        else syncProtocolsToParent(next, protocolAnthropic, selectedProvider);
    };

    const toggleAnthropicProtocol = () => {
        if (effectiveLocked) return;
        if (selectedProvider && !selectedProvider.supportsAnthropic) return;
        if (customMode) { if (!protocolAnthropic) selectCustomProtocol(false); return; }
        const next = !protocolAnthropic;
        if (isExistingFusion && !next && !protocolOpenAI) return;
        setProtocolAnthropic(next);
        setVerificationResult(null);
        if (isExistingFusion) handleFusionDowngrade(protocolOpenAI, next);
        else syncProtocolsToParent(protocolOpenAI, next, selectedProvider);
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
        const raw = (fusionMode ? fusOpenAIUrl : data.apiBase) || providerInputValue || '';
        try {
            const host = new URL(raw).hostname;
            if (host) return host;
        } catch { /* not a URL */ }
        return t('providerDialog.keyName.fallback', {defaultValue: 'Custom Provider'});
    }, [selectedProvider, fusionMode, fusOpenAIUrl, data.apiBase, providerInputValue, t]);

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

        const probeMessages = {
            failed: t('providerDialog.verification.failed'),
            networkError: t('providerDialog.verification.networkError'),
        };

        // Fusion form: a fused provider answers BOTH protocols, so verifying
        // only one URL would leave the other (often the just-typed one) untested.
        // Probe both endpoints and report per-side results.
        if (fusionMode) {
            const openai = fusOpenAIUrl.trim();
            const anthropic = fusAnthropicUrl.trim();
            const effectiveName = ensureName();
            if (!effectiveName || !openai || !anthropic || !data.token) {
                setVerificationResult({
                    success: false,
                    message: t('providerDialog.verification.missingFields'),
                });
                return false;
            }
            setVerifying(true);
            setVerificationResult(null);
            const [oRes, aRes] = await Promise.all([
                runProviderProbe(
                    {name: effectiveName, apiStyle: 'openai', apiBase: openai, token: data.token, authType: data.authType},
                    probeMessages,
                ),
                runProviderProbe(
                    {name: effectiveName, apiStyle: 'anthropic', apiBase: anthropic, token: data.token, authType: data.authType},
                    probeMessages,
                ),
            ]);
            const success = oRes.success && aRes.success;
            const sideLine = (label: string, r: VerificationResult) =>
                `${r.success ? '✓' : '✗'} ${label}: ${r.message}`;
            setVerificationResult({
                success,
                message: success
                    ? t('providerDialog.fusionForm.verifyBothOk', {defaultValue: 'Both endpoints verified'})
                    : !oRes.success && !aRes.success
                        ? t('providerDialog.fusionForm.verifyBothFailed', {defaultValue: 'Both endpoints failed'})
                        : t('providerDialog.fusionForm.verifyOneFailed', {
                            defaultValue: '{{side}} endpoint failed',
                            side: oRes.success ? 'Anthropic' : 'OpenAI',
                        }),
                details: [sideLine('OpenAI', oRes), sideLine('Anthropic', aRes)].join(' • '),
            });
            setVerifying(false);
            return success;
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
            probeMessages,
        );
        setVerificationResult(result);
        setVerifying(false);
        return result.success;
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        if (fusionMode) {
            // Fusion form: two URLs, one key, always a single fused record.
            const openai = fusOpenAIUrl.trim();
            const anthropic = fusAnthropicUrl.trim();
            if (!openai || !anthropic) {
                setBaseUrlError(true);
                return;
            }
            const resolved: Partial<EnhancedProviderFormData> = {
                apiBaseOpenAI: openai,
                apiBaseAnthropic: anthropic,
                apiBase: openai,
                apiStyle: 'openai',
                createFusionProvider: true,
                name: ensureName(),
            };
            (resolved as any).protocols = ['openai', 'anthropic'];
            setSubmitting(true);
            try {
                await onSubmit(e, resolved);
            } finally {
                setSubmitting(false);
            }
            return;
        }
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

    // Fusion form implies both protocols; its own URL validation gates submit.
    const hasAnyProtocol = fusionMode || protocolOpenAI || protocolAnthropic;

    // Persistent /v1 suffix hint: shown in custom mode when an OpenAI
    // protocol is selected and the user has typed a URL that doesn't already
    // end with /v1. Stays hidden while the field is empty — nothing to append to,
    // and popping a hint over a blank input is just noise.
    const currentUrl = data.apiBase || providerInputValue;
    const urlAlreadyHasV1 = /\/v1\/?$/.test(currentUrl);
    const showV1Hint = customMode && protocolOpenAI && currentUrl.trim().length > 0 && !urlAlreadyHasV1;
    const applyV1Suffix = () => {
        const base = currentUrl.replace(/\/+$/, '');
        const newUrl = `${base}/v1`;
        setProviderInputValue(newUrl);
        onChangeRef.current('apiBase', newUrl);
        setVerificationResult(null);
    };

    // When both protocols are checked on a template that exposes two base URLs,
    // the outcome ("merge into one" vs "create two") is otherwise invisible.
    // Surface it as a one-line hint that tracks the fusion toggle.
    const hasBothBaseUrls = !!selectedProvider?.baseUrlOpenAI && !!selectedProvider?.baseUrlAnthropic;
    // add mode: shown when both protocols are selected
    // edit mode: shown for non-fusion providers when the template supports both sides (upgrade path)
    // Fusion toggle / topology hint are template-only concerns (presets keep the
    // split-vs-merge choice). Custom and fusion paths never show them.
    const showFusionToggle = !customMode && !fusionMode && (mode === 'add'
        ? (protocolOpenAI && protocolAnthropic)
        : (!isExistingFusion && hasBothBaseUrls));
    const showTopologyHint = !customMode && !fusionMode && protocolOpenAI && protocolAnthropic && hasBothBaseUrls;
    const willMergeBaseUrls = createFusionProvider;

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

                        {fusionMode ? (
                          <Stack spacing={2}>
                            <TextField
                                size="small"
                                fullWidth
                                label={t('providerDialog.customFusion.openAILabel')}
                                placeholder={t('providerDialog.provider.customPlaceholder', {defaultValue: 'https://api.example.com/v1'})}
                                value={fusOpenAIUrl}
                                onChange={(e) => {
                                    setFusOpenAIUrl(e.target.value);
                                    if (e.target.value.trim()) setBaseUrlError(false);
                                    setVerificationResult(null);
                                }}
                                onBlur={() => onChangeRef.current('apiBaseOpenAI', fusOpenAIUrl)}
                                required
                                error={baseUrlError && !fusOpenAIUrl.trim()}
                            />
                            <TextField
                                size="small"
                                fullWidth
                                label={t('providerDialog.customFusion.anthropicLabel')}
                                placeholder={t('providerDialog.fusionForm.anthropicPlaceholder', {defaultValue: 'https://api.example.com/anthropic'})}
                                value={fusAnthropicUrl}
                                onChange={(e) => {
                                    setFusAnthropicUrl(e.target.value);
                                    if (e.target.value.trim()) setBaseUrlError(false);
                                    setVerificationResult(null);
                                }}
                                onBlur={() => onChangeRef.current('apiBaseAnthropic', fusAnthropicUrl)}
                                required
                                error={baseUrlError && !fusAnthropicUrl.trim()}
                                helperText={t('providerDialog.fusionForm.help', {defaultValue: 'Both protocols share the API key below. Inbound requests are routed to the matching endpoint.'})}
                            />
                            {mode === 'edit' && onConvertToSingle && (
                                <Link
                                    component="button"
                                    type="button"
                                    variant="caption"
                                    underline="hover"
                                    sx={{alignSelf: 'flex-start'}}
                                    onClick={onConvertToSingle}
                                >
                                    {t('providerDialog.fusionForm.convertToSingle', {defaultValue: 'Convert to a single endpoint'})}
                                </Link>
                            )}
                          </Stack>
                        ) : customMode ? (
                            <Tooltip
                                open={showV1Hint}
                                title={
                                    <Stack direction="row" alignItems="center" spacing={0.75}>
                                        <Typography variant="body2" color="text.secondary">
                                            {t('providerDialog.v1Hint.message', {
                                                defaultValue: 'Most OpenAI-compatible APIs need a /v1 suffix.',
                                            })}
                                        </Typography>
                                        <Link
                                            component="button"
                                            type="button"
                                            variant="body2"
                                            onClick={applyV1Suffix}
                                            underline="always"
                                            sx={{
                                                fontWeight: 600,
                                                whiteSpace: 'nowrap',
                                            }}
                                        >
                                            {t('providerDialog.v1Hint.apply', {defaultValue: 'Append /v1'})}
                                        </Link>
                                    </Stack>
                                }
                                placement="top"
                                arrow
                                disableFocusListener
                                disableHoverListener
                                disableTouchListener
                                slotProps={{
                                    tooltip: {
                                        sx: {
                                            bgcolor: 'background.paper',
                                            color: 'text.primary',
                                            border: 1,
                                            borderColor: 'divider',
                                            boxShadow: 2,
                                            px: 1.5,
                                            py: 1,
                                        },
                                    },
                                    arrow: {
                                        sx: {
                                            fontSize: 16,
                                            color: 'background.paper',
                                            '&::before': {
                                                border: 1,
                                                borderColor: 'divider',
                                            },
                                        },
                                    },
                                }}
                            >
                                <TextField
                                    size="small"
                                    fullWidth
                                    label={t('providerDialog.provider.label')}
                                    placeholder={t('providerDialog.provider.customPlaceholder', {defaultValue: 'https://api.example.com/v1'})}
                                    value={providerInputValue}
                                    onChange={(e) => {
                                        const val = e.target.value;
                                        setProviderInputValue(val);
                                        if (val.trim()) setBaseUrlError(false);
                                    }}
                                    onBlur={() => {
                                        if (data.apiBase !== providerInputValue) {
                                            onChangeRef.current('apiBase', providerInputValue);
                                            onChangeRef.current('providerBaseUrls', undefined);
                                        }
                                    }}
                                    required
                                    error={baseUrlError}
                                    helperText={baseUrlError ? t('providerDialog.provider.required', {defaultValue: 'Base URL is required'}) : undefined}
                                />
                            </Tooltip>
                        ) : (
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
                        )}

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

                        {!fusionMode && (
                            <ProtocolSelector
                                selectedProvider={selectedProvider}
                                protocolOpenAI={protocolOpenAI}
                                protocolAnthropic={protocolAnthropic}
                                fusionLocked={effectiveLocked}
                                singleSelect={customMode}
                                openAICapabilities={openAICapabilities}
                                onToggleOpenAI={toggleOpenAIProtocol}
                                onToggleAnthropic={toggleAnthropicProtocol}
                                recommendOpenAI={customMode}
                            />
                        )}

                        {/* Edit-mode upgrade: turn a single endpoint into a fusion one.
                            Suppressed when the template-driven FusionToggle is visible —
                            one upgrade affordance per dialog, never two. */}
                        {mode === 'edit' && !fusionMode && !showFusionToggle && data.authType !== 'oauth' && onConvertToFusion && (
                            <Link
                                component="button"
                                type="button"
                                variant="caption"
                                underline="hover"
                                sx={{alignSelf: 'flex-start'}}
                                onClick={onConvertToFusion}
                            >
                                {t('providerDialog.providerSingle.convertToFusion', {defaultValue: 'Add an Anthropic endpoint (make it a Fusion provider)'})}
                            </Link>
                        )}

                        {showFusionToggle && (
                            <FusionToggle
                                checked={createFusionProvider}
                                onChange={(checked) => {
                                    setCreateFusionProvider(checked);
                                    onChange('createFusionProvider', checked);
                                    if (mode === 'edit' && selectedProvider) {
                                        if (checked) {
                                            // Upgrade: auto-select both protocols and populate URLs from template.
                                            setProtocolOpenAI(true);
                                            setProtocolAnthropic(true);
                                            onChange('apiBaseOpenAI', selectedProvider.baseUrlOpenAI || '');
                                            onChange('apiBaseAnthropic', selectedProvider.baseUrlAnthropic || '');
                                            onChange('apiBase', selectedProvider.baseUrlOpenAI || data.apiBase);
                                            onChange('apiStyle', 'openai');
                                        } else {
                                            // Revert: restore original single-protocol state.
                                            const snap = initialFusionRef.current;
                                            setProtocolOpenAI(snap.apiStyle === 'openai');
                                            setProtocolAnthropic(snap.apiStyle === 'anthropic');
                                            onChange('apiBaseOpenAI', '');
                                            onChange('apiBaseAnthropic', '');
                                            onChange('apiBase', snap.apiBase);
                                            onChange('apiStyle', snap.apiStyle as 'openai' | 'anthropic');
                                        }
                                    } else {
                                        syncProtocolsToParent(protocolOpenAI, protocolAnthropic, selectedProvider);
                                    }
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
                                v1Hint={{
                                    show: !verificationResult.success
                                        && protocolOpenAI
                                        && !(/\/v1\/?$/.test(data.apiBase || providerInputValue)),
                                    onApply: () => {
                                        const base = (data.apiBase || providerInputValue).replace(/\/+$/, '');
                                        const newUrl = `${base}/v1`;
                                        setProviderInputValue(newUrl);
                                        onChangeRef.current('apiBase', newUrl);
                                        setVerificationResult(null);
                                    },
                                }}
                            />
                        )}

                        {/* Proxy URL — exposed at top level since it's a common requirement */}
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

                        {/* Advanced accordion — user-agent, name, enabled */}
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
                                    {t('providerDialog.advanced.label', {defaultValue: 'Advanced — user-agent, name'})}
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
