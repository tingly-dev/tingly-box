import { Cancel, CheckCircle, ContentCopy, Delete, Edit, Visibility } from '@mui/icons-material';
import {
    Box,
    Button,
    FormControlLabel,
    IconButton,
    Menu,
    MenuItem,
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
    Typography
} from '@mui/material';
import { useState } from 'react';
import type { ProviderModelsData, Provider } from '../types/provider';



interface ProviderTableProps {
    providers: Provider[];
    providerModels?: ProviderModelsData;
    onEdit?: (providerName: string) => void;
    onToggle?: (providerName: string) => void;
    onDelete?: (providerName: string) => void;
    onSetDefault?: (providerName: string) => void;
    onFetchModels?: (providerName: string) => void;
}

interface TokenMenuState {
    anchor: HTMLElement | null;
    showToken: boolean;
}

interface TokenModalState {
    open: boolean;
    providerName: string;
    token: string;
}

interface DeleteModalState {
    open: boolean;
    providerName: string;
}

const ProviderTable = ({
    providers,
    onEdit,
    onToggle,
    onDelete,
}: ProviderTableProps) => {
    const [tokenStates, setTokenStates] = useState<{ [key: string]: TokenMenuState }>({});
    const [tokenModal, setTokenModal] = useState<TokenModalState>({
        open: false,
        providerName: '',
        token: ''
    });
    const [deleteModal, setDeleteModal] = useState<DeleteModalState>({
        open: false,
        providerName: ''
    });

    const handleTokenMenuClick = (event: React.MouseEvent<HTMLElement>, providerName: string) => {
        setTokenStates(prev => ({
            ...prev,
            [providerName]: {
                anchor: event.currentTarget,
                showToken: prev[providerName]?.showToken || false
            }
        }));
    };

    const handleTokenMenuClose = (providerName: string) => {
        setTokenStates(prev => ({
            ...prev,
            [providerName]: {
                ...prev[providerName],
                anchor: null
            }
        }));
    };

    const handleViewToken = (providerName: string) => {
        const provider = providers.find(p => p.name === providerName);
        if (provider && provider.token) {
            setTokenModal({
                open: true,
                providerName: providerName,
                token: provider.token
            });
        }
        handleTokenMenuClose(providerName);
    };

    const handleCloseTokenModal = () => {
        setTokenModal({
            open: false,
            providerName: '',
            token: ''
        });
    };

    const handleDeleteClick = (providerName: string) => {
        setDeleteModal({
            open: true,
            providerName
        });
    };

    const handleCloseDeleteModal = () => {
        setDeleteModal({
            open: false,
            providerName: ''
        });
    };

    const handleConfirmDelete = () => {
        if (onDelete && deleteModal.providerName) {
            onDelete(deleteModal.providerName);
        }
        handleCloseDeleteModal();
    };

    const handleCopyToken = async (provider: Provider) => {
        if (provider.token) {
            try {
                await navigator.clipboard.writeText(provider.token);
            } catch (err) {
                console.error('Failed to copy token:', err);
            }
        }
        handleTokenMenuClose(provider.name);
    };

    const formatTokenDisplay = (provider: Provider) => {
        const tokenState = tokenStates[provider.name];
        const showToken = tokenState?.showToken || false;

        if (!provider.token) return 'Not set';
        if (showToken) return provider.token;
        if (provider.token.length <= 12) return provider.token; // If too short, show as is

        const prefix = provider.token.substring(0, 4);
        const suffix = provider.token.substring(provider.token.length - 4);
        return `${prefix}${'*'.repeat(4)}${suffix}`;
    };

    return (
        <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
            <Table>
                <TableHead>
                    <TableRow>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Status</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>Provider</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 200 }}>API Base</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>API Version</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>API Token</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Actions</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => (
                        <TableRow key={provider.name}>
                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <FormControlLabel
                                        control={
                                            <Switch
                                                checked={provider.enabled}
                                                onChange={() => onToggle?.(provider.name)}
                                                size="small"
                                                color="success"
                                            />
                                        }
                                        label=""
                                    />
                                    <Typography variant="body2" color={provider.enabled ? 'success.main' : 'error.main'}>
                                        {provider.enabled ? 'Enabled' : 'Disabled'}
                                    </Typography>
                                </Stack>
                            </TableCell>

                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    {provider.enabled ? (
                                        <CheckCircle color="success" fontSize="small" />
                                    ) : (
                                        <Cancel color="error" fontSize="small" />
                                    )}
                                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                                        {provider.name}
                                    </Typography>
                                </Stack>
                            </TableCell>

                            <TableCell>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontFamily: 'monospace',
                                        wordBreak: 'break-all',
                                        maxWidth: 200
                                    }}
                                >
                                    {provider.api_base}
                                </Typography>
                            </TableCell>

                            <TableCell>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontFamily: 'monospace'
                                    }}
                                >
                                    {provider.api_style || 'openai'}
                                </Typography>
                            </TableCell>

                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <Typography
                                        variant="body2"
                                        sx={{
                                            fontFamily: 'monospace',
                                            wordBreak: 'break-all',
                                            flex: 1,
                                            minWidth: 0
                                        }}
                                    >
                                        {formatTokenDisplay(provider)}
                                    </Typography>
                                    {provider.token && (
                                        <Stack direction="row" spacing={0.25}>
                                            <Tooltip title="View Token">
                                                <IconButton
                                                    size="small"
                                                    onClick={() => handleViewToken(provider.name)}
                                                    sx={{ p: 0.25 }}
                                                >
                                                    <Visibility fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title="Copy Token">
                                                <IconButton
                                                    size="small"
                                                    onClick={() => handleCopyToken(provider)}
                                                    sx={{ p: 0.25 }}
                                                >
                                                    <ContentCopy fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                            {/* <IconButton
                                                size="small"
                                                onClick={(e) => handleTokenMenuClick(e, provider.name)}
                                                sx={{ p: 0.25 }}
                                            >
                                                <Typography variant="caption" sx={{ fontSize: '0.7rem' }}>
                                                    •••
                                                </Typography>
                                            </IconButton> */}
                                        </Stack>
                                    )}
                                </Stack>
                            </TableCell>


                            <TableCell>
                                <Stack direction="row" spacing={0.5}>
                                    {onEdit && (
                                        <Tooltip title="Edit">
                                            <IconButton
                                                size="small"
                                                color="primary"
                                                onClick={() => onEdit(provider.name)}
                                            >
                                                <Edit fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    )}
                                    {onDelete && (
                                        <Tooltip title="Delete">
                                            <IconButton
                                                size="small"
                                                color="error"
                                                onClick={() => handleDeleteClick(provider.name)}
                                            >
                                                <Delete fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    )}
                                </Stack>
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>

            {/* Token context menus for each provider */}
            {Object.entries(tokenStates).map(([providerName, tokenState]) => (
                <Menu
                    key={providerName}
                    anchorEl={tokenState.anchor}
                    open={Boolean(tokenState.anchor)}
                    onClose={() => handleTokenMenuClose(providerName)}
                    anchorOrigin={{
                        vertical: 'bottom',
                        horizontal: 'right',
                    }}
                    transformOrigin={{
                        vertical: 'top',
                        horizontal: 'right',
                    }}
                >
                    <MenuItem onClick={() => handleViewToken(providerName)}>
                        <Visibility fontSize="small" sx={{ mr: 1 }} />
                        View Token
                    </MenuItem>
                    <MenuItem onClick={() => {
                        const provider = providers.find(p => p.name === providerName);
                        if (provider) handleCopyToken(provider);
                    }}>
                        <ContentCopy fontSize="small" sx={{ mr: 1 }} />
                        Copy Token
                    </MenuItem>
                </Menu>
            ))}

            {/* Token View Modal */}
            <Modal
                open={tokenModal.open}
                onClose={handleCloseTokenModal}
                aria-labelledby="token-modal-title"
                aria-describedby="token-modal-description"
            >
                <Box
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 600,
                        maxWidth: '80vw',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography id="token-modal-title" variant="h6" component="h2" sx={{ mb: 2 }}>
                        API Token - {tokenModal.providerName}
                    </Typography>

                    <Box sx={{ mb: 3 }}>
                        <Typography id="token-modal-description" variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                            Full token for {tokenModal.providerName}:
                        </Typography>

                        <Box
                            sx={{
                                p: 2,
                                bgcolor: 'grey.100',
                                borderRadius: 1,
                                fontFamily: 'monospace',
                                fontSize: '0.875rem',
                                wordBreak: 'break-all',
                                border: '1px solid',
                                borderColor: 'divider'
                            }}
                        >
                            {tokenModal.token}
                        </Box>
                    </Box>

                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <IconButton
                            color="primary"
                            onClick={async () => {
                                if (tokenModal.token) {
                                    try {
                                        await navigator.clipboard.writeText(tokenModal.token);
                                    } catch (err) {
                                        console.error('Failed to copy token:', err);
                                    }
                                }
                            }}
                            title="Copy Token"
                        >
                            <ContentCopy />
                        </IconButton>

                        <Tooltip title="Close">
                            <IconButton onClick={handleCloseTokenModal}>
                                <Cancel />
                            </IconButton>
                        </Tooltip>
                    </Stack>
                </Box>
            </Modal>

            {/* Delete Confirmation Modal */}
            <Modal
                open={deleteModal.open}
                onClose={handleCloseDeleteModal}
                aria-labelledby="delete-modal-title"
                aria-describedby="delete-modal-description"
            >
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
                    <Typography id="delete-modal-title" variant="h6" component="h2" sx={{ mb: 2 }}>
                        Delete Provider
                    </Typography>

                    <Typography id="delete-modal-description" variant="body2" sx={{ mb: 3 }}>
                        Are you sure you want to delete the provider "{deleteModal.providerName}"? This action cannot be undone.
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
        </TableContainer>
    );
};

export default ProviderTable;