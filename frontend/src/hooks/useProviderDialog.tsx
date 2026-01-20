import { useState } from 'react';
import { api } from '../services/api';
import type { EnhancedProviderFormData } from '../components/ProviderFormDialog';

interface UseProviderDialogOptions {
    defaultApiStyle?: 'openai' | 'anthropic' | undefined;
    onProviderAdded?: () => void;
}

interface UseProviderDialogReturn {
    providerDialogOpen: boolean;
    providerFormData: EnhancedProviderFormData;
    handleAddProviderClick: () => void;
    handleProviderSubmit: (e: React.FormEvent) => Promise<void>;
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

    const handleProviderSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
            no_key_required: providerFormData.noKeyRequired,
            proxy_url: providerFormData.proxyUrl,
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
        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
            no_key_required: providerFormData.noKeyRequired,
            proxy_url: providerFormData.proxyUrl,
        };

        const result = await api.addProvider(providerData, true);

        if (result.success) {
            showNotification('API Key added successfully!', 'success');
            setProviderDialogOpen(false);
            onProviderAdded?.();
        } else {
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
