import {
    Box,
    Chip,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Tab,
    Tabs,
    Typography,
} from '@mui/material';
import { useCallback, useState } from 'react';
import CloseIcon from '@mui/icons-material/Close';
import SystemLogViewer from '@/components/SystemLogViewer';
import SmartRoutingLogViewer from '@/components/SmartRoutingLogViewer';
import type { SmartRoutingLogEntry } from '@/components/SmartRoutingLogViewer';

interface ScenarioLogDialogProps {
    open: boolean;
    onClose: () => void;
    scenario: string;
}

const getAuthHeader = () => ({
    Authorization: `Bearer ${localStorage.getItem('user_auth_token') || ''}`,
});

const ScenarioLogDialog = ({ open, onClose, scenario }: ScenarioLogDialogProps) => {
    const [tab, setTab] = useState(0);

    const getLogs = useCallback(
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

    const getSmartRoutingLogs = useCallback(
        async (params?: { limit?: number }) => {
            const q = new URLSearchParams();
            if (params?.limit) q.append('limit', String(params.limit));

            const res = await fetch(`/api/v1/system/smart-routing/logs?${q}`, { headers: getAuthHeader() });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();

            const logs: SmartRoutingLogEntry[] = (data.logs || []).filter(
                (e: SmartRoutingLogEntry) => e.fields?.scenario === scenario,
            );
            return { total: logs.length, logs };
        },
        [scenario],
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
        <Dialog open={open} onClose={onClose} maxWidth="xl" fullWidth PaperProps={{ sx: { height: '80vh' } }}>
            <DialogTitle sx={{ pb: 0 }}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Stack direction="row" alignItems="center" spacing={1}>
                        <Typography variant="h6">Logs</Typography>
                        <Chip label={scenario} size="small" variant="outlined" sx={{ fontSize: '0.72rem', height: 22 }} />
                    </Stack>
                    <IconButton size="small" onClick={onClose}>
                        <CloseIcon />
                    </IconButton>
                </Stack>
                <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mt: 1, borderBottom: 1, borderColor: 'divider' }}>
                    <Tab label="Model Requests" />
                    <Tab label="Smart Routing" />
                </Tabs>
            </DialogTitle>

            <DialogContent sx={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', pt: 1.5 }}>
                <Box sx={{ flex: 1, minHeight: 0, display: tab === 0 ? 'flex' : 'none', flexDirection: 'column' }}>
                    <SystemLogViewer
                        getLogs={getLogs}
                        getRequestBody={getRequestBody}
                        pathPrefix={`/tingly/${scenario}/`}
                    />
                </Box>
                <Box sx={{ flex: 1, minHeight: 0, display: tab === 1 ? 'flex' : 'none', flexDirection: 'column' }}>
                    <SmartRoutingLogViewer getLogs={getSmartRoutingLogs} />
                </Box>
            </DialogContent>
        </Dialog>
    );
};

export default ScenarioLogDialog;
