import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import type { Provider } from '@/types/provider';
import { Add, Refresh as RefreshIcon } from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Stack,
    Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';

const AgentPage = () => {
    const [imbots, setImbots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Load ImBot settings
    const loadImbots = useCallback(async () => {
        setLoading(true);
        setError(null);

        try {
            const response = await api.getImBotSettingsList();
            if (response?.success && Array.isArray(response.settings)) {
                setImbots(response.settings);
            } else if (response?.success === false) {
                setError(response.error || 'Failed to load ImBot settings');
            }
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to load data');
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadImbots();
    }, [loadImbots]);

    // Handlers for bot operations
    const handleBotToggle = useCallback(async (uuid: string, enabled: boolean) => {
        try {
            const result = await api.toggleImBotSetting(uuid);
            if (result?.success) {
                await loadImbots();
            } else {
                setError(result?.error || 'Failed to update bot');
            }
        } catch (err) {
            setError('Failed to update bot');
        }
    }, [loadImbots]);

    const handleCWDChange = useCallback(async (uuid: string, cwd: string) => {
        try {
            const result = await api.updateImbotSetting(uuid, { default_cwd: cwd });
            if (result?.success) {
                await loadImbots();
            } else {
                setError(result?.error || 'Failed to update working directory');
            }
        } catch (err) {
            setError('Failed to update working directory');
        }
    }, [loadImbots]);

    const handleAgentClick = useCallback((botUuid: string, currentAgent: string | null) => {
        // For now, just log - agent selection needs to be implemented
        console.log('Agent click for bot:', botUuid, 'current agent:', currentAgent);
    }, []);

    if (loading) {
        return (
            <PageLayout>
                <UnifiedCard
                    title="Agents"
                    subtitle="AI agent configuration"
                    size="full"
                >
                    <Stack direction="row" justifyContent="center" py={8} spacing={2}>
                        <CircularProgress size={32} />
                        <Typography variant="body2" color="text.secondary">
                            Loading agent configurations...
                        </Typography>
                    </Stack>
                </UnifiedCard>
            </PageLayout>
        );
    }

    if (error) {
        return (
            <PageLayout>
                <UnifiedCard
                    title="Agents"
                    subtitle="AI agent configuration"
                    size="full"
                >
                    <Stack spacing={2} py={4}>
                        <Alert severity="error">{error}</Alert>
                        <Button onClick={loadImbots} startIcon={<RefreshIcon />}>
                            Retry
                        </Button>
                    </Stack>
                </UnifiedCard>
            </PageLayout>
        );
    }

    return (
        <PageLayout>
            <UnifiedCard
                title="Agents"
                subtitle="Configure and manage AI agents for remote control"
                size="full"
                rightAction={
                    <Button onClick={loadImbots} startIcon={<RefreshIcon />} size="small">
                        Refresh
                    </Button>
                }
            >

            </UnifiedCard>
        </PageLayout>
    );
};

export default AgentPage;
