import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import VirtualModelsTable from '@/components/VirtualModelsTable';
import EmptyState from '@/components/EmptyState';
import { Typography } from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNotify } from '@/hooks/useNotify';
import { api } from '../services/api';
import type { Provider } from '../types/provider';

const VirtualModelsPage = () => {
    const { t } = useTranslation();
    const notify = useNotify();
    const [providers, setProviders] = useState<Provider[]>([]);
    const [loading, setLoading] = useState(true);

    const loadProviders = async () => {
        setLoading(true);
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        } else {
            notify.error(`Failed to load providers: ${result.error}`);
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
            notify.success(result.message);
            loadProviders();
        } else {
            notify.error(`Failed to toggle provider: ${result.error}`);
        }
    };

    return (
        <PageLayout loading={loading}>
            <UnifiedCard
                title={t('layout.virtualModels', { defaultValue: 'Virtual Models' })}
                titleHeadingLevel={1}
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
                    <EmptyState
                        title="No Virtual Models Available"
                        description="Virtual models are seeded at server startup. Restart the server if this page is empty."
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
        </PageLayout>
    );
};

export default VirtualModelsPage;
