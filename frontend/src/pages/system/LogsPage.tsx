import { Stack, Switch, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import LogExplorer from '@/components/LogExplorer';
import UnifiedCard from '@/components/UnifiedCard';

const LogsPage = () => {
    const [debugMode, setDebugMode] = useState(false);
    const [loadingDebug, setLoadingDebug] = useState(false);

    useEffect(() => {
        fetchDebugMode();
    }, []);

    const fetchDebugMode = async () => {
        try {
            const response = await fetch('/api/v1/system/logs/level', {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('user_auth_token') || ''}`,
                },
            });
            if (response.ok) {
                const data = await response.json();
                setDebugMode(data.level === 'debug');
            }
        } catch (error) {
            console.error('Failed to fetch debug mode:', error);
        }
    };

    const handleDebugModeChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
        const newDebugMode = event.target.checked;
        setLoadingDebug(true);
        try {
            const response = await fetch('/api/v1/system/logs/level', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('user_auth_token') || ''}`,
                },
                body: JSON.stringify({ level: newDebugMode ? 'debug' : 'info' }),
            });
            if (response.ok) {
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
            rightAction={
                <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="body2" color="text.secondary">
                        Debug Mode
                    </Typography>
                    <Switch
                        checked={debugMode}
                        onChange={handleDebugModeChange}
                        disabled={loadingDebug}
                        size="small"
                    />
                </Stack>
            }
        >
            <LogExplorer />
        </UnifiedCard>
    );
};

export default LogsPage;
