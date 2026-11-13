import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, TextField, Typography } from '@mui/material';
import type { ConfigRecord } from '@/components/RoutingGraphTypes';
import ConfirmPopover from '@/components/ConfirmPopover';
import { radius } from '@tingly/ui/theme/radius';

// ============================================================================
// Delete Confirmation Popover
// ============================================================================

export interface RuleCardDeleteDialogProps {
    anchorEl: HTMLElement | null;
    onClose: () => void;
    onConfirm: () => void;
}

/**
 * Confirms rule deletion, opening at the control that asked for it
 */
export function RuleCardDeleteDialog({ anchorEl, onClose, onConfirm }: RuleCardDeleteDialogProps) {
    return (
        <ConfirmPopover
            anchorEl={anchorEl}
            onClose={onClose}
            onConfirm={onConfirm}
            title="Delete Routing Rule"
            description="This cannot be undone."
            confirmLabel="Delete"
            confirmColor="error"
        />
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
                    sx={{
                        '& .MuiInputBase-root': {
                            border: '1px solid',
                            borderColor: 'divider',
                            borderRadius: radius.card,
                            padding: '10px 12px',
                        },
                        '& .MuiInputBase-input': {
                            padding: 0,
                        },
                    }}
                    slotProps={{
                        input: { disableUnderline: true }
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
