import {Delete, Edit, Route} from '@mui/icons-material';
import {
    Box,
    Button,
    FormControlLabel,
    IconButton,
    Modal,
    Paper,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tooltip,
    Typography,
} from '@mui/material';
import {useState} from 'react';
import {BotPlatformConfig, BotSettings} from '@/types/bot';

interface BotTableProps {
    bots: BotSettings[];
    platforms: BotPlatformConfig[];
    onEdit?: (uuid: string) => void;
    onToggle?: (uuid: string) => void;
    onDelete?: (uuid: string) => void;
}

interface DeleteModalState {
    open: boolean;
    uuid: string;
    name: string;
}

const BotTable = ({bots, platforms, onEdit, onToggle, onDelete}: BotTableProps) => {
    const [deleteModal, setDeleteModal] = useState<DeleteModalState>({
        open: false,
        uuid: '',
        name: '',
    });

    const getPlatformConfig = (platform: string): BotPlatformConfig | undefined => {
        return platforms.find(p => p.platform === platform);
    };

    const handleDeleteClick = (uuid: string) => {
        const bot = bots.find(b => b.uuid === uuid);
        setDeleteModal({
            open: true,
            uuid,
            name: bot?.name || bot?.platform || 'Unknown Bot',
        });
    };

    const handleCloseDeleteModal = () => {
        setDeleteModal({open: false, uuid: '', name: ''});
    };

    const handleConfirmDelete = () => {
        if (onDelete && deleteModal.uuid) {
            onDelete(deleteModal.uuid);
        }
        handleCloseDeleteModal();
    };

    return (
        <>
            <TableContainer component={Paper} elevation={0} sx={{border: 1, borderColor: 'divider'}}>
                <Table>
                    <TableHead>
                        <TableRow>
                            <TableCell sx={{fontWeight: 600, minWidth: 120}}>Status</TableCell>
                            <TableCell sx={{fontWeight: 600, minWidth: 120}}>Name</TableCell>
                            <TableCell sx={{fontWeight: 600, minWidth: 140}}>Platform</TableCell>
                            <TableCell sx={{fontWeight: 600, minWidth: 80}}>Proxy</TableCell>
                            <TableCell sx={{fontWeight: 600, minWidth: 120}}>Chat ID</TableCell>
                            <TableCell sx={{fontWeight: 600, minWidth: 100}}>Actions</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {bots.map((bot) => {
                            const platformConfig = getPlatformConfig(bot.platform);

                            return (
                                <TableRow key={bot.uuid}>
                                    {/* Status */}
                                    <TableCell>
                                        <Stack direction="row" alignItems="center" spacing={1}>
                                            <FormControlLabel
                                                control={
                                                    <Switch
                                                        checked={bot.enabled ?? true}
                                                        onChange={() => onToggle?.(bot.uuid!)}
                                                        size="small"
                                                        color="success"
                                                    />
                                                }
                                                label=""
                                            />
                                            <Typography variant="body2"
                                                        color={(bot.enabled ?? true) ? 'success.main' : 'error.main'}>
                                                {(bot.enabled ?? true) ? 'Enabled' : 'Disabled'}
                                            </Typography>
                                        </Stack>
                                    </TableCell>
                                    {/* Name */}
                                    <TableCell>
                                        <Typography variant="body2" sx={{fontWeight: 500}}>
                                            {bot.name || platformConfig?.display_name || bot.platform}
                                        </Typography>
                                    </TableCell>
                                    {/* Platform */}
                                    <TableCell>
                                        <Typography variant="body2" sx={{fontWeight: 500}}>
                                            {platformConfig?.display_name || bot.platform}
                                        </Typography>
                                    </TableCell>
                                    {/* Proxy */}
                                    <TableCell align="center">
                                        {bot.proxy_url ? (
                                            <Tooltip title={bot.proxy_url} arrow>
                                                <Route fontSize="small" sx={{color: 'text.secondary'}}/>
                                            </Tooltip>
                                        ) : (
                                            <Typography variant="body2" color="text.secondary">-</Typography>
                                        )}
                                    </TableCell>
                                    {/* Chat ID */}
                                    <TableCell>
                                        <Typography variant="body2" sx={{fontFamily: 'monospace'}}>
                                            {bot.chat_id || '-'}
                                        </Typography>
                                    </TableCell>
                                    {/* Actions */}
                                    <TableCell>
                                        <Stack direction="row" spacing={0.5}>
                                            {onEdit && (
                                                <Tooltip title="Edit">
                                                    <IconButton size="small" color="primary"
                                                                onClick={() => onEdit(bot.uuid!)}>
                                                        <Edit fontSize="small"/>
                                                    </IconButton>
                                                </Tooltip>
                                            )}
                                            {onDelete && (
                                                <Tooltip title="Delete">
                                                    <IconButton size="small" color="error"
                                                                onClick={() => handleDeleteClick(bot.uuid!)}>
                                                        <Delete fontSize="small"/>
                                                    </IconButton>
                                                </Tooltip>
                                            )}
                                        </Stack>
                                    </TableCell>
                                </TableRow>
                            );
                        })}
                    </TableBody>
                </Table>
            </TableContainer>

            {/* Delete Confirmation Modal */}
            <Modal open={deleteModal.open} onClose={handleCloseDeleteModal}>
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
                    <Typography variant="h6" sx={{mb: 2}}>Delete Bot Configuration</Typography>
                    <Typography variant="body2" sx={{mb: 3}}>
                        Are you sure you want to delete the bot configuration "{deleteModal.name}"? This action cannot
                        be undone.
                    </Typography>
                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <Button onClick={handleCloseDeleteModal} color="inherit">
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

export default BotTable;
