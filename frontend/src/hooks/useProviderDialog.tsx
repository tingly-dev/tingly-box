import { useState } from 'react';
import { api } from '../services/api';
import type { EnhancedProviderFormData } from '@/components/ProviderFormDialog';

interface UseProviderDialogOptions {
    defaultApiStyle?: 'openai' | 'anthropic' | undefined;
    onProviderAdded?: () => void;
}

interface UseProviderDialogReturn {
    providerDialogOpen: boolean;
    providerFormData: EnhancedProviderFormData;
    handleAddProviderClick: () => void;
    handleProviderSubmit: (e: React.FormEvent, resolved?: Partial<EnhancedProviderFormData>) => Promise<void>;
    handleProviderForceAdd: () => Promise<void>;
    handleCloseDialog: () => void;
    handleFieldChange: (field: keyof EnhancedProviderFormData, value: any) => void;
}

export const useProviderDialog = (
    showNotification: (message: string, severity: 'success' | 'error') => void,
    options: UseProviderDialogOptions = {}
): UseProviderDialogReturn => {
    const { defaultApiStyle, onProviderAdded } = options;

    const [providerDialogOpen, setProviderDialogOpen] = useState(false);
    const [providerFormData, setProviderFormData] = useState<EnhancedProviderFormData>({
        name: '',
        apiBase: '',
        apiStyle: defaultApiStyle || undefined,
        token: '',
        enabled: true,
        noKeyRequired: false,
        proxyUrl: '',
    });

    const handleAddProviderClick = () => {
        setProviderFormData({
            name: '',
            apiBase: '',
            apiStyle: defaultApiStyle || undefined,
            token: '',
            enabled: true,
            noKeyRequired: false,
            proxyUrl: '',
        });
        setProviderDialogOpen(true);
    };

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
            showNotification('API Key added successfully!', 'success');
            setProviderDialogOpen(false);
            onProviderAdded?.();
        } else {
            showNotification(`Failed to add API Key: ${result.error}`, 'error');
        }
    };

    // Handle force-add: skip probe and submit directly
    const handleProviderForceAdd = async () => {
        console.log('Force add called with data:', providerFormData);

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

        console.log('Calling api.addProvider with force=true:', providerData);
        const result = await api.addProvider(providerData, true);
        console.log('addProvider result:', result);

        if (result.success) {
            showNotification('API Key added successfully!', 'success');
            setProviderDialogOpen(false);
            onProviderAdded?.();
        } else {
            console.error('Force add failed:', result);
            showNotification(`Failed to add API Key: ${result.error}`, 'error');
        }
    };

    const handleCloseDialog = () => {
        setProviderDialogOpen(false);
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
    };
};
