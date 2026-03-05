import {
    Alert,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Typography,
} from '@mui/material';
import WarningIcon from '@mui/icons-material/Warning';
import React from 'react';

interface RoutingModeConfirmDialogProps {
    open: boolean;
    hasRules: boolean;
    ruleCount?: number;
    onConfirm: () => void;
    onCancel: () => void;
}

export const RoutingModeConfirmDialog: React.FC<RoutingModeConfirmDialogProps> = ({
    open,
    hasRules,
    ruleCount = 0,
    onConfirm,
    onCancel,
}) => {
    return (
        <Dialog
            open={open}
            onClose={onCancel}
            maxWidth="sm"
            fullWidth
            PaperProps={{
                sx: {
                    borderRadius: 2,
                },
            }}
        >
            <DialogTitle>Switch to Direct Routing?</DialogTitle>
            <DialogContent>
                <Box sx={{ mb: 2 }}>
                    {hasRules ? (
                        <Alert severity="warning" icon={<WarningIcon fontSize="inherit" />} sx={{ mb: 2 }}>
                            You have {ruleCount} smart {ruleCount === 1 ? 'rule' : 'rules'} configured.
                        </Alert>
                    ) : (
                        <Alert severity="info" sx={{ mb: 2 }}>
                            No smart rules will be affected.
                        </Alert>
                    )}

                    <Typography variant="body1" color="text.primary">
                        {hasRules
                            ? "Switching to Direct Routing will remove all smart rules. Your default providers will be kept as direct routing providers."
                            : "Switch to Direct Routing mode. Your providers will handle requests directly."
                        }
                    </Typography>
                </Box>

                {hasRules && (
                    <Box
                        sx={{
                            p: 2,
                            backgroundColor: 'warning.50',
                            borderRadius: 1,
                            border: '1px solid',
                            borderColor: 'warning.200',
                        }}
                    >
                        <Typography variant="caption" color="text.secondary">
                            <strong>Tip:</strong> Consider exporting your smart rules before switching if you might need them later.
                        </Typography>
                    </Box>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onCancel} color="inherit">
                    Cancel
                </Button>
                <Button
                    onClick={onConfirm}
                    variant="contained"
                    color="primary"
                    autoFocus
                >
                    Switch to Direct
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default RoutingModeConfirmDialog;
