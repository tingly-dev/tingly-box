import { Box, Tab, Tabs } from '@mui/material';
import { useState } from 'react';
import RequestLog from '@/components/RequestLog';
import SystemLogViewer from '@/components/SystemLogViewer';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

const LogsPage = () => {
    const [currentTab, setCurrentTab] = useState(0);

    const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
        setCurrentTab(newValue);
    };

    return (
        <UnifiedCard size="full">
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
