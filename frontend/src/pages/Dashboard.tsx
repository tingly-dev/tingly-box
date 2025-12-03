import { Box, CircularProgress } from '@mui/material';
import { useEffect, useState } from 'react';
import CardGrid, { CardGridItem } from '../components/CardGrid';
import ModelConfigCard from '../components/ModelConfigCard.tsx';
import { api } from '../services/api';

const Dashboard = () => {
    const [defaults, setDefaults] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        loadAllData();
    }, []);

    const loadAllData = async () => {
        setLoading(true);
        await Promise.all([
            loadDefaults(),
            loadProviderSelectionPanel(),
        ]);
        setLoading(false);
    };

    const loadDefaults = async () => {
        const result = await api.getDefaults();
        if (result.success) {
            setDefaults(result.data);
        }
    };

    const loadProviderSelectionPanel = async () => {
        const [providersResult, modelsResult, defaultsResult] = await Promise.all([
            api.getProviders(),
            api.getProviderModels(),
            api.getDefaults(),
        ]);

        if (providersResult.success && modelsResult.success) {
            setProviders(providersResult.data);
            setProviderModels(modelsResult.data);
            if (defaultsResult.success) {
                setDefaults(defaultsResult.data);
            }
        }
    };

    const fetchProviderModels = async (providerName: string) => {
        const result = await api.getProviderModelsByName(providerName);
        if (result.success) {
            await loadProviderSelectionPanel();
        }
    };

    if (loading) {
        return (
            <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
                <CircularProgress />
            </Box>
        );
    }

    return (
        <Box>
            <CardGrid>
                {/* Model Configuration */}
                <CardGridItem xs={12}>
                    <ModelConfigCard
                        defaults={defaults}
                        providers={providers}
                        providerModels={providerModels}
                        onLoadDefaults={loadDefaults}
                        onLoadProviderSelectionPanel={loadProviderSelectionPanel}
                        onFetchModels={fetchProviderModels}
                    />
                </CardGridItem>
            </CardGrid>
        </Box>
    );
};

export default Dashboard;
