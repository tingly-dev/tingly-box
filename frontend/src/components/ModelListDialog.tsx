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
import React, { useCallback, useEffect, useRef, useState } from 'react';
import api from '../services/api';
import ModelSelectDialog from './ModelSelectDialog.tsx';
import { ProbeV2Dialog } from './probe/ProbeV2Dialog';
import type { Provider } from '../types/provider';
import type { ProbeV2Response } from '@/types/probe-v2';

interface ModelListDialogProps {
    open: boolean;
    onClose: () => void;
    provider: Provider | null;
}

const ModelListDialog = ({ open, onClose, provider }: ModelListDialogProps) => {
    const [selectedModel, setSelectedModel] = useState<string>('');
    const [testing, setTesting] = useState(false);
    const [probeDialogOpen, setProbeDialogOpen] = useState(false);
    const [probeModel, setProbeModel] = useState<string>('');

    // Ref to track if dialog is still open (to avoid showing results after closing)
    const isDialogOpenRef = useRef(true);

    // Reset when dialog closes
    useEffect(() => {
        if (!open) {
            isDialogOpenRef.current = false;
            setTesting(false);
            setSelectedModel('');
            setProbeDialogOpen(false);
            setProbeModel('');
        } else {
            isDialogOpenRef.current = true;
        }
    }, [open]);

    const handleTest = async (model: string) => {
        if (!provider || testing) return;

        setTesting(true);
        setProbeModel(model);
        setProbeDialogOpen(true);

        // Note: ProbeV2Dialog handles the API call internally
        // We just need to open the dialog with the right parameters
        setTesting(false);
    };

    const handleCloseProbeDialog = () => {
        setProbeDialogOpen(false);
        setProbeModel('');
    };

    const handleClose = () => {
        onClose();
    };

    return (
        <>
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
                            selectedModel={selectedModel}
                            onSelected={(option) => setSelectedModel(option.model || '')}
                            onSelectionClear={() => setSelectedModel('')}
                            singleProvider={provider}
                            onTest={handleTest}
                            testing={testing}
                        />
                    </Box>
                </DialogContent>
            </Dialog>

            {/* Probe V2 Dialog */}
            {provider && probeModel && (
                <ProbeV2Dialog
                    open={probeDialogOpen}
                    onClose={handleCloseProbeDialog}
                    targetType="provider"
                    targetId={provider.uuid}
                    targetName={provider.name}
                    model={probeModel}
                    testMode="streaming"
                />
            )}
        </>
    );
};

export default ModelListDialog;