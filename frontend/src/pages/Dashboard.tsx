import { Box, CircularProgress, Alert } from '@mui/material';
import { useEffect, useState } from 'react';
import CardGrid, { CardGridItem } from '../components/CardGrid';
import ModelConfigCard from '../components/ModelConfigCard.tsx';
import ProvidersSummaryCard from '../components/ProvidersSummaryCard';
import ServerStatusCard from '../components/ServerStatusCard';
import AuthenticationCard from '../components/AuthenticationCard';
import RecentActivityCard from '../components/RecentActivityCard';
import ProviderSelectionCard from '../components/ProviderSelectionCard';
import { api } from '../services/api';

const Dashboard = () => {
    const [serverStatus, setServerStatus] = useState<any>(null);
    const [providersStatus, setProvidersStatus] = useState<any>(null);
    const [recentActivity, setRecentActivity] = useState<any[]>([]);
    const [defaults, setDefaults] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        loadAllData();
        const interval = setInterval(() => {
            loadServerStatus();
            loadProvidersStatus();
            loadRecentActivity();
        }, 30000);
        return () => clearInterval(interval);
    }, []);

    const loadAllData = async () => {
        setLoading(true);
        await Promise.all([
            loadServerStatus(),
            loadProvidersStatus(),
            loadRecentActivity(),
            loadDefaults(),
            loadProviderSelectionPanel(),
        ]);
        setLoading(false);
    };

    const loadServerStatus = async () => {
        const result = await api.getStatus();
        if (result.success) {
            setServerStatus(result.data);
        }
    };

    const loadProvidersStatus = async () => {
        const result = await api.getProviders();
        if (result.success) {
            setProvidersStatus(result.data);
        }
    };

    const loadRecentActivity = async () => {
        const result = await api.getHistory(5);
        if (result.success) {
            setRecentActivity(result.data);
        }
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

    const setDefaultProviderHandler = async (providerName: string) => {
        const currentDefaults = await api.getDefaults();
        if (!currentDefaults.success) {
            setMessage({ type: 'error', text: 'Failed to get current defaults' });
            return;
        }

        const payload = {
            defaultProvider: providerName,
            defaultModel: currentDefaults.data.defaultModel || '',
            requestModel: currentDefaults.data.requestModel || 'tingly',
            responseModel: currentDefaults.data.responseModel || '',
        };

        const result = await api.setDefaults(payload);
        if (result.success) {
            setMessage({ type: 'success', text: `Set ${providerName} as default provider` });
            await loadProviderSelectionPanel();
            await loadDefaults();
        } else {
            setMessage({ type: 'error', text: result.error });
        }
    };

    const fetchProviderModels = async (providerName: string) => {
        const result = await api.getProviderModelsByName(providerName);
        if (result.success) {
            setMessage({ type: 'success', text: `Successfully fetched models for ${providerName}` });
            await loadProviderSelectionPanel();
        } else {
            setMessage({ type: 'error', text: `Failed to fetch models: ${result.error}` });
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
            {message && (
                <Alert
                    severity={message.type}
                    sx={{ mb: 2 }}
                    onClose={() => setMessage(null)}
                >
                    {message.text}
                </Alert>
            )}

            <CardGrid>
                {/* Default Model Configuration */}
                <CardGridItem xs={12}>
                    <ModelConfigCard
                        defaults={defaults}
                        providers={providers}
                        providerModels={providerModels}
                        onLoadDefaults={loadDefaults}
                        onLoadProviderSelectionPanel={loadProviderSelectionPanel}
                    />
                </CardGridItem>

                {/* Provider Selection */}
                <CardGridItem xs={12} md={6}>
                    <ProviderSelectionCard
                        providers={providers}
                        defaults={defaults}
                        providerModels={providerModels}
                        onSetDefault={setDefaultProviderHandler}
                        onFetchModels={fetchProviderModels}
                    />
                </CardGridItem>

                {/* Server Status */}
                <CardGridItem xs={12} md={6}>
                    <ServerStatusCard
                        serverStatus={serverStatus}
                        onLoadServerStatus={loadServerStatus}
                    />
                </CardGridItem>

                {/* Providers Summary */}
                <CardGridItem xs={12} md={6}>
                    <ProvidersSummaryCard providersStatus={providersStatus} />
                </CardGridItem>

                {/* Authentication */}
                <CardGridItem xs={12} md={6}>
                    <AuthenticationCard />
                </CardGridItem>

                {/* Recent Activity */}
                <CardGridItem xs={12} md={6}>
                    <RecentActivityCard recentActivity={recentActivity} />
                </CardGridItem>
            </CardGrid>
        </Box>
    );
};

export default Dashboard;
