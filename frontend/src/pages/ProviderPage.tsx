import { Add } from '@mui/icons-material';
import { Alert, Box, Button, Snackbar, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ProviderTable from '../components/ProviderTable.tsx';
import { PageLayout } from '../components/PageLayout';
import PresetProviderFormDialog from '../components/PresetProviderFormDialog.tsx';
import { type ProviderFormData } from '../components/ProviderFormDialog.tsx';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';

const ProviderPage = () => {
    const { t } = useTranslation();
    const [providers, setProviders] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const [snackbar, setSnackbar] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    // Dialog state
    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>('add');
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>({
        uuid: undefined,
        name: '',
        apiBase: '',
        apiStyle: undefined,
        token: '',
        enabled: true,
    });

    useEffect(() => {
        loadProviders();
    }, []);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setSnackbar({ open: true, message, severity });
    };

    const handleAddProviderClick = () => {
        setDialogMode('add');
        setProviderFormData({
            uuid: undefined,
            name: '',
            apiBase: '',
            apiStyle: undefined,
            token: '',
            enabled: true,
        });
        setDialogOpen(true);
    };

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        } else {
            showNotification(t('provider.notifications.loadFailed', { error: result.error }), 'error');
        }
        setLoading(false);
    };

    const handleProviderSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
            ...(dialogMode === 'edit' && { enabled: providerFormData.enabled }),
        };

        const result = dialogMode === 'add'
            ? await api.addProvider(providerData)
            : await api.updateProvider(providerFormData.uuid!, {
                ...providerData,
                token: providerFormData.token || undefined,
            });

        if (result.success) {
            showNotification(t(`provider.notifications.${dialogMode === 'add' ? 'added' : 'updated'}`), 'success');
            setDialogOpen(false);
            loadProviders();
        } else {
            showNotification(t(`provider.notifications.${dialogMode === 'add' ? 'addFailed' : 'updateFailed'}`, { error: result.error }), 'error');
        }
    };

    const handleDeleteProvider = async (uuid: string) => {
        const result = await api.deleteProvider(uuid);

        if (result.success) {
            showNotification(t('provider.notifications.deleted'), 'success');
            loadProviders();
        } else {
            showNotification(t('provider.notifications.deleteFailed', { error: result.error }), 'error');
        }
    };

    const handleToggleProvider = async (uuid: string) => {
        const result = await api.toggleProvider(uuid);

        if (result.success) {
            showNotification(result.message, 'success');
            loadProviders();
        } else {
            showNotification(t('provider.notifications.toggleFailed', { error: result.error }), 'error');
        }
    };

    const handleEditProvider = async (uuid: string) => {
        const result = await api.getProvider(uuid);

        if (result.success) {
            const provider = result.data;
            setDialogMode('edit');
            setProviderFormData({
                uuid: provider.uuid,
                name: provider.name,
                apiBase: provider.api_base,
                apiStyle: provider.api_style || 'openai',
                token: provider.token || "",
                enabled: provider.enabled,
            });
            setDialogOpen(true);
        } else {
            showNotification(t('provider.notifications.loadDetailFailed', { error: result.error }), 'error');
        }
    };

    return (
        <PageLayout loading={loading}>
            {providers.length > 0 && (
                <UnifiedCard
                    title={t('provider.pageTitle')}
                    subtitle={providers.length > 0 ? t('provider.subtitleWithCount', { count: providers.length }) : t('provider.subtitleEmpty')}
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1} alignItems="center">
                            <Button
                                variant="contained"
                                startIcon={<Add />}
                                onClick={handleAddProviderClick}
                                size="small"
                            >
                                {t('provider.addButton')}
                            </Button>
                        </Stack>
                    }
                >
                    <Box sx={{ flex: 1 }}>
                        <ProviderTable
                            providers={providers}
                            onEdit={handleEditProvider}
                            onToggle={handleToggleProvider}
                            onDelete={handleDeleteProvider}
                        />
                    </Box>
                </UnifiedCard>
            )}

            {providers.length === 0 && (
                <UnifiedCard
                    title={t('provider.emptyCardTitle')}
                    subtitle={t('provider.emptyCardSubtitle')}
                    size="large"
                >
                    <Box textAlign="center" py={3}>
                        <Typography color="text.secondary" gutterBottom>
                            {t('provider.emptyCardContent')}
                        </Typography>
                        <Button
                            variant="contained"
                            startIcon={<Add />}
                            onClick={() => setDialogOpen(true)}
                            sx={{ mt: 2 }}
                        >
                            {t('provider.emptyCardButton')}
                        </Button>
                    </Box>
                </UnifiedCard>
            )}

            {/* Provider Dialog */}
            {/* <CredentialFormDialog
                open={dialogOpen}
                onClose={() => setDialogOpen(false)}
                onSubmit={handleProviderSubmit}
                data={providerFormData}
                onChange={(field, value) => setProviderFormData(prev => ({ ...prev, [field]: value }))}
                mode={dialogMode}
            /> */}

            <PresetProviderFormDialog
                open={dialogOpen}
                onClose={() => setDialogOpen(false)}
                onSubmit={handleProviderSubmit}
                data={providerFormData}
                onChange={(field, value) => setProviderFormData(prev => ({ ...prev, [field]: value }))}
                mode={dialogMode}
            />

            {/* Snackbar for notifications */}
            <Snackbar
                open={snackbar.open}
                autoHideDuration={6000}
                onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            >
                <Alert
                    onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
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

export default ProviderPage;
