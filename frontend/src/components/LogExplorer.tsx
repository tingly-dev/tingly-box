import { Box, Stack, Tab, Tabs } from '@mui/material';
import { useCallback, useState } from 'react';
import SystemLogViewer from '@/components/SystemLogViewer';
import RequestsViewer, {
    type ModelRequestDetail,
    type ModelRequestSummary,
    type RequestFilters,
} from '@/components/RequestsViewer';

interface LogExplorerProps {
    // When set, the scenario filter is initialized to this value but can be changed/cleared by the user.
    // Used by the per-scenario quick-open dialog to provide context without locking the view.
    initialScenario?: string;
}

const getAuthHeader = () => ({
    Authorization: `Bearer ${localStorage.getItem('user_auth_token') || ''}`,
});

const LogExplorer = ({ initialScenario }: LogExplorerProps) => {
    const [tab, setTab] = useState(0);

    const getRequests = useCallback(async (params?: RequestFilters) => {
        const q = new URLSearchParams();
        if (params?.limit) q.append('limit', String(params.limit));
        if (params?.scenario) q.append('scenario', params.scenario);
        if (params?.provider) q.append('provider', params.provider);
        if (params?.status) q.append('status', params.status);

        const res = await fetch(`/api/v1/requests?${q}`, { headers: getAuthHeader() });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        return { total: data.total || 0, requests: (data.requests || []) as ModelRequestSummary[] };
    }, []);

    const getRequestDetail = useCallback(async (id: string): Promise<ModelRequestDetail | null> => {
        const res = await fetch(`/api/v1/requests/${encodeURIComponent(id)}`, { headers: getAuthHeader() });
        if (!res.ok) {
            if (res.status === 404) return null;
            throw new Error(`HTTP ${res.status}`);
        }
        return (await res.json()) as ModelRequestDetail;
    }, []);

    const getSystemLogs = useCallback(
        async (params?: { limit?: number; level?: string; since?: string }) => {
            const q = new URLSearchParams();
            if (params?.limit) q.append('limit', String(params.limit));
            if (params?.level) q.append('level', params.level);
            if (params?.since) q.append('since', params.since);

            const res = await fetch(`/api/v1/system/logs?${q}`, { headers: getAuthHeader() });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            return { total: data.total || 0, logs: data.logs || [] };
        },
        [],
    );

    const getRequestBody = useCallback(async (bodyRef: string) => {
        const res = await fetch(`/api/v1/log/request/${bodyRef}`, { headers: getAuthHeader() });
        if (!res.ok) {
            if (res.status === 404) return null;
            throw new Error(`HTTP ${res.status}`);
        }
        return res.json();
    }, []);

    return (
        <Stack sx={{ height: '100%', minHeight: 0 }} spacing={0}>
            <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ borderBottom: 1, borderColor: 'divider' }}>
                <Tab label="AI Logs" />
                <Tab label="System Logs" />
            </Tabs>

            <Box sx={{ flex: 1, minHeight: 0, display: tab === 0 ? 'flex' : 'none', flexDirection: 'column' }}>
                <RequestsViewer
                    getRequests={getRequests}
                    getRequestDetail={getRequestDetail}
                    initialScenario={initialScenario}
                />
            </Box>
            <Box sx={{ flex: 1, minHeight: 0, display: tab === 1 ? 'flex' : 'none', flexDirection: 'column' }}>
                <SystemLogViewer getLogs={getSystemLogs} getRequestBody={getRequestBody} />
            </Box>
        </Stack>
    );
};

export default LogExplorer;
