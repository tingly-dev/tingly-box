import { useEffect, useMemo, useState } from 'react';
import {
    Alert,
    Box,
    Button,
    Checkbox,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Grid,
    IconButton,
    Paper,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TextField,
    Typography,
} from '@mui/material';
import {
    Add,
    ContentCopy,
    DeleteOutline,
    Refresh as RefreshIcon,
    Shield,
    Visibility,
    VisibilityOff,
    VpnKey,
    CheckCircleOutline,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

type ProtectedCredential = {
    id: string;
    name: string;
    type: 'api_key' | 'token' | 'private_key';
    alias_token: string;
    description?: string;
    tags?: string[];
    enabled: boolean;
    secret_mask: string;
};

type ImportableProvider = {
    uuid: string;
    name: string;
    auth_type?: string;
    token?: string;
    oauth_detail?: {
        access_token?: string;
    };
    enabled?: boolean;
};

type CredentialEditorState = {
    name: string;
    type: 'api_key' | 'token' | 'private_key';
    secret: string;
    aliasToken: string;
    secretMask: string;
    currentSecret: string;
};

const emptyEditorState: CredentialEditorState = {
    name: '',
    type: 'token',
    secret: '',
    aliasToken: '',
    secretMask: '',
    currentSecret: '',
};

const GuardrailsCredentialsPage = () => {
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [credentials, setCredentials] = useState<ProtectedCredential[]>([]);
    const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [editorOpen, setEditorOpen] = useState(false);
    const [importDialogOpen, setImportDialogOpen] = useState(false);
    const [editingCredentialId, setEditingCredentialId] = useState<string | null>(null);
    const [deleteCredentialId, setDeleteCredentialId] = useState<string | null>(null);
    const [deleteSelectedOpen, setDeleteSelectedOpen] = useState(false);
    const [pendingSave, setPendingSave] = useState(false);
    const [pendingImport, setPendingImport] = useState(false);
    const [editorLoading, setEditorLoading] = useState(false);
    const [showCurrentSecret, setShowCurrentSecret] = useState(false);
    const [importableProviders, setImportableProviders] = useState<ImportableProvider[]>([]);
    const [selectedProviderIDs, setSelectedProviderIDs] = useState<string[]>([]);
    const [editorState, setEditorState] = useState<CredentialEditorState>(emptyEditorState);
    const [editorMessage, setEditorMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

    const selectedCredentials = useMemo(
        () => credentials.filter((credential) => selectedIDs.includes(credential.id)),
        [credentials, selectedIDs]
    );

    // Dialog-trigger buttons can keep MUI focus styling after the pointer leaves.
    // Blur the active element on open/close paths so toolbar actions return to a neutral state.
    const blurActiveElement = () => {
        const active = document.activeElement;
        if (active instanceof HTMLElement) {
            active.blur();
        }
    };

    const loadCredentials = async () => {
        try {
            setLoading(true);
            const result = await api.getGuardrailsCredentials();
            if (Array.isArray(result?.data)) {
                setCredentials(result.data);
                setActionMessage(null);
                return;
            }
            setCredentials([]);
            setActionMessage({ type: 'error', text: result?.error || 'Failed to load protected credentials.' });
        } catch (error: any) {
            setCredentials([]);
            setActionMessage({ type: 'error', text: error?.message || 'Failed to load protected credentials.' });
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadCredentials();
    }, []);

    const loadImportableProviders = async () => {
        const result = await api.getProviders();
        if (!result?.success || !Array.isArray(result?.data)) {
            setActionMessage({ type: 'error', text: result?.error || 'Failed to load credentials.' });
            return;
        }
        const importable = result.data.filter((provider: ImportableProvider) => {
            const secret = provider.auth_type === 'oauth' ? provider.oauth_detail?.access_token : provider.token;
            return !!secret;
        });
        setImportableProviders(importable);
    };

    const openImportDialog = async () => {
        blurActiveElement();
        setSelectedProviderIDs([]);
        setImportDialogOpen(true);
        await loadImportableProviders();
    };

    const openNewDialog = () => {
        blurActiveElement();
        setEditingCredentialId(null);
        setEditorState(emptyEditorState);
        setEditorMessage(null);
        setShowCurrentSecret(false);
        setEditorOpen(true);
    };

    const openEditDialog = async (credential: ProtectedCredential) => {
        blurActiveElement();
        setEditingCredentialId(credential.id);
        setEditorState(emptyEditorState);
        setEditorMessage(null);
        setShowCurrentSecret(false);
        setEditorOpen(true);
        setEditorLoading(true);
        try {
            const result = await api.getGuardrailsCredential(credential.id);
            const detail = result?.data;
            if (!result?.success || !detail) {
                setEditorMessage({ type: 'error', text: result?.error || 'Failed to load protected credential.' });
                return;
            }
            setEditorState({
                name: detail.name || credential.name,
                type: detail.type || credential.type,
                secret: '',
                aliasToken: detail.alias_token || credential.alias_token,
                secretMask: detail.secret_mask || credential.secret_mask,
                currentSecret: detail.secret || '',
            });
        } catch (error: any) {
            setEditorMessage({ type: 'error', text: error?.message || 'Failed to load protected credential.' });
        } finally {
            setEditorLoading(false);
        }
    };

    const handleSaveCredential = async () => {
        if (!editorState.name.trim()) {
            setEditorMessage({ type: 'error', text: 'Credential name is required.' });
            return;
        }
        if (!editingCredentialId && !editorState.secret.trim()) {
            setEditorMessage({ type: 'error', text: 'Credential secret is required.' });
            return;
        }

        const existingCredential = editingCredentialId ? credentials.find((credential) => credential.id === editingCredentialId) : null;
        const payload = {
            name: editorState.name,
            type: editorState.type,
            secret: editorState.secret,
            description: existingCredential?.description || '',
            tags: existingCredential?.tags || [],
            enabled: existingCredential?.enabled ?? true,
        };

        try {
            setPendingSave(true);
            const result = editingCredentialId
                ? await api.updateGuardrailsCredential(editingCredentialId, payload)
                : await api.createGuardrailsCredential(payload);
            if (!result?.success) {
                setEditorMessage({ type: 'error', text: result?.error || 'Failed to save protected credential.' });
                return;
            }
            setEditorOpen(false);
            setEditorMessage(null);
            await loadCredentials();
            setActionMessage({ type: 'success', text: 'Protected credential saved.' });
        } catch (error: any) {
            setEditorMessage({ type: 'error', text: error?.message || 'Failed to save protected credential.' });
        } finally {
            setPendingSave(false);
        }
    };

    const handleDeleteCredential = async () => {
        if (!deleteCredentialId) {
            return;
        }
        try {
            const result = await api.deleteGuardrailsCredential(deleteCredentialId);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to delete protected credential.' });
                return;
            }
            setDeleteCredentialId(null);
            setSelectedIDs((current) => current.filter((id) => id !== deleteCredentialId));
            await loadCredentials();
            setActionMessage({ type: 'success', text: 'Protected credential deleted.' });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to delete protected credential.' });
        }
    };

    const handleDeleteSelectedCredentials = async () => {
        if (selectedIDs.length === 0) {
            return;
        }
        try {
            for (const credentialID of selectedIDs) {
                const result = await api.deleteGuardrailsCredential(credentialID);
                if (!result?.success) {
                    setActionMessage({ type: 'error', text: result?.error || 'Failed to delete protected credentials.' });
                    return;
                }
            }
            setDeleteSelectedOpen(false);
            setSelectedIDs([]);
            await loadCredentials();
            setActionMessage({
                type: 'success',
                text: selectedIDs.length === 1 ? 'Deleted 1 protected credential.' : `Deleted ${selectedIDs.length} protected credentials.`,
            });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to delete protected credentials.' });
        }
    };

    const handleToggleCredential = async (credential: ProtectedCredential, enabled: boolean) => {
        const result = await api.updateGuardrailsCredential(credential.id, {
            name: credential.name,
            type: credential.type,
            description: credential.description || '',
            tags: credential.tags || [],
            enabled,
        });
        if (!result?.success) {
            setActionMessage({ type: 'error', text: result?.error || 'Failed to update protected credential.' });
            return;
        }
        await loadCredentials();
    };

    const handleImportProviders = async () => {
        if (selectedProviderIDs.length === 0) {
            setActionMessage({ type: 'error', text: 'Select at least one credential to import.' });
            return;
        }

        try {
            setPendingImport(true);
            let imported = 0;
            for (const providerID of selectedProviderIDs) {
                const provider = importableProviders.find((item) => item.uuid === providerID);
                if (!provider) {
                    continue;
                }
                const secret = provider.auth_type === 'oauth' ? provider.oauth_detail?.access_token : provider.token;
                if (!secret) {
                    continue;
                }
                const payload = {
                    name: provider.name,
                    type: provider.auth_type === 'oauth' ? 'token' : 'api_key',
                    secret,
                    enabled: provider.enabled ?? true,
                };
                const result = await api.createGuardrailsCredential(payload);
                if (!result?.success) {
                    setActionMessage({ type: 'error', text: result?.error || `Failed to import ${provider.name}.` });
                    return;
                }
                imported += 1;
            }
            setImportDialogOpen(false);
            await loadCredentials();
            setActionMessage({
                type: 'success',
                text: imported === 1 ? 'Imported 1 credential from Credentials.' : `Imported ${imported} credentials from Credentials.`,
            });
        } finally {
            setPendingImport(false);
        }
    };

    const handleCreateMaskPolicyDraft = () => {
        if (selectedCredentials.length === 0) {
            return;
        }
        blurActiveElement();
        navigate('/guardrails/rules', {
            state: {
                newPolicyDraft: {
                    name: selectedCredentials.length === 1 ? `Mask ${selectedCredentials[0].name}` : 'Mask Protected Credentials',
                    kind: 'content',
                    enabled: true,
                    verdict: 'mask',
                    reason:
                        selectedCredentials.length === 1
                            ? `Replace ${selectedCredentials[0].name} with its alias token before content reaches the model.`
                            : 'Replace protected credentials with alias tokens before content reaches the model.',
                    credentialRefs: selectedCredentials.map((credential) => credential.id),
                    patterns: '',
                },
            },
        });
    };

    const toggleSelected = (credentialId: string, checked: boolean) => {
        setSelectedIDs((current) => {
            if (checked) {
                return Array.from(new Set([...current, credentialId]));
            }
            return current.filter((id) => id !== credentialId);
        });
    };

    const allSelected = credentials.length > 0 && selectedIDs.length === credentials.length;
    const enabledCount = credentials.filter((credential) => credential.enabled).length;
    const allImportableSelected = importableProviders.length > 0 && selectedProviderIDs.length === importableProviders.length;

    const handleCopyAliasToken = async () => {
        if (!editorState.aliasToken) {
            return;
        }
        try {
            await navigator.clipboard.writeText(editorState.aliasToken);
            setEditorMessage({ type: 'success', text: 'Alias token copied.' });
        } catch {
            setEditorMessage({ type: 'error', text: 'Failed to copy alias token.' });
        }
    };

    const handleCloseEditor = () => {
        blurActiveElement();
        setEditorOpen(false);
        setEditorLoading(false);
        setShowCurrentSecret(false);
        setEditorState(emptyEditorState);
        setEditorMessage(null);
    };

    return (
        <PageLayout
            loading={loading}
            title="Protected Credentials"
            subtitle="Keep real secrets local, give the model alias tokens, and generate mask policies from selected credentials."
            rightAction={
                <Stack direction="row" spacing={1}>
                    <Button variant="outlined" startIcon={<RefreshIcon />} onClick={loadCredentials}>
                        Reload
                    </Button>
                </Stack>
            }
        >
            <Stack spacing={3}>
                {actionMessage && <Alert severity={actionMessage.type}>{actionMessage.text}</Alert>}

                <Grid container spacing={2}>
                    <Grid size={{ xs: 12, sm: 4 }}>
                        <UnifiedCard title="Total" size="full">
                            <Stack direction="row" spacing={1.5} alignItems="center">
                                <VpnKey color="primary" />
                                <Box>
                                    <Typography variant="h4" sx={{ fontWeight: 600 }}>{credentials.length}</Typography>
                                    <Typography variant="body2" color="text.secondary">Protected credentials</Typography>
                                </Box>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 4 }}>
                        <UnifiedCard title="Enabled" size="full">
                            <Stack direction="row" spacing={1.5} alignItems="center">
                                <CheckCircleOutline color="success" />
                                <Box>
                                    <Typography variant="h4" sx={{ fontWeight: 600 }}>{enabledCount}</Typography>
                                    <Typography variant="body2" color="text.secondary">Active for masking</Typography>
                                </Box>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 4 }}>
                        <UnifiedCard title="Selected" size="full">
                            <Stack direction="row" spacing={1.5} alignItems="center">
                                <Shield color="warning" />
                                <Box>
                                    <Typography variant="h4" sx={{ fontWeight: 600 }}>{selectedCredentials.length}</Typography>
                                    <Typography variant="body2" color="text.secondary">Ready for one mask policy</Typography>
                                </Box>
                            </Stack>
                        </UnifiedCard>
                    </Grid>
                </Grid>

                <UnifiedCard
                    title="Protected Credentials"
                    subtitle="Add sensitive credentials here when you do not want the model to see them directly."
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            {credentials.length > 0 && (
                                <Button
                                    variant="outlined"
                                    startIcon={<Shield />}
                                    disabled={selectedCredentials.length === 0}
                                    onClick={handleCreateMaskPolicyDraft}
                                >
                                    Create Mask Policy
                                </Button>
                            )}
                            {selectedCredentials.length > 0 && (
                                <Button
                                    variant="outlined"
                                    color="error"
                                    startIcon={<DeleteOutline />}
                                    onClick={() => {
                                        blurActiveElement();
                                        setDeleteSelectedOpen(true);
                                    }}
                                >
                                    Delete Selected
                                </Button>
                            )}
                            <Button variant="outlined" onClick={openImportDialog}>
                                Import from Credentials
                            </Button>
                            <Button variant="contained" startIcon={<Add />} onClick={openNewDialog}>
                                Add Credential
                            </Button>
                        </Stack>
                    }
                >
                    <Alert severity="info" sx={{ mb: 2 }}>
                        Protected credentials are replaced with alias tokens before content reaches the model. When a tool needs the real value, Tingly restores it locally at execution time.
                    </Alert>
                    {credentials.length === 0 ? (
                        <Box sx={{ py: 8, textAlign: 'center' }}>
                            <Stack spacing={2} alignItems="center">
                                <Box
                                    sx={{
                                        width: 72,
                                        height: 72,
                                        borderRadius: 2,
                                        bgcolor: 'primary.main',
                                        color: 'primary.contrastText',
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                    }}
                                >
                                    <VpnKey sx={{ fontSize: 34 }} />
                                </Box>
                                <Stack spacing={1} alignItems="center">
                                    <Typography variant="h5" sx={{ fontWeight: 600 }}>
                                        No protected credentials
                                    </Typography>
                                    <Typography variant="body1" color="text.secondary" sx={{ maxWidth: 560 }}>
                                        Add credentials here before wiring pseudonymization into policies.
                                    </Typography>
                                </Stack>
                                <Button variant="contained" startIcon={<Add />} onClick={openNewDialog}>
                                    Add Credential
                                </Button>
                                <Button variant="outlined" onClick={openImportDialog}>
                                    Import from Credentials
                                </Button>
                            </Stack>
                        </Box>
                    ) : (
                        <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
                            <Table>
                                <TableHead>
                                    <TableRow>
                                        <TableCell padding="checkbox">
                                            <Checkbox
                                                checked={allSelected}
                                                indeterminate={selectedIDs.length > 0 && !allSelected}
                                                onChange={(event) => setSelectedIDs(event.target.checked ? credentials.map((item) => item.id) : [])}
                                            />
                                        </TableCell>
                                        <TableCell sx={{ fontWeight: 600, minWidth: 180 }}>Name</TableCell>
                                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Type</TableCell>
                                        <TableCell sx={{ fontWeight: 600, minWidth: 220 }}>Alias Token</TableCell>
                                        <TableCell sx={{ fontWeight: 600, minWidth: 180 }}>Stored Secret</TableCell>
                                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Status</TableCell>
                                        <TableCell sx={{ fontWeight: 600, minWidth: 110 }}>Actions</TableCell>
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {credentials.map((credential) => (
                                        <TableRow
                                            key={credential.id}
                                            hover
                                            selected={selectedIDs.includes(credential.id)}
                                            onClick={() => openEditDialog(credential)}
                                            sx={{ cursor: 'pointer' }}
                                        >
                                            <TableCell padding="checkbox" onClick={(event) => event.stopPropagation()}>
                                                <Checkbox
                                                    checked={selectedIDs.includes(credential.id)}
                                                    onChange={(event) => toggleSelected(credential.id, event.target.checked)}
                                                />
                                            </TableCell>
                                            <TableCell>
                                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                    {credential.name}
                                                </Typography>
                                            </TableCell>
                                            <TableCell>
                                                <Chip size="small" label={credential.type.replace('_', ' ')} variant="outlined" />
                                            </TableCell>
                                            <TableCell>
                                                <Typography variant="caption" sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
                                                    {credential.alias_token}
                                                </Typography>
                                            </TableCell>
                                            <TableCell>
                                                <Typography variant="caption" sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
                                                    {credential.secret_mask}
                                                </Typography>
                                            </TableCell>
                                            <TableCell onClick={(event) => event.stopPropagation()}>
                                                <Stack direction="row" alignItems="center" spacing={0.75}>
                                                    <Switch
                                                        size="small"
                                                        checked={credential.enabled}
                                                        onChange={(event) => handleToggleCredential(credential, event.target.checked)}
                                                    />
                                                    <Typography variant="body2" color={credential.enabled ? 'success.main' : 'text.secondary'}>
                                                        {credential.enabled ? 'Enabled' : 'Disabled'}
                                                    </Typography>
                                                </Stack>
                                            </TableCell>
                                            <TableCell onClick={(event) => event.stopPropagation()}>
                                                <Button
                                                    color="error"
                                                    size="small"
                                                    startIcon={<DeleteOutline />}
                                                    onClick={() => setDeleteCredentialId(credential.id)}
                                                >
                                                    Delete
                                                </Button>
                                            </TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        </TableContainer>
                    )}
                </UnifiedCard>
            </Stack>

            <Dialog open={editorOpen} onClose={handleCloseEditor} fullWidth maxWidth="sm" disableRestoreFocus>
                <DialogTitle>{editingCredentialId ? 'Edit Protected Credential' : 'New Protected Credential'}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        {editorMessage && <Alert severity={editorMessage.type}>{editorMessage.text}</Alert>}
                        <Alert severity="info">
                            The real secret is hidden in the UI. When editing an existing credential, leave the secret field empty to keep the current value.
                        </Alert>
                        <TextField
                            label="Name"
                            value={editorState.name}
                            onChange={(event) => setEditorState((state) => ({ ...state, name: event.target.value }))}
                            fullWidth
                            required
                            size="small"
                            disabled={editorLoading}
                        />
                        {editingCredentialId && (
                            <Stack spacing={1.5}>
                                <TextField
                                    label="Alias Token"
                                    value={editorState.aliasToken}
                                    fullWidth
                                    size="small"
                                    slotProps={{
                                        input: {
                                            readOnly: true,
                                            endAdornment: (
                                                <Button
                                                    size="small"
                                                    startIcon={<ContentCopy fontSize="small" />}
                                                    onClick={handleCopyAliasToken}
                                                    sx={{ minWidth: 'auto', ml: 1 }}
                                                >
                                                    Copy
                                                </Button>
                                            ),
                                        },
                                    }}
                                />
                                <TextField
                                    label="Current Secret"
                                    value={showCurrentSecret ? editorState.currentSecret : editorState.secretMask}
                                    fullWidth
                                    size="small"
                                    type={showCurrentSecret ? 'text' : 'password'}
                                    slotProps={{
                                        input: {
                                            readOnly: true,
                                            endAdornment: (
                                                <IconButton
                                                    edge="end"
                                                    onClick={() => setShowCurrentSecret((current) => !current)}
                                                    size="small"
                                                >
                                                    {showCurrentSecret ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                                </IconButton>
                                            ),
                                        },
                                    }}
                                />
                            </Stack>
                        )}
                        <Box>
                            <Typography variant="subtitle2" sx={{ mb: 1 }}>
                                Credential Type
                            </Typography>
                            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                                {[
                                    { value: 'api_key', label: 'API Key', description: 'Static API keys and service secrets.' },
                                    { value: 'token', label: 'Token', description: 'Bearer tokens, session tokens, and access tokens.' },
                                    { value: 'private_key', label: 'Private Key', description: 'Multi-line private keys and PEM content.' },
                                ].map((option) => {
                                    const selected = editorState.type === option.value;
                                    return (
                                        <Box
                                            key={option.value}
                                            onClick={() => setEditorState((state) => ({ ...state, type: option.value as CredentialEditorState['type'] }))}
                                            sx={{
                                                flex: 1,
                                                border: '1px solid',
                                                borderColor: selected ? 'primary.main' : 'divider',
                                                borderRadius: 2,
                                                p: 1.5,
                                                cursor: 'pointer',
                                                bgcolor: selected ? 'action.selected' : 'transparent',
                                            }}
                                        >
                                            <Stack spacing={0.5}>
                                                <Typography variant="body2" fontWeight={600}>
                                                    {option.label}
                                                </Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    {option.description}
                                                </Typography>
                                            </Stack>
                                        </Box>
                                    );
                                })}
                            </Stack>
                        </Box>
                        <TextField
                            label={editingCredentialId ? 'Secret (leave empty to keep current value)' : 'Secret'}
                            value={editorState.secret}
                            onChange={(event) => setEditorState((state) => ({ ...state, secret: event.target.value }))}
                            fullWidth
                            multiline
                            minRows={editorState.type === 'private_key' ? 4 : 2}
                            type="password"
                            size="small"
                            disabled={editorLoading}
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={handleCloseEditor}>Cancel</Button>
                    <Button variant="contained" disabled={pendingSave || editorLoading} onClick={handleSaveCredential}>Save</Button>
                </DialogActions>
            </Dialog>

            <Dialog
                open={!!deleteCredentialId}
                onClose={() => {
                    setDeleteCredentialId(null);
                    blurActiveElement();
                }}
                disableRestoreFocus
            >
                <DialogTitle>Delete Protected Credential</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        Delete this protected credential? Existing mask policies will keep their credential refs and need to be updated manually.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button
                        onClick={() => {
                            setDeleteCredentialId(null);
                            blurActiveElement();
                        }}
                    >
                        Cancel
                    </Button>
                    <Button color="error" variant="contained" onClick={handleDeleteCredential}>Delete</Button>
                </DialogActions>
            </Dialog>

            <Dialog
                open={deleteSelectedOpen}
                onClose={() => {
                    setDeleteSelectedOpen(false);
                    blurActiveElement();
                }}
                disableRestoreFocus
            >
                <DialogTitle>Delete Selected Credentials</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        {selectedIDs.length === 1
                            ? 'Delete the selected protected credential? Existing mask policies will keep their credential refs and need to be updated manually.'
                            : `Delete ${selectedIDs.length} selected protected credentials? Existing mask policies will keep their credential refs and need to be updated manually.`}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button
                        onClick={() => {
                            setDeleteSelectedOpen(false);
                            blurActiveElement();
                        }}
                    >
                        Cancel
                    </Button>
                    <Button color="error" variant="contained" onClick={handleDeleteSelectedCredentials}>
                        Delete
                    </Button>
                </DialogActions>
            </Dialog>

            <Dialog
                open={importDialogOpen}
                onClose={() => {
                    if (!pendingImport) {
                        setImportDialogOpen(false);
                        blurActiveElement();
                    }
                }}
                fullWidth
                maxWidth="sm"
                disableRestoreFocus
            >
                <DialogTitle>Import from Credentials</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        <Alert severity="info">
                            Import existing credentials from the main Credentials page into Guardrails protection. The imported value will be stored locally as a protected credential with its own alias token.
                        </Alert>
                        {importableProviders.length === 0 ? (
                            <Typography variant="body2" color="text.secondary">
                                No importable credentials found.
                            </Typography>
                        ) : (
                            <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
                                <Table size="small">
                                    <TableHead>
                                        <TableRow>
                                            <TableCell padding="checkbox">
                                                <Checkbox
                                                    checked={allImportableSelected}
                                                    indeterminate={selectedProviderIDs.length > 0 && !allImportableSelected}
                                                    onChange={(event) =>
                                                        setSelectedProviderIDs(
                                                            event.target.checked ? importableProviders.map((provider) => provider.uuid) : []
                                                        )
                                                    }
                                                />
                                            </TableCell>
                                            <TableCell sx={{ fontWeight: 600 }}>Name</TableCell>
                                            <TableCell sx={{ fontWeight: 600 }}>Type</TableCell>
                                        </TableRow>
                                    </TableHead>
                                    <TableBody>
                                        {importableProviders.map((provider) => {
                                            const selected = selectedProviderIDs.includes(provider.uuid);
                                            return (
                                                <TableRow
                                                    key={provider.uuid}
                                                    hover
                                                    selected={selected}
                                                    onClick={() =>
                                                        setSelectedProviderIDs((current) =>
                                                            current.includes(provider.uuid)
                                                                ? current.filter((id) => id !== provider.uuid)
                                                                : [...current, provider.uuid]
                                                        )
                                                    }
                                                    sx={{ cursor: 'pointer' }}
                                                >
                                                    <TableCell padding="checkbox">
                                                        <Checkbox checked={selected} />
                                                    </TableCell>
                                                    <TableCell>{provider.name}</TableCell>
                                                    <TableCell>
                                                        <Chip
                                                            size="small"
                                                            label={provider.auth_type === 'oauth' ? 'token' : 'api key'}
                                                            variant="outlined"
                                                        />
                                                    </TableCell>
                                                </TableRow>
                                            );
                                        })}
                                    </TableBody>
                                </Table>
                            </TableContainer>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button
                        onClick={() => {
                            setImportDialogOpen(false);
                            blurActiveElement();
                        }}
                        disabled={pendingImport}
                    >
                        Cancel
                    </Button>
                    <Button variant="contained" onClick={handleImportProviders} disabled={pendingImport || selectedProviderIDs.length === 0}>
                        Import
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default GuardrailsCredentialsPage;
