import { Box, FormControlLabel, Stack, Switch, Tab, Tabs, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import RequestLog from '@/components/RequestLog';
import SystemLogViewer from '@/components/SystemLogViewer';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

const LogsPage = () => {
    const [currentTab, setCurrentTab] = useState(0);
    const [debugMode, setDebugMode] = useState(false);
    const [loadingDebug, setLoadingDebug] = useState(false);

    // Fetch current debug mode on mount
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
            } else {
                console.error('Failed to set debug mode');
            }
        } catch (error) {
            console.error('Failed to set debug mode:', error);
        } finally {
            setLoadingDebug(false);
        }
    };

    const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
        setCurrentTab(newValue);
    };

    return (
        <UnifiedCard
            title="Logs"
            size="full"
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
            <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
                    <Tabs value={currentTab} onChange={handleTabChange}>
                        <Tab label="HTTP Logs" />
                        <Tab label="System Logs" />
                    </Tabs>
                </Box>
                <Box sx={{ flex: 1, overflow: 'hidden', display: currentTab === 0 ? 'block' : 'none' }}>
                    <RequestLog
                        getLogs={async () => {
                            try {
                                const { logsApi } = await api.instances();
                                const response = await logsApi.apiV1LogGet();
                                return {
                                    total: response.data.total || 0,
                                    logs: response.data.logs || [],
                                };
                            } catch (error: any) {
                                console.error('Failed to get logs:', error);
                                return { total: 0, logs: [] };
                            }
                        }}
                        clearLogs={async () => {
                            try {
                                const { logsApi } = await api.instances();
                                await logsApi.apiV1LogDelete();
                                return { success: true, message: 'Logs cleared' };
                            } catch (error: any) {
                                console.error('Failed to clear logs:', error);
                                return { success: false, message: error.message || 'Failed to clear logs' };
                            }
                        }}
                    />
                </Box>
                <Box sx={{ flex: 1, overflow: 'hidden', display: currentTab === 1 ? 'block' : 'none' }}>
                    <SystemLogViewer
                        getLogs={async (params) => {
                            try {
                                const queryParams = new URLSearchParams();
                                if (params?.limit) queryParams.append('limit', params.limit.toString());
                                if (params?.level) queryParams.append('level', params.level);
                                if (params?.since) queryParams.append('since', params.since);

                                const response = await fetch(`/api/v1/system/logs?${queryParams.toString()}`, {
                                    headers: {
                                        'Authorization': `Bearer ${localStorage.getItem('user_auth_token') || ''}`,
                                    },
                                });

                                if (!response.ok) {
                                    throw new Error(`HTTP error! status: ${response.status}`);
                                }

                                const data = await response.json();
                                return {
                                    total: data.total || 0,
                                    logs: data.logs || [],
                                };
                            } catch (error: any) {
                                console.error('Failed to get system logs:', error);
                                return { total: 0, logs: [] };
                            }
                        }}
                    />
                </Box>
            </Box>
        </UnifiedCard>
    );
};

export default LogsPage;
