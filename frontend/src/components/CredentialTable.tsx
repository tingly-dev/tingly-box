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
import api from '../services/api';
import type { Provider, ProviderModelsData } from '../types/provider';


interface ProviderTableProps {
    providers: Provider[];
    providerModels?: ProviderModelsData;
    onEdit?: (providerUuid: string) => void;
    onToggle?: (providerUuid: string) => void;
    onDelete?: (providerUuid: string) => void;
    onSetDefault?: (providerUuid: string) => void;
    onFetchModels?: (providerUuid: string) => void;
}

interface TokenMenuState {
    anchor: HTMLElement | null;
    showToken: boolean;
}

interface TokenModalState {
    open: boolean;
    providerName: string;
    token: string;
    loading: boolean;
}

interface DeleteModalState {
    open: boolean;
    providerUuid: string;
    providerName: string;
}

const CredentialTable = ({
    providers,
    onEdit,
    onToggle,
    onDelete,
}: ProviderTableProps) => {
    const [tokenStates, setTokenStates] = useState<{ [key: string]: TokenMenuState }>({});
    const [tokenModal, setTokenModal] = useState<TokenModalState>({
        open: false,
        providerName: '',
        token: '',
        loading: false
    });
    const [deleteModal, setDeleteModal] = useState<DeleteModalState>({
        open: false,
        providerUuid: '',
        providerName: ''
    });

    // Function to fetch full token for a provider
    const fetchFullToken = async (providerUuid: string): Promise<string> => {
        try {
            const response = await api.getProvider(providerUuid)
            if (!response.success) {
                throw new Error(`Failed to fetch token for provider ${providerUuid}`);
            }
            const data = response.data;
            return data.token || '';
        } catch (error) {
            console.error('Error fetching full token:', error);
            throw error;
        }
    };

    const handleTokenMenuClick = (event: React.MouseEvent<HTMLElement>, providerUuid: string) => {
        setTokenStates(prev => ({
            ...prev,
            [providerUuid]: {
                anchor: event.currentTarget,
                showToken: prev[providerUuid]?.showToken || false
            }
        }));
    };

    const handleTokenMenuClose = (providerUuid: string) => {
        setTokenStates(prev => ({
            ...prev,
            [providerUuid]: {
                ...prev[providerUuid],
                anchor: null
            }
        }));
    };

    const handleViewToken = async (providerUuid: string) => {
        // Open modal with loading state
        setTokenModal({
            open: true,
            providerName: '', // Will be set later after we get provider data
            token: '',
            loading: true
        });

        try {
            // Fetch the full token from API
            const fullToken = await fetchFullToken(providerUuid);

            // Also fetch provider data to get the name for display
            const providerResponse = await api.getProvider(providerUuid);
            if (providerResponse.success) {
                setTokenModal(prev => ({
                    ...prev,
                    providerName: providerResponse.data.name,
                    token: fullToken,
                    loading: false
                }));
            } else {
                setTokenModal(prev => ({
                    ...prev,
                    token: fullToken,
                    loading: false
                }));
            }
        } catch (error) {
            console.error('Failed to fetch token:', error);
            // Update modal with error state
            setTokenModal(prev => ({
                ...prev,
                token: '',
                loading: false
            }));
        }
        handleTokenMenuClose(providerUuid);
    };

    const handleCloseTokenModal = () => {
        setTokenModal({
            open: false,
            providerName: '',
            token: '',
            loading: false
        });
    };

    const handleDeleteClick = async (providerUuid: string) => {
        // Get provider name for display in the delete modal
        const provider = providers.find(p => p.uuid === providerUuid);
        const providerName = provider?.name || 'Unknown Provider';

        setDeleteModal({
            open: true,
            providerUuid,
            providerName
        });
    };

    const handleCloseDeleteModal = () => {
        setDeleteModal({
            open: false,
            providerUuid: '',
            providerName: ''
        });
    };

    const handleConfirmDelete = () => {
        if (onDelete && deleteModal.providerUuid) {
            onDelete(deleteModal.providerUuid);
        }
        handleCloseDeleteModal();
    };

    const formatTokenDisplay = (provider: Provider) => {
        const tokenState = tokenStates[provider.uuid];
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
                        <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>Name</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>API Key</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 200 }}>API Base</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>API Style</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Actions</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Status</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => (
                        <TableRow key={provider.uuid}>
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
                                                    onClick={() => handleViewToken(provider.uuid)}
                                                    sx={{ p: 0.25 }}
                                                >
                                                    <Visibility fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                            {/* <IconButton
                                                size="small"
                                                onClick={(e) => handleTokenMenuClick(e, provider.uuid)}
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
                                <Stack direction="row" spacing={0.5}>
                                    {onEdit && (
                                        <Tooltip title="Edit">
                                            <IconButton
                                                size="small"
                                                color="primary"
                                                onClick={() => onEdit(provider.uuid)}
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
                                                onClick={() => handleDeleteClick(provider.uuid)}
                                            >
                                                <Delete fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    )}
                                </Stack>
                            </TableCell>
                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <FormControlLabel
                                        control={
                                            <Switch
                                                checked={provider.enabled}
                                                onChange={() => onToggle?.(provider.uuid)}
                                                size="small"
                                                color="success"
                                            />
                                        }
                                        label=""
                                    />
                                    <Typography variant="body2"
                                        color={provider.enabled ? 'success.main' : 'error.main'}>
                                        {provider.enabled ? 'Enabled' : 'Disabled'}
                                    </Typography>
                                </Stack>
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>

            {/* Token context menus for each provider */}
            {Object.entries(tokenStates).map(([providerUuid, tokenState]) => (
                <Menu
                    key={providerUuid}
                    anchorEl={tokenState.anchor}
                    open={Boolean(tokenState.anchor)}
                    onClose={() => handleTokenMenuClose(providerUuid)}
                    anchorOrigin={{
                        vertical: 'bottom',
                        horizontal: 'right',
                    }}
                    transformOrigin={{
                        vertical: 'top',
                        horizontal: 'right',
                    }}
                >
                    <MenuItem onClick={() => handleViewToken(providerUuid)}>
                        <Visibility fontSize="small" sx={{ mr: 1 }} />
                        View Token
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
                        API Key - {tokenModal.providerName}
                    </Typography>

                    {tokenModal.loading ? (
                        <Box sx={{ mb: 3, textAlign: 'center', py: 4 }}>
                            <Typography variant="body2" color="text.secondary">
                                Loading API key...
                            </Typography>
                        </Box>
                    ) : (
                        <Box sx={{ mb: 3 }}>
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
                                {tokenModal.token || 'Failed to load token'}
                            </Box>
                        </Box>
                    )}

                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <IconButton
                            color="primary"
                            disabled={tokenModal.loading || !tokenModal.token}
                            onClick={async () => {
                                if (tokenModal.token) {
                                    try {
                                        await navigator.clipboard.writeText(tokenModal.token);
                                    } catch (err) {
                                        console.error('Failed to copy token:', err);
                                    }
                                }
                            }}
                            title={tokenModal.loading ? "Loading..." : "Copy Token"}
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
                        Are you sure you want to delete the provider "{deleteModal.providerName}"? This action cannot
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
        </TableContainer>
    );
};

export default CredentialTable;