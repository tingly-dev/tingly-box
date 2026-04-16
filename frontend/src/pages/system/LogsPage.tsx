import { Box, FormControlLabel, Stack, Switch, Typography, Alert, Tabs, Tab } from '@mui/material';
import { useEffect, useState } from 'react';
import SystemLogViewer from '@/components/SystemLogViewer';
import UnifiedCard from '@/components/UnifiedCard';

interface TabPanelProps {
    children?: React.ReactNode;
    index: number;
    value: number;
}

function TabPanel(props: TabPanelProps) {
    const { children, value, index, ...other } = props;
    return (
        <div
            role="tabpanel"
            hidden={value !== index}
            id={`logs-tabpanel-${index}`}
            aria-labelledby={`logs-tab-${index}`}
            style={{ height: '100%', overflow: 'hidden' }}
            {...other}
        >
            {value === index && <Box sx={{ height: '100%' }}>{children}</Box>}
        </div>
    );
}

const LogsPage = () => {
    const [tabValue, setTabValue] = useState(0);
    const [debugMode, setDebugMode] = useState(false);
    const [loadingDebug, setLoadingDebug] = useState(false);
    const [logError, setLogError] = useState<string | null>(null);

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
            } else {
                // Try to extract error message from response
                let errorMsg = `Failed to fetch debug mode (${response.status})`;
                try {
                    const errData = await response.json();
                    if (errData.error) errorMsg = errData.error;
                } catch {}
                console.error(errorMsg);
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
                // Try to extract error message from response
                let errorMsg = `Failed to set debug mode (${response.status})`;
                try {
                    const errData = await response.json();
                    if (errData.error) errorMsg = errData.error;
                } catch {}
                console.error(errorMsg);
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
            <Stack sx={{ height: '100%', minHeight: 0 }} spacing={0}>
                <Tabs
                    value={tabValue}
                    onChange={(_, newValue) => setTabValue(newValue)}
                    sx={{ borderBottom: 1, borderColor: 'divider' }}
                >
                    <Tab label="HTTP Requests" />
                    <Tab label="System Logs" />
                </Tabs>

                {logError && (
                    <Alert severity="error" onClose={() => setLogError(null)} sx={{ m: 1 }}>
                        {logError}
                    </Alert>
                )}

                <TabPanel value={tabValue} index={0}>
                    <HttpLogViewer
                        token={localStorage.getItem('user_auth_token') || ''}
                        onError={setLogError}
                    />
                </TabPanel>

                <TabPanel value={tabValue} index={1}>
                    <SystemLogViewer
                        getLogs={async (params) => {
                            setLogError(null);
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
                                    let errorDetail = `HTTP error! status: ${response.status}`;
                                    try {
                                        const errorData = await response.json();
                                        if (errorData.error) {
                                            errorDetail = errorData.error;
                                        }
                                    } catch {
                                        errorDetail = response.statusText || errorDetail;
                                    }
                                    throw new Error(errorDetail);
                                }

                                const data = await response.json();
                                return {
                                    total: data.total || 0,
                                    logs: data.logs || [],
                                };
                            } catch (error: any) {
                                const errorMessage = error instanceof Error ? error.message : 'Unknown error';
                                console.error('Failed to get system logs:', errorMessage);
                                setLogError(`Failed to load system logs: ${errorMessage}`);
                                return { total: 0, logs: [] };
                            }
                        }}
                    />
                </TabPanel>
            </Stack>
        </UnifiedCard>
    );
};

// HTTP Request Log Viewer Component
interface HttpLogViewerProps {
    token: string;
    onError: (error: string | null) => void;
}

const HttpLogViewer = ({ token, onError }: HttpLogViewerProps) => {
    const getLogs = async (params?: { limit?: number; level?: string; since?: string }) => {
        try {
            const queryParams = new URLSearchParams();
            if (params?.limit) queryParams.append('limit', params.limit.toString());
            if (params?.level) queryParams.append('level', params.level);
            if (params?.since) queryParams.append('since', params.since);

            const response = await fetch(`/api/v1/log?${queryParams.toString()}`, {
                headers: {
                    'Authorization': `Bearer ${token}`,
                },
            });

            if (!response.ok) {
                let errorDetail = `HTTP error! status: ${response.status}`;
                try {
                    const errorData = await response.json();
                    if (errorData.error) {
                        errorDetail = errorData.error;
                    }
                } catch {
                    errorDetail = response.statusText || errorDetail;
                }
                throw new Error(errorDetail);
            }

            const data = await response.json();
            return {
                total: data.total || 0,
                logs: data.logs || [],
            };
        } catch (error: any) {
            const errorMessage = error instanceof Error ? error.message : 'Unknown error';
            console.error('Failed to get HTTP logs:', errorMessage);
            onError(`Failed to load HTTP logs: ${errorMessage}`);
            return { total: 0, logs: [] };
        }
    };

    const getRequestBody = async (bodyRef: string) => {
        try {
            const response = await fetch(`/api/v1/log/request/${bodyRef}`, {
                headers: {
                    'Authorization': `Bearer ${token}`,
                },
            });

            if (!response.ok) {
                if (response.status === 404) {
                    return null;
                }
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            return await response.json();
        } catch (error: any) {
            console.error('Failed to get request body:', error);
            throw error;
        }
    };

    return (
        <SystemLogViewer
            getLogs={getLogs}
            getRequestBody={getRequestBody}
        />
    );
};

export default LogsPage;
