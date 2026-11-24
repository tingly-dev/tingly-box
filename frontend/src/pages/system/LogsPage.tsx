import { Stack, Switch, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import LogExplorer from '@/components/LogExplorer';
import UnifiedCard from '@/components/UnifiedCard';
import { controlApi } from '@/services/openapi';

const LogsPage = () => {
    const [debugMode, setDebugMode] = useState(false);
    const [loadingDebug, setLoadingDebug] = useState(false);

    useEffect(() => {
        const fetchDebugMode = async () => {
            try {
                const data = await controlApi((client, headers) => client.GET('/api/v1/system/logs/level', {headers}));
                if (data?.success !== false) {
                    setDebugMode(data.level === 'debug');
                }
            } catch (error) {
                console.error('Failed to fetch debug mode:', error);
            }
        };

        void fetchDebugMode();
    }, []);

    const handleDebugModeChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
        const newDebugMode = event.target.checked;
        setLoadingDebug(true);
        try {
            const data = await controlApi((client, headers) => client.POST('/api/v1/system/logs/level', {
                headers,
                body: {level: newDebugMode ? 'debug' : 'info'},
            }));
            if (data?.success !== false) {
                setDebugMode(newDebugMode);
            }
        } catch (error) {
            console.error('Failed to set debug mode:', error);
        } finally {
            setLoadingDebug(false);
        }
    };

    return (
        <UnifiedCard
            title="Logs"
            size="full"
            height="calc(100vh - 48px)"
            titleMarginBottom={0.75}
            rightAction={
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <Typography variant="body2" color="text.secondary">
                        Debug Mode
                    </Typography>
                    <Switch
                        size="small"
                        checked={debugMode}
                        onChange={handleDebugModeChange}
                        disabled={loadingDebug}
                        slotProps={{ input: { 'aria-label': 'Debug Mode' } }}
                    />
                </Stack>
            }
        >
            <LogExplorer />
        </UnifiedCard>
    );
};

export default LogsPage;
