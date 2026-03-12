import { useState, useCallback } from 'react';
import { Dialog, DialogTitle, DialogContent, Typography } from '@mui/material';
import type { Provider } from '@/types/provider';
import type { ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import ModelSelectDialog from '@/components/ModelSelectDialog';
import api from '@/services/api';
import type { BotSettings } from '@/types/bot';

interface BotModelDialogOptions {
    bot: BotSettings | null;
    providers: Provider[];
    onUpdate: (uuid: string, provider: string, model: string) => Promise<void>;
    onClose: () => void;
}

export const useBotModelDialog = (options: BotModelDialogOptions) => {
    const { bot, providers, onUpdate, onClose } = options;

    const [open, setOpen] = useState(false);
    const [saving, setSaving] = useState(false);

    const openDialog = useCallback(() => {
        setOpen(true);
    }, []);

    const closeDialog = useCallback(() => {
        setOpen(false);
        onClose?.();
    }, [onClose]);

    const handleModelSelect = useCallback(async (option: ProviderSelectTabOption) => {
        if (!bot) return;

        setSaving(true);
        try {
            await onUpdate(bot.uuid!, option.provider.uuid, option.model);
            closeDialog();
        } finally {
            setSaving(false);
        }
    }, [bot, onUpdate, closeDialog]);

    const handleSelectionClear = useCallback(async () => {
        if (!bot) return;

        setSaving(true);
        try {
            await onUpdate(bot.uuid!, '', '');
            closeDialog();
        } finally {
            setSaving(false);
        }
    }, [bot, onUpdate, closeDialog]);

    // Get current selection for pre-selection in dialog
    const selectedProvider = bot?.smartguide_provider;
    const selectedModel = bot?.smartguide_model;

    // Dialog component - using the same pattern as useModelSelectDialog
    const BotModelDialog: React.FC<{ open: boolean }> = ({ open: dialogOpen }) => {
        if (!dialogOpen) return null;

        return (
            <Dialog
                open={dialogOpen}
                onClose={closeDialog}
                maxWidth="lg"
                fullWidth
                PaperProps={{
                    sx: { height: '80vh' }
                }}
            >
                <DialogTitle sx={{ textAlign: 'center' }}>
                    <Typography variant="h6">Configure SmartGuide Model</Typography>
                </DialogTitle>
                <DialogContent>
                    <ModelSelectDialog
                        providers={providers}
                        selectedProvider={selectedProvider}
                        selectedModel={selectedModel}
                        onSelected={handleModelSelect}
                        onSelectionClear={handleSelectionClear}
                    />
                </DialogContent>
            </Dialog>
        );
    };

    return {
        openDialog,
        closeDialog,
        BotModelDialog,
        isOpen: open,
        isSaving: saving,
    };
};
