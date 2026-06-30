import { useState, useCallback } from 'react';
import { api } from '../services/api';
import type { EnhancedProviderFormData } from '@/components/ProviderFormDialog';
import type { ConnectSelection } from '@/components/ConnectProviderDialog';

interface UseProviderDialogOptions {
    defaultApiStyle?: 'openai' | 'anthropic' | undefined;
    onProviderAdded?: () => void;
    onImport?: () => void;
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

const emptyForm = (apiStyle?: 'openai' | 'anthropic'): EnhancedProviderFormData => ({
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
 * Returns null when the picker result doesn't open the form (oauth / import).
 */
export function buildProviderFormData(selection: ConnectSelection): ConnectFormResult | null {
    if (selection.kind === 'oauth' || selection.kind === 'import') {
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
            userAgent: '',
            providerBaseUrls: { openai: p.baseUrlOpenAI, anthropic: p.baseUrlAnthropic },
            selectedProviderId: p.id,
        },
        optionalEditableToken: isLocal,
    };
}

interface UseProviderDialogReturn {
    providerDialogOpen: boolean;
    providerFormData: EnhancedProviderFormData;
    /** @deprecated Use handleConnectAIClick to open the picker instead. */
    handleAddProviderClick: () => void;
    handleProviderSubmit: (e: React.FormEvent, resolved?: Partial<EnhancedProviderFormData>) => Promise<void>;
    handleProviderForceAdd: () => Promise<void>;
    handleCloseDialog: () => void;
    handleFieldChange: (field: keyof EnhancedProviderFormData, value: any) => void;
    connectDialogOpen: boolean;
    handleConnectAIClick: () => void;
    handleConnectSelect: (selection: ConnectSelection) => void;
    handleCloseConnect: () => void;
    customMode: boolean;
    dualMode: boolean;
    fromConnectPicker: boolean;
}

export const useProviderDialog = (
    showNotification: (message: string, severity: 'success' | 'error') => void,
    options: UseProviderDialogOptions = {}
): UseProviderDialogReturn => {
    const { defaultApiStyle, onProviderAdded, onImport } = options;

    const [providerDialogOpen, setProviderDialogOpen] = useState(false);
    const [connectDialogOpen, setConnectDialogOpen] = useState(false);
    const [customMode, setCustomMode] = useState(false);
    const [dualMode, setDualMode] = useState(false);
    const [fromConnectPicker, setFromConnectPicker] = useState(false);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>(emptyForm(defaultApiStyle));

    const handleAddProviderClick = () => {
        setProviderFormData(emptyForm(defaultApiStyle));
        setCustomMode(false);
        setDualMode(false);
        setFromConnectPicker(false);
        setProviderDialogOpen(true);
    };

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
            // oauth / import — handled by caller via other dialogs
            if (selection.kind === 'import') onImport?.();
            return;
        }

        setCustomMode(selection.kind === 'custom');
        setProviderFormData(built.formData);
        setProviderDialogOpen(true);
    }, [defaultApiStyle, onImport]);

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
        setCustomMode(false);
        setDualMode(false);
        setFromConnectPicker(false);
    };

    const handleFieldChange = (field: keyof EnhancedProviderFormData, value: any) => {
        setProviderFormData(prev => ({ ...prev, [field]: value }));
    };

    return {
        providerDialogOpen,
        providerFormData,
        handleAddProviderClick,
        handleProviderSubmit,
        handleProviderForceAdd,
        handleCloseDialog,
        handleFieldChange,
        connectDialogOpen,
        handleConnectAIClick,
        handleConnectSelect,
        handleCloseConnect,
        customMode,
        dualMode,
        fromConnectPicker,
    };
};
