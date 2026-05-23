import {
    Chip,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import { useCallback } from 'react';
import CloseIcon from '@mui/icons-material/Close';
import RequestsViewer, {
    type ModelRequestDetail,
    type ModelRequestSummary,
    type RequestFilters,
} from '@/components/RequestsViewer';

interface ScenarioLogDialogProps {
    open: boolean;
    onClose: () => void;
    scenario: string;
}

const getAuthHeader = () => ({
    Authorization: `Bearer ${localStorage.getItem('user_auth_token') || ''}`,
});

const ScenarioLogDialog = ({ open, onClose, scenario }: ScenarioLogDialogProps) => {
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

    return (
        <Dialog open={open} onClose={onClose} maxWidth="xl" fullWidth PaperProps={{ sx: { height: '80vh' } }}>
            <DialogTitle sx={{ pb: 1 }}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Stack direction="row" alignItems="center" spacing={1}>
                        <Typography variant="h6">Requests</Typography>
                        <Chip label={scenario} size="small" variant="outlined" sx={{ fontSize: '0.72rem', height: 22 }} />
                    </Stack>
                    <IconButton size="small" onClick={onClose}>
                        <CloseIcon />
                    </IconButton>
                </Stack>
            </DialogTitle>

            <DialogContent sx={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', pt: 1.5 }}>
                <RequestsViewer
                    getRequests={getRequests}
                    getRequestDetail={getRequestDetail}
                    lockedScenario={scenario}
                />
            </DialogContent>
        </Dialog>
    );
};

export default ScenarioLogDialog;
