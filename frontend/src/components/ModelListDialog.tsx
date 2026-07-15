import { Close as CloseIcon, ContentCopy } from '@/components/icons';
import {
    Box,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import ModelSelectDialog from './ModelSelectDialog.tsx';
import type { Provider } from '../types/provider';

interface ModelListDialogProps {
    open: boolean;
    onClose: () => void;
    provider: Provider | null;
}

const ModelListDialog = ({ open, onClose, provider }: ModelListDialogProps) => {
    const [selectedModel, setSelectedModel] = useState<string>('');

    // Reset selection when dialog closes
    useEffect(() => {
        if (!open) {
            setSelectedModel('');
        }
    }, [open]);

    const handleClose = () => {
        onClose();
    };

    return (
        <Dialog
            open={open}
            onClose={handleClose}
            maxWidth="lg"
            fullWidth
            PaperProps={{
                sx: { height: '80vh', display: 'flex', flexDirection: 'column' }
            }}
        >
            <DialogTitle sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="h6">Model List</Typography>
                <IconButton onClick={handleClose} size="small">
                    <CloseIcon />
                </IconButton>
            </DialogTitle>
            <DialogContent sx={{ p: 0 }}>
                <Box sx={{ height: '70vh', overflow: 'auto', p: 2 }}>
                    <ModelSelectDialog
                        providers={provider ? [provider] : []}
                        selectedProvider={provider?.uuid}
                        activeTab={provider?.uuid}
                        selectedModel={selectedModel}
                        onSelected={(option) => setSelectedModel(option.model || '')}
                        onSelectionClear={() => setSelectedModel('')}
                        singleProvider={provider}
                    />
                </Box>
            </DialogContent>
        </Dialog>
    );
};

export default ModelListDialog;