import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import VirtualModelsTable from '@/components/VirtualModelsTable';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import { Alert, Snackbar, Typography } from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../services/api';
import type { Provider } from '../types/provider';

const VirtualModelsPage = () => {
    const { t } = useTranslation();
    const [providers, setProviders] = useState<Provider[]>([]);
    const [loading, setLoading] = useState(true);
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setSnackbar({ open: true, message, severity });
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        } else {
            showNotification(`Failed to load providers: ${result.error}`, 'error');
        }
        setLoading(false);
    };

    useEffect(() => {
        loadProviders();
    }, []);

    const vmodelProviders = useMemo(
        () => providers.filter((p: any) => p.auth_type === 'vmodel'),
        [providers]
    );

    const handleToggleProvider = async (uuid: string) => {
        const result = await api.toggleProvider(uuid);
        if (result.success) {
            showNotification(result.message, 'success');
            loadProviders();
        } else {
            showNotification(`Failed to toggle provider: ${result.error}`, 'error');
        }
    };

    return (
        <PageLayout loading={loading}>
            <UnifiedCard
                title={t('layout.virtualModels', { defaultValue: 'Virtual Models' })}
                subtitle={t('layout.virtualModelsTooltip', {
                    defaultValue:
                        'Built-in synthetic model providers for onboarding, demos, and dry-runs.',
                })}
                size="full"
            >
                {vmodelProviders.length > 0 ? (
                    <VirtualModelsTable
                        providers={vmodelProviders}
                        onToggle={handleToggleProvider}
                    />
                ) : (
                    <EmptyStateGuide
                        title="No Virtual Models Available"
                        description="Virtual models are seeded at server startup. Restart the server if this page is empty."
                        showHeroIcon={false}
                    />
                )}
                <Typography
                    variant="caption"
                    sx={{
                        color: "text.secondary",
                        mt: 2,
                        display: 'block'
                    }}>
                    Builtin providers are seeded on every startup; they cannot be deleted, only enabled or disabled here.
                </Typography>
            </UnifiedCard>
            <Snackbar
                open={snackbar.open}
                autoHideDuration={6000}
                onClose={() => setSnackbar((prev) => ({ ...prev, open: false }))}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            >
                <Alert
                    onClose={() => setSnackbar((prev) => ({ ...prev, open: false }))}
                    severity={snackbar.severity}
                    variant="filled"
                    sx={{ width: '100%' }}
                >
                    {snackbar.message}
                </Alert>
            </Snackbar>
        </PageLayout>
    );
};

export default VirtualModelsPage;
