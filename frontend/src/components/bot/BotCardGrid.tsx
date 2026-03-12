import { Alert, Box, Button, Modal, Paper, Skeleton, Stack, Typography } from '@mui/material';
import { useCallback, useState } from 'react';
import type { BotPlatformConfig, BotSettings } from '@/types/bot';
import type { Provider } from '@/types/provider';
import { CardGrid, CardGridItem } from '../CardGrid';
import BotCard from './BotCard';

interface BotCardGridProps {
    bots: BotSettings[];
    platforms: BotPlatformConfig[];
    providers: Provider[];
    onEdit?: (uuid: string) => void;
    onDelete?: (uuid: string) => void;
    onBotToggle?: (uuid: string, enabled: boolean) => void;
    onBotModelSelect?: (botUuid: string) => void;
    onCWDChange?: (botUuid: string, cwd: string) => void;
    loading?: boolean;
    error?: string | null;
    togglingBotUuid?: string | null;
}

const SkeletonCard = () => (
    <Paper
        elevation={0}
        sx={{
            border: 1,
            borderColor: 'divider',
            borderRadius: 2,
            p: 2,
            height: 200,
        }}
    >
        <Stack spacing={1}>
            <Skeleton variant="text" width={120} height={32} />
            <Skeleton variant="rectangular" width="100%" height={100} />
            <Skeleton variant="text" width={80} height={20} />
        </Stack>
    </Paper>
);

const BotCardGrid: React.FC<BotCardGridProps> = ({
    bots,
    platforms,
    providers,
    onEdit,
    onDelete,
    onBotToggle,
    onBotModelSelect,
    onCWDChange,
    loading = false,
    error = null,
    togglingBotUuid = null,
}) => {
    const [deleteModal, setDeleteModal] = useState<{ open: boolean; uuid: string; name: string }>({
        open: false,
        uuid: '',
        name: '',
    });

    const handleDeleteClick = useCallback((uuid: string) => {
        const bot = bots.find((b) => b.uuid === uuid);
        setDeleteModal({
            open: true,
            uuid,
            name: bot?.name || bot?.platform || 'Unknown Bot',
        });
    }, [bots]);

    const handleConfirmDelete = useCallback(() => {
        onDelete?.(deleteModal.uuid);
        setDeleteModal({ open: false, uuid: '', name: '' });
    }, [onDelete, deleteModal.uuid]);

    const handleBotToggle = useCallback((uuid: string, enabled: boolean) => {
        onBotToggle?.(uuid, enabled);
    }, [onBotToggle]);

    const handleBotModelClick = useCallback((uuid: string) => {
        onBotModelSelect?.(uuid);
    }, [onBotModelSelect]);

    const handleCWDChange = useCallback((uuid: string, cwd: string) => {
        onCWDChange?.(uuid, cwd);
    }, [onCWDChange]);

    // Error state
    if (error) {
        return (
            <Alert severity="error" sx={{ mb: 2 }}>
                {error}
            </Alert>
        );
    }

    // Loading state
    if (loading) {
        return (
            <CardGrid columns={{ xs: 12, sm: 6, md: 6, lg: 6, xl: 4 }}>
                <CardGridItem xs={12} sm={6} md={6} lg={6} xl={4}>
                    <SkeletonCard />
                </CardGridItem>
                <CardGridItem xs={12} sm={6} md={6} lg={6} xl={4}>
                    <SkeletonCard />
                </CardGridItem>
                <CardGridItem xs={12} sm={6} md={6} lg={6} xl={4}>
                    <SkeletonCard />
                </CardGridItem>
            </CardGrid>
        );
    }

    // Empty state
    if (bots.length === 0) {
        return (
            <Box sx={{ textAlign: 'center', py: 8 }}>
                <Typography variant="body1" color="text.secondary">
                    No bot configurations yet. Click "Add Bot" to create one.
                </Typography>
            </Box>
        );
    }

    return (
        <>
            <CardGrid columns={{ xs: 12, sm: 6, md: 6, lg: 6, xl: 4 }}>
                {bots.map((bot) => {
                    const isToggling = togglingBotUuid === bot.uuid;
                    return (
                        <CardGridItem key={bot.uuid} xs={12} sm={6} md={6} lg={6} xl={4}>
                            <BotCard
                                bot={bot}
                                providers={providers}
                                onEdit={() => onEdit?.(bot.uuid!)}
                                onDelete={() => handleDeleteClick(bot.uuid!)}
                                onBotToggle={(enabled) => handleBotToggle(bot.uuid!, enabled)}
                                onModelClick={() => handleBotModelClick(bot.uuid!)}
                                onCWDChange={(cwd) => handleCWDChange(bot.uuid!, cwd)}
                                isToggling={isToggling}
                            />
                        </CardGridItem>
                    );
                })}
            </CardGrid>

            {/* Delete Confirmation Modal */}
            <Modal open={deleteModal.open} onClose={() => setDeleteModal((prev) => ({ ...prev, open: false }))}>
                <Box
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 400,
                        maxWidth: '80vw',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography variant="h6" sx={{ mb: 2 }}>
                        Delete Bot Configuration
                    </Typography>
                    <Typography variant="body2" sx={{ mb: 3 }}>
                        Are you sure you want to delete the bot configuration "{deleteModal.name}"? This action
                        cannot be undone.
                    </Typography>
                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <Button onClick={() => setDeleteModal((prev) => ({ ...prev, open: false }))} color="inherit">
                            Cancel
                        </Button>
                        <Button onClick={handleConfirmDelete} color="error" variant="contained">
                            Delete
                        </Button>
                    </Stack>
                </Box>
            </Modal>
        </>
    );
};

export default BotCardGrid;
