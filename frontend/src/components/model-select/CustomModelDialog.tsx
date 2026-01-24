import { Button, Dialog, DialogActions, DialogContent, DialogTitle, TextField } from '@mui/material';
import React, { useCallback } from 'react';
import { useModelSelectContext } from '../../contexts/ModelSelectContext';

export interface CustomModelDialogProps {
    onSave: () => void;
}

export function CustomModelDialog({ onSave }: CustomModelDialogProps) {
    const { customModelDialog, closeCustomModelDialog, updateCustomModelDialogValue } = useModelSelectContext();

    const handleSave = useCallback(() => {
        onSave();
    }, [onSave]);

    const handleCancel = useCallback(() => {
        closeCustomModelDialog();
    }, [closeCustomModelDialog]);

    return (
        <Dialog
            open={customModelDialog.open}
            onClose={handleCancel}
            maxWidth="sm"
            fullWidth
        >
            <DialogTitle>
                {customModelDialog.originalValue ? 'Edit Custom Model' : 'Add Custom Model'}
            </DialogTitle>
            <DialogContent>
                <TextField
                    autoFocus
                    margin="dense"
                    label="Model Name"
                    fullWidth
                    variant="outlined"
                    value={customModelDialog.value}
                    onChange={(e) => updateCustomModelDialogValue(e.target.value)}
                    placeholder="Enter custom model name..."
                    sx={{
                        mt: 1,
                        '& .MuiOutlinedInput-root': {
                            borderRadius: 1.5,
                        }
                    }}
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={handleCancel}>
                    Cancel
                </Button>
                <Button
                    onClick={handleSave}
                    variant="contained"
                    disabled={!customModelDialog.value?.trim()}
                >
                    {customModelDialog.originalValue ? 'Update' : 'Add'}
                </Button>
            </DialogActions>
        </Dialog>
    );
}

// Memoize to prevent unnecessary re-renders
export default React.memo(CustomModelDialog);
