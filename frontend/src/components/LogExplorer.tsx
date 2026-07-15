import { Box, Stack, Tab, Tabs } from '@mui/material';
import { useCallback, useState } from 'react';
import SystemLogViewer from '@/components/SystemLogViewer';
import AILogViewer, {
    type ModelRequestDetail,
    type ModelRequestSummary,
    type RequestFilters,
} from '@/components/AILogViewer.tsx';
import { getControlApiClient, getControlApiHeaders } from '@/services/openapi';

interface LogExplorerProps {
    // When set, the scenario filter is initialized to this value but can be changed/cleared by the user.
    // Used by the per-scenario quick-open dialog to provide context without locking the view.
    initialScenario?: string;
}

const LogExplorer = ({ initialScenario }: LogExplorerProps) => {
    const [tab, setTab] = useState(0);

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
            <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ borderBottom: 1, borderColor: 'divider' }}>
                <Tab label="AI Logs" />
                <Tab label="System Logs" />
            </Tabs>

            <Box sx={{ flex: 1, minHeight: 0, display: tab === 0 ? 'flex' : 'none', flexDirection: 'column' }}>
                <AILogViewer
                    getRequests={getRequests}
                    getRequestDetail={getRequestDetail}
                    initialScenario={initialScenario}
                />
            </Box>
            <Box sx={{ flex: 1, minHeight: 0, display: tab === 1 ? 'flex' : 'none', flexDirection: 'column' }}>
                <SystemLogViewer getLogs={getSystemLogs} />
            </Box>
        </Stack>
    );
};

export default LogExplorer;
