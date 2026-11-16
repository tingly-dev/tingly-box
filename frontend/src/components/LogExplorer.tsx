import { Box, Stack, Tab, Tabs } from '@mui/material';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import SystemLogViewer from '@/components/SystemLogViewer';
import AgentUsagePanel from '@/components/dashboard/AgentUsagePanel';
import AILogViewer, {
    type ModelRequestDetail,
    type ModelRequestSummary,
    type RequestFilters,
} from '@/components/AILogViewer.tsx';
import { type TraceDetail } from '@/components/RequestJourney';
import { getControlApiClient, getControlApiHeaders } from '@/services/openapi';

interface LogExplorerProps {
    // One agent's Log (the dialog its page opens): its requests only, plus a
    // Usage tab with today's numbers. Without it, every request (System → Logs).
    scenario?: string;
}

type LogTab = 'requests' | 'usage' | 'system';

const LogExplorer = ({ scenario }: LogExplorerProps) => {
    const { t } = useTranslation();
    const [tab, setTab] = useState<LogTab>('requests');

    const getRequests = useCallback(async (params?: RequestFilters) => {
        const [client, headers] = await Promise.all([getControlApiClient(), getControlApiHeaders()]);
        const result = await client.GET('/api/v1/requests', {
            headers,
            params: {query: {
                limit: params?.limit,
                scenario: params?.scenario,
                provider: params?.provider,
                status: params?.status,
            }},
        });
        if (!result.response.ok) throw new Error(`HTTP ${result.response.status}`);
        const data = result.data!;
        return { total: data.total || 0, requests: (data.requests || []) as ModelRequestSummary[] };
    }, []);

    const getRequestDetail = useCallback(async (id: string): Promise<ModelRequestDetail | null> => {
        const [client, headers] = await Promise.all([getControlApiClient(), getControlApiHeaders()]);
        const result = await client.GET('/api/v1/requests/{id}', {
            headers,
            params: {path: {id}},
        });
        if (!result.response.ok) {
            if (result.response.status === 404) return null;
            throw new Error(`HTTP ${result.response.status}`);
        }
        return result.data as ModelRequestDetail;
    }, []);

    const getTrace = useCallback(async (traceId: string): Promise<TraceDetail | null> => {
        const [client, headers] = await Promise.all([getControlApiClient(), getControlApiHeaders()]);
        const result = await client.GET('/api/v1/traces/{trace_id}', {
            headers,
            params: {path: {trace_id: traceId}},
        });
        if (!result.response.ok) {
            // 404 = evicted from the in-memory ring; render as "no longer buffered".
            if (result.response.status === 404) return null;
            throw new Error(`HTTP ${result.response.status}`);
        }
        return result.data as TraceDetail;
    }, []);

    const getSystemLogs = useCallback(
        async (params?: { limit?: number; level?: string; since?: string }) => {
            const [client, headers] = await Promise.all([getControlApiClient(), getControlApiHeaders()]);
            const result = await client.GET('/api/v1/system/logs', {
                headers,
                params: {query: {limit: params?.limit}},
            });
            if (!result.response.ok) throw new Error(`HTTP ${result.response.status}`);
            const data = result.data!;
            return { total: data.total || 0, logs: data.logs || [] };
        },
        [],
    );

    return (
        <Stack sx={{ height: '100%', minHeight: 0 }} spacing={0}>
            <Tabs
                value={tab}
                onChange={(_, v) => setTab(v)}
                aria-label={t('logViewer.views', { defaultValue: 'Log views' })}
                sx={{
                    minHeight: 40,
                    borderBottom: 1,
                    borderColor: 'divider',
                    '& .MuiTab-root': {
                        minWidth: 0,
                        minHeight: 40,
                        px: 2,
                        py: 0.75,
                        textTransform: 'none',
                        fontWeight: 600,
                    },
                }}
            >
                <Tab value="requests" label={t('logViewer.tabs.requests', { defaultValue: 'Requests' })} />
                {scenario && <Tab value="usage" label={t('logViewer.tabs.usage', { defaultValue: 'Usage' })} />}
                <Tab value="system" label={t('logViewer.tabs.system', { defaultValue: 'System' })} />
            </Tabs>

            {/* Panes stay mounted (filters and expanded rows survive a tab switch). */}
            <Box sx={{ flex: 1, minHeight: 0, pt: 1.5, display: tab === 'requests' ? 'flex' : 'none', flexDirection: 'column' }}>
                <AILogViewer
                    getRequests={getRequests}
                    getRequestDetail={getRequestDetail}
                    getTrace={getTrace}
                    scenario={scenario}
                />
            </Box>
            {scenario && tab === 'usage' && (
                <Box sx={{ flex: 1, minHeight: 0, pt: 2, overflow: 'auto' }}>
                    <AgentUsagePanel scenario={scenario} />
                </Box>
            )}
            <Box sx={{ flex: 1, minHeight: 0, pt: 1.5, display: tab === 'system' ? 'flex' : 'none', flexDirection: 'column' }}>
                <SystemLogViewer getLogs={getSystemLogs} />
            </Box>
        </Stack>
    );
};

export default LogExplorer;
