import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, TextField, Typography } from '@mui/material';
import type { ConfigRecord } from '@/components/RoutingGraphTypes';

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

// ============================================================================
// Rule Flag Edit Dialog
// ============================================================================

export interface RuleFlagEditDialogProps {
    open: boolean;
    value: string;
    error?: string;
    onChange: (value: string) => void;
    onClose: () => void;
    onSave: () => void;
}

export function RuleFlagEditDialog({ open, value, error, onChange, onClose, onSave }: RuleFlagEditDialogProps) {
    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>Edit Rule Flags</DialogTitle>
            <DialogContent>
                <DialogContentText sx={{ mb: 2 }}>
                    Use comma-separated key/value pairs. Example: <strong>cursor_compat=true,cursor_compat_auto=false</strong>
                </DialogContentText>
                <TextField
                    autoFocus
                    fullWidth
                    label="Flags"
                    value={value}
                    onChange={(e) => onChange(e.target.value)}
                    error={Boolean(error)}
                    helperText={error || ' '}
                    variant="standard"
                    InputProps={{ disableUnderline: true }}
                    sx={{
                        '& .MuiInputBase-root': {
                            border: '1px solid',
                            borderColor: 'divider',
                            borderRadius: 2,
                            padding: '10px 12px',
                        },
                        '& .MuiInputBase-input': {
                            padding: 0,
                        },
                    }}
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose} color="primary">
                    Cancel
                </Button>
                <Button onClick={onSave} color="primary" variant="contained">
                    Save
                </Button>
            </DialogActions>
        </Dialog>
    );
}
