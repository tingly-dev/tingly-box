import {
    Alert,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { Visibility, VisibilityOff } from '@/components/icons';
import CopyIconButton from '@/components/CopyIconButton';

export type CredentialEditorState = {
    name: string;
    type: 'api_key' | 'token' | 'private_key';
    secret: string;
    aliasToken: string;
    secretMask: string;
    currentSecret: string;
};

type CredentialEditorDialogProps = {
    open: boolean;
    editingCredentialId: string | null;
    editorState: CredentialEditorState;
    editorMessage: { type: 'success' | 'error'; text: string } | null;
    editorLoading: boolean;
    saving: boolean;
    showCurrentSecret: boolean;
    onEditorStateChange: (updater: (state: CredentialEditorState) => CredentialEditorState) => void;
    onToggleShowCurrentSecret: () => void;
    onClose: () => void;
    onSave: () => void;
};

// The editor message is rendered inline (not a toast) on purpose: save/load
// failures must stay visible next to the fields they belong to (see P1 history).
const CredentialEditorDialog = ({
    open,
    editingCredentialId,
    editorState,
    editorMessage,
    editorLoading,
    saving,
    showCurrentSecret,
    onEditorStateChange,
    onToggleShowCurrentSecret,
    onClose,
    onSave,
}: CredentialEditorDialogProps) => (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm" disableRestoreFocus>
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
                    onChange={(event) => onEditorStateChange((state) => ({ ...state, name: event.target.value }))}
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
                                        <CopyIconButton
                                            value={editorState.aliasToken}
                                            label="Copy alias token"
                                            edge="end"
                                        />
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
                                            onClick={onToggleShowCurrentSecret}
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
                                    onClick={() => onEditorStateChange((state) => ({ ...state, type: option.value as CredentialEditorState['type'] }))}
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
                                        <Typography variant="body2" sx={{
                                            fontWeight: 600
                                        }}>
                                            {option.label}
                                        </Typography>
                                        <Typography variant="caption" sx={{
                                            color: "text.secondary"
                                        }}>
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
                    onChange={(event) => onEditorStateChange((state) => ({ ...state, secret: event.target.value }))}
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
            <Button onClick={onClose}>Cancel</Button>
            <Button variant="contained" disabled={saving || editorLoading} onClick={onSave}>Save</Button>
        </DialogActions>
    </Dialog>
);

export default CredentialEditorDialog;
