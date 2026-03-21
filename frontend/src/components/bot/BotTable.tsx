import type { BotPlatformConfig, BotSettings } from '@/types/bot';
import type { Provider } from '@/types/provider';
import {
    Box,
    Button,
    Modal,
    Paper,
    Skeleton,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
    Alert,
} from '@mui/material';
import { useCallback, useState } from 'react';
import BotGraphRow from './BotGraphRow';

interface BotTableProps {
    bots: BotSettings[];
    platforms: BotPlatformConfig[];
    providers: Provider[];
    onEdit?: (uuid: string) => void;
    onDelete?: (uuid: string) => void;
    onBotToggle?: (uuid: string, enabled: boolean) => void;
    onBotModelSelect?: (botUuid: string) => void;
    onCWDChange?: (botUuid: string, cwd: string) => void;
    defaultExpanded?: string[];
    loading?: boolean;
    error?: string | null;
    togglingBotUuid?: string | null;
}

const SkeletonRow = () => (
    <TableRow>
        {Array(7).fill(0).map((_, i) => (
            <TableCell key={i}>
                <Skeleton variant="text" width={i === 0 ? 40 : 80} />
            </TableCell>
        ))}
    </TableRow>
);

const BotTable = ({
    bots,
    platforms,
    providers,
    onEdit,
    onDelete,
    onBotToggle,
    onBotModelSelect,
    onCWDChange,
    defaultExpanded = [],
    loading = false,
    error = null,
    togglingBotUuid = null,
}: BotTableProps) => {
    const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set(defaultExpanded));
    const [deleteModal, setDeleteModal] = useState<{ open: boolean; uuid: string; name: string }>({
        open: false,
        uuid: '',
        name: '',
    });

    const handleToggleExpand = useCallback((botUuid: string) => {
        setExpandedRows(prev => {
            const next = new Set(prev);
            if (next.has(botUuid)) next.delete(botUuid);
            else next.add(botUuid);
            return next;
        });
    }, []);

    const handleDeleteClick = useCallback((uuid: string) => {
        const bot = bots.find(b => b.uuid === uuid);
        setDeleteModal({ open: true, uuid, name: bot?.name || bot?.platform || 'Unknown Bot' });
    }, [bots]);

    const handleConfirmDelete = useCallback(() => {
        onDelete?.(deleteModal.uuid);
        setDeleteModal({ open: false, uuid: '', name: '' });
    }, [onDelete, deleteModal.uuid]);

    const tableHeadCells = [
        { sx: { fontWeight: 600, minWidth: 60 }, label: '' },
        { sx: { fontWeight: 600, minWidth: 120 }, label: 'Alias' },
        { sx: { fontWeight: 600, minWidth: 140 }, label: 'Platform' },
        { sx: { fontWeight: 600, minWidth: 80 }, label: 'Proxy' },
        { sx: { fontWeight: 600, minWidth: 120 }, label: 'Chat ID' },
        { sx: { fontWeight: 600, minWidth: 150 }, label: 'Default Agent' },
        { sx: { fontWeight: 600, minWidth: 100 }, label: 'Actions' },
    ];

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
            <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
                <Table>
                    <TableHead>
                        <TableRow>
                            {tableHeadCells.map((cell, i) => (
                                <TableCell key={i} sx={cell.sx}>{cell.label}</TableCell>
                            ))}
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        <SkeletonRow />
                        <SkeletonRow />
                        <SkeletonRow />
                    </TableBody>
                </Table>
            </TableContainer>
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
            <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
                <Table>
                    <TableHead>
                        <TableRow>
                            {tableHeadCells.map((cell, i) => (
                                <TableCell key={i} sx={cell.sx}>{cell.label}</TableCell>
                            ))}
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {bots.map(bot => {
                            const isExpanded = expandedRows.has(bot.uuid || '');
                            const isToggling = togglingBotUuid === bot.uuid;
                            return (
                                <BotGraphRow
                                    key={bot.uuid}
                                    bot={bot}
                                    providers={providers}
                                    isExpanded={isExpanded}
                                    onToggleExpand={() => handleToggleExpand(bot.uuid || '')}
                                    onCWDChange={(cwd) => onCWDChange?.(bot.uuid!, cwd)}
                                    onModelClick={() => onBotModelSelect?.(bot.uuid!)}
                                    onBotToggle={onBotToggle}
                                    onEdit={onEdit ? () => onEdit(bot.uuid!) : undefined}
                                    onDelete={() => handleDeleteClick(bot.uuid!)}
                                    isToggling={isToggling}
                                />
                            );
                        })}
                    </TableBody>
                </Table>
            </TableContainer>

            <Modal open={deleteModal.open} onClose={() => setDeleteModal(prev => ({ ...prev, open: false }))}>
                <Box sx={{
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
                }}>
                    <Typography variant="h6" sx={{ mb: 2 }}>Delete Bot Configuration</Typography>
                    <Typography variant="body2" sx={{ mb: 3 }}>
                        Are you sure you want to delete the bot configuration "{deleteModal.name}"? This action cannot be undone.
                    </Typography>
                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <Button onClick={() => setDeleteModal(prev => ({ ...prev, open: false }))} color="inherit">Cancel</Button>
                        <Button onClick={handleConfirmDelete} color="error" variant="contained">Delete</Button>
                    </Stack>
                </Box>
            </Modal>
        </>
    );
};

export default BotTable;
