import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Typography,
} from '@mui/material';
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
                    <Typography variant="body1" color="text.primary">
                        {hasRules
                            ? `You have ${ruleCount} smart ${ruleCount === 1 ? 'rule' : 'rules'} configured. They will be preserved but inactive in Direct mode.`
                            : "Switch to Direct Routing mode."
                        }
                    </Typography>
                </Box>
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
