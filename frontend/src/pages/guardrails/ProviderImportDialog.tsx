import {
    Alert,
    Button,
    Chip,
    Checkbox,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Paper,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
} from '@mui/material';

export type ImportableProvider = {
    uuid: string;
    name: string;
    auth_type?: string;
    token?: string;
    oauth_detail?: {
        access_token?: string;
    };
    enabled?: boolean;
};

type ProviderImportDialogProps = {
    open: boolean;
    pendingImport: boolean;
    importableProviders: ImportableProvider[];
    selectedProviderIDs: string[];
    onToggleAll: (checked: boolean) => void;
    onToggleProvider: (uuid: string) => void;
    onClose: () => void;
    onCancel: () => void;
    onImport: () => void;
};

const ProviderImportDialog = ({
    open,
    pendingImport,
    importableProviders,
    selectedProviderIDs,
    onToggleAll,
    onToggleProvider,
    onClose,
    onCancel,
    onImport,
}: ProviderImportDialogProps) => {
    const allSelected = importableProviders.length > 0 && selectedProviderIDs.length === importableProviders.length;

    return (
        <Dialog
            open={open}
            onClose={onClose}
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
                        <Typography variant="body2" sx={{
                            color: "text.secondary"
                        }}>
                            No importable credentials found.
                        </Typography>
                    ) : (
                        <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
                            <Table size="small">
                                <TableHead>
                                    <TableRow>
                                        <TableCell padding="checkbox">
                                            <Checkbox
                                                checked={allSelected}
                                                indeterminate={selectedProviderIDs.length > 0 && !allSelected}
                                                onChange={(event) => onToggleAll(event.target.checked)}
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
                                                onClick={() => onToggleProvider(provider.uuid)}
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
                <Button onClick={onCancel} disabled={pendingImport}>
                    Cancel
                </Button>
                <Button variant="contained" onClick={onImport} disabled={pendingImport || selectedProviderIDs.length === 0}>
                    Import
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ProviderImportDialog;
