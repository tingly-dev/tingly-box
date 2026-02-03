import { Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, Typography } from '@mui/material';
import type { ProbeResponse } from '@/client';
import Probe from '@/components/ProbeModal';
import type { ConfigRecord } from '@/components/RoutingGraphTypes';

// ============================================================================
// Probe Dialog
// ============================================================================

export interface RuleCardProbeDialogProps {
    open: boolean;
    onClose: () => void;
    configRecord: ConfigRecord | null;
    isProbing: boolean;
    probeResult: ProbeResponse | null;
    detailsExpanded: boolean;
    providerName: string;
    onToggleDetails: () => void;
}

/**
 * Dialog component for displaying probe results
 */
export function RuleCardProbeDialog({
    open,
    onClose,
    configRecord,
    isProbing,
    probeResult,
    detailsExpanded,
    providerName,
    onToggleDetails,
}: RuleCardProbeDialogProps) {
    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="md"
            fullWidth
            PaperProps={{
                sx: { height: 'auto', maxHeight: '80vh' },
            }}
        >
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography variant="h6">Connection Test Result</Typography>
                <Typography variant="body2" color="text.secondary">
                    {providerName} / {configRecord?.providers[0]?.model || ''}
                </Typography>
            </DialogTitle>
            <DialogContent>
                <Probe
                    provider={configRecord?.providers[0]?.provider}
                    model={configRecord?.providers[0]?.model}
                    isProbing={isProbing}
                    probeResult={probeResult}
                    onToggleDetails={onToggleDetails}
                    detailsExpanded={detailsExpanded}
                />
            </DialogContent>
        </Dialog>
    );
}

// ============================================================================
// Delete Confirmation Dialog
// ============================================================================

export interface RuleCardDeleteDialogProps {
    open: boolean;
    onClose: () => void;
    onConfirm: () => void;
}

/**
 * Dialog component for confirming rule deletion
 */
export function RuleCardDeleteDialog({ open, onClose, onConfirm }: RuleCardDeleteDialogProps) {
    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>Delete Routing Rule</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    Are you sure you want to delete this routing rule? This action cannot be undone.
                </DialogContentText>
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose} color="primary">
                    Cancel
                </Button>
                <Button onClick={onConfirm} color="error" variant="contained">
                    Delete
                </Button>
            </DialogActions>
        </Dialog>
    );
}
