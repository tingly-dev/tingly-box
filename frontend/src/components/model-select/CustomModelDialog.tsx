import { Button, Dialog, DialogActions, DialogContent, DialogTitle, TextField } from '@mui/material';
import React, { useCallback } from 'react';
import { useModelSelectContext } from '../../contexts/ModelSelectContext';
import { useCustomModels } from '../../hooks/useCustomModels';
import { dispatchCustomModelUpdate } from '../../hooks/useCustomModels';

export interface CustomModelDialogProps {
    onCustomModelSave?: (providerUuid: string, customModel: string) => void;
}

export function CustomModelDialog({ onCustomModelSave }: CustomModelDialogProps) {
    const { customModelDialog, closeCustomModelDialog, updateCustomModelDialogValue } = useModelSelectContext();
    const { saveCustomModel, updateCustomModel } = useCustomModels();

    const handleSave = useCallback(() => {
        const customModel = customModelDialog.value?.trim();
        if (customModel && customModelDialog.provider) {
            if (customModelDialog.originalValue) {
                // Editing: use updateCustomModel to atomically replace old value with new value
                updateCustomModel(customModelDialog.provider.uuid, customModelDialog.originalValue, customModel);
                dispatchCustomModelUpdate(customModelDialog.provider.uuid, customModel);
            } else {
                // Adding new: use saveCustomModel
                if (saveCustomModel(customModelDialog.provider.uuid, customModel)) {
                    dispatchCustomModelUpdate(customModelDialog.provider.uuid, customModel);
                }
            }

            // Then save to persistence through parent component
            if (onCustomModelSave) {
                onCustomModelSave(customModelDialog.provider.uuid, customModel);
            }
        }
        closeCustomModelDialog();
    }, [customModelDialog, saveCustomModel, updateCustomModel, onCustomModelSave, closeCustomModelDialog]);

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
const MemoizedCustomModelDialog = React.memo(CustomModelDialog);
export default MemoizedCustomModelDialog;
export { CustomModelDialog };
