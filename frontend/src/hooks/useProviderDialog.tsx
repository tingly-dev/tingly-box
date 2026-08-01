import { useState, useCallback } from 'react';
import { api } from '../services/api';
import type { EnhancedProviderFormData } from '@/components/ProviderFormDialog';
import type { ConnectSelection } from '@/components/ConnectProviderDialog';

interface UseProviderDialogOptions {
    defaultApiStyle?: 'openai' | 'anthropic' | undefined;
    onProviderAdded?: () => void;
}

// ── Shared: ConnectSelection → form data ────────────────────────────
// Single source of truth for all entry points. Every page that opens the
// provider form from the "Connect AI" picker must go through this function.

export interface ConnectFormResult {
    /** Ready-to-use form data for the ProviderFormDialog. */
    formData: EnhancedProviderFormData;
    /** Self-hosted / local providers: token is optional but editable. */
    optionalEditableToken: boolean;
}

export const emptyForm = (apiStyle?: 'openai' | 'anthropic'): EnhancedProviderFormData => ({
    name: '',
    apiBase: '',
    apiStyle: apiStyle || undefined,
    token: '',
    enabled: true,
    noKeyRequired: false,
    proxyUrl: '',
});

/**
 * Convert a picker selection into provider form data.
 * Returns null when the picker result doesn't open the form (oauth / import / paste).
 */
export function buildProviderFormData(selection: ConnectSelection): ConnectFormResult | null {
    if (selection.kind === 'oauth' || selection.kind === 'import' || selection.kind === 'paste') {
        return null;
    }

    if (selection.kind === 'custom') {
        return { formData: emptyForm(), optionalEditableToken: false };
    }

    const p = selection.provider;
    const isLocal = selection.kind === 'local';

    return {
        formData: {
            name: p.alias || p.name,
            apiBase: isLocal
                ? (p as any).url || p.baseUrlOpenAI || p.baseUrlAnthropic || ''
                : p.baseUrlOpenAI || p.baseUrlAnthropic || '',
            apiStyle: isLocal ? 'openai' : undefined,
            token: isLocal ? ((p as any).defaultApiKey ?? '') : '',
            enabled: true,
            noKeyRequired: isLocal ? !((p as any).defaultApiKey) : false,
            proxyUrl: '',
            providerBaseUrls: { openai: p.baseUrlOpenAI, anthropic: p.baseUrlAnthropic },
            selectedProviderId: p.id,
        },
        optionalEditableToken: isLocal,
    };
}

export interface UseProviderDialogReturn {
    providerDialogOpen: boolean;
    providerFormData: EnhancedProviderFormData;
    handleProviderSubmit: (e: React.FormEvent, resolved?: Partial<EnhancedProviderFormData>) => Promise<void>;
    handleProviderForceAdd: () => Promise<void>;
    handleCloseDialog: () => void;
    handleFieldChange: (field: keyof EnhancedProviderFormData, value: any) => void;
    connectDialogOpen: boolean;
    handleConnectAIClick: () => void;
    handleConnectSelect: (selection: ConnectSelection) => void;
    handleCloseConnect: () => void;
    /** Apply a paste-detected prefill and open the provider form. */
    handlePastePick: (prefill: EnhancedProviderFormData) => void;
    /** Open state for the built-in PasteDetectDialog (owned by this hook). */
    pasteDialogOpen: boolean;
    handleClosePasteDialog: () => void;
    /** Open state for the built-in ImportModal (owned by this hook). */
    importModalOpen: boolean;
    importing: boolean;
    handleImportClick: () => void;
    handleCloseImport: () => void;
    handleImportData: (data: string) => Promise<void>;
    /** Open state for the built-in add-flow OAuthDialog (owned by this hook). */
    oauthDialogOpen: boolean;
    oauthAutoStartId: string | null;
    handleCloseOAuth: () => void;
    handleOAuthSuccess: () => void;
    fromConnectPicker: boolean;
    /** Self-hosted / local providers: token is optional but editable. */
    optionalEditableToken: boolean;
}

// Owns the complete "Connect AI" add flow: the picker plus every downstream
// dialog it can route to (API-key form, OAuth sign-in, paste & detect,
// import). Render the dialogs with <ConnectAIDialogs flow={...}/> so every
// surface responds to every picker card the same way.
export const useProviderDialog = (
    showNotification: (message: string, severity: 'success' | 'error') => void,
    options: UseProviderDialogOptions = {}
): UseProviderDialogReturn => {
    const { defaultApiStyle, onProviderAdded } = options;

    const [providerDialogOpen, setProviderDialogOpen] = useState(false);
    const [connectDialogOpen, setConnectDialogOpen] = useState(false);
    const [pasteDialogOpen, setPasteDialogOpen] = useState(false);
    const [importModalOpen, setImportModalOpen] = useState(false);
    const [importing, setImporting] = useState(false);
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthAutoStartId, setOAuthAutoStartId] = useState<string | null>(null);
    const [fromConnectPicker, setFromConnectPicker] = useState(false);
    const [optionalEditableToken, setOptionalEditableToken] = useState(false);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>(emptyForm(defaultApiStyle));

    const handleConnectAIClick = useCallback(() => {
        setConnectDialogOpen(true);
    }, []);

    const handleCloseConnect = useCallback(() => {
        setConnectDialogOpen(false);
    }, []);

    const handleConnectSelect = useCallback((selection: ConnectSelection) => {
        setConnectDialogOpen(false);
        setFromConnectPicker(true);

        const built = buildProviderFormData(selection);
        if (!built) {
            // Non-form routes — all owned here so every consumer gets them.
            if (selection.kind === 'oauth') {
                setOAuthAutoStartId(selection.providerId);
                setOAuthDialogOpen(true);
            }
            if (selection.kind === 'import') setImportModalOpen(true);
            if (selection.kind === 'paste') setPasteDialogOpen(true);
            return;
        }

        setProviderFormData(built.formData);
        setOptionalEditableToken(built.optionalEditableToken);
        setProviderDialogOpen(true);
    }, []);

    // Paste-detected prefill: apply it and open the form (token always required
    // here — paste produces an explicit value or the user chose manual fill).
    const handlePastePick = useCallback((prefill: EnhancedProviderFormData) => {
        setPasteDialogOpen(false);
        setFromConnectPicker(true);
        setOptionalEditableToken(false);
        setProviderFormData(prefill);
        setProviderDialogOpen(true);
    }, []);

    const handleClosePasteDialog = useCallback(() => {
        setPasteDialogOpen(false);
    }, []);

    const handleImportClick = useCallback(() => {
        setImportModalOpen(true);
    }, []);

    const handleCloseImport = useCallback(() => {
        setImportModalOpen(false);
    }, []);

    const handleImportData = useCallback(async (data: string) => {
        setImporting(true);
        try {
            const result = await api.importProvider(data);
            if (result.success) {
                const created = result.data?.providers_created || 0;
                const used = result.data?.providers_used || 0;
                let message = 'Provider import completed';
                if (created > 0) message += `. ${created} new provider${created > 1 ? 's' : ''} created`;
                if (used > 0) message += `. ${used} existing provider${used > 1 ? 's' : ''} referenced`;
                if (created === 0 && used === 0) message = 'No providers found in import data';
                showNotification(message, 'success');
                setImportModalOpen(false);
                onProviderAdded?.();
            } else {
                showNotification(`Import failed: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch (err: any) {
            showNotification(`Import failed: ${err?.message || 'Unknown error'}`, 'error');
        } finally {
            setImporting(false);
        }
    }, [showNotification, onProviderAdded]);

    const handleCloseOAuth = useCallback(() => {
        setOAuthDialogOpen(false);
        setOAuthAutoStartId(null);
    }, []);

    const handleOAuthSuccess = useCallback(() => {
        setOAuthDialogOpen(false);
        setOAuthAutoStartId(null);
        showNotification('Provider connected via OAuth!', 'success');
        onProviderAdded?.();
    }, [showNotification, onProviderAdded]);

    const handleProviderSubmit = async (e: React.FormEvent, resolved?: Partial<EnhancedProviderFormData>) => {
        e.preventDefault();

        // Merge dialog-resolved fields over form state; they arrive via async
        // onChange and may not be in state yet at submit time.
        const fd = { ...providerFormData, ...(resolved || {}) };
        const providerData = {
            name: fd.name,
            api_base: fd.apiBase,
            api_style: fd.apiStyle,
            api_base_openai: fd.apiBaseOpenAI || undefined,
            api_base_anthropic: fd.apiBaseAnthropic || undefined,
            token: fd.token,
            no_key_required: fd.noKeyRequired,
            proxy_url: fd.proxyUrl,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            showNotification('Provider connected successfully!', 'success');
            setProviderDialogOpen(false);
            onProviderAdded?.();
        } else {
            showNotification(`Failed to connect provider: ${result.error}`, 'error');
        }
    };

    // Handle force-add: skip probe and submit directly
    const handleProviderForceAdd = async () => {
        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            api_base_openai: providerFormData.apiBaseOpenAI || undefined,
            api_base_anthropic: providerFormData.apiBaseAnthropic || undefined,
            token: providerFormData.token,
            no_key_required: providerFormData.noKeyRequired,
            proxy_url: providerFormData.proxyUrl,
        };

        const result = await api.addProvider(providerData, true);

        if (result.success) {
            showNotification('Provider connected successfully!', 'success');
            setProviderDialogOpen(false);
            onProviderAdded?.();
        } else {
            console.error('Force add failed:', result);
            showNotification(`Failed to connect provider: ${result.error}`, 'error');
        }
    };

    const handleCloseDialog = () => {
        setProviderDialogOpen(false);
        setFromConnectPicker(false);
        setOptionalEditableToken(false);
    };

    const handleFieldChange = (field: keyof EnhancedProviderFormData, value: any) => {
        setProviderFormData(prev => ({ ...prev, [field]: value }));
    };

    return {
        providerDialogOpen,
        providerFormData,
        handleProviderSubmit,
        handleProviderForceAdd,
        handleCloseDialog,
        handleFieldChange,
        connectDialogOpen,
        handleConnectAIClick,
        handleConnectSelect,
        handleCloseConnect,
        handlePastePick,
        pasteDialogOpen,
        handleClosePasteDialog,
        importModalOpen,
        importing,
        handleImportClick,
        handleCloseImport,
        handleImportData,
        oauthDialogOpen,
        oauthAutoStartId,
        handleCloseOAuth,
        handleOAuthSuccess,
        fromConnectPicker,
        optionalEditableToken,
    };
};
