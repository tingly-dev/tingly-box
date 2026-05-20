import {
    Box,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Button,
    Typography,
    Stack,
    Link,
    TextField,
    IconButton,
    CircularProgress,
    List,
    ListItem,
    ListItemText,
} from '@mui/material';
import React, { useState } from 'react';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import api from '@/services/api';

interface ClaudeDesktopConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    rules?: any[];
    onRulesRefresh?: () => void;
}

const MODEL_PREFIX = 'claude-';

const ClaudeDesktopConfigModal: React.FC<ClaudeDesktopConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    copyToClipboard,
    rules = [],
    onRulesRefresh,
}) => {
    const { token } = useScenarioPageModal();
    const [newModelName, setNewModelName] = useState('');
    const [adding, setAdding] = useState(false);
    const [deletingUuid, setDeletingUuid] = useState<string | null>(null);
    const [error, setError] = useState('');

    const modelRules = rules.filter(r => r.request_model && r.request_model !== '*');

    const validateModelName = (name: string): string => {
        if (!name.trim()) return 'Model name is required';
        if (!name.startsWith(MODEL_PREFIX)) return `Model name must start with "${MODEL_PREFIX}"`;
        if (modelRules.some(r => r.request_model === name.trim())) return 'Model already exists';
        return '';
    };

    const handleAdd = async () => {
        const trimmed = newModelName.trim();
        const validationError = validateModelName(trimmed);
        if (validationError) {
            setError(validationError);
            return;
        }
        setAdding(true);
        setError('');
        try {
            const result = await api.createRule('', {
                scenario: 'claude_desktop',
                request_model: trimmed,
                active: true,
                services: [],
            });
            if (result?.success) {
                setNewModelName('');
                onRulesRefresh?.();
            } else {
                setError(result?.error || 'Failed to add model');
            }
        } finally {
            setAdding(false);
        }
    };

    const handleDelete = async (uuid: string) => {
        setDeletingUuid(uuid);
        try {
            await api.deleteRule(uuid);
            onRulesRefresh?.();
        } finally {
            setDeletingUuid(null);
        }
    };

    const handleNewModelChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        setNewModelName(e.target.value);
        if (error) setError('');
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter') void handleAdd();
    };

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            PaperProps={{
                sx: { borderRadius: 3 }
            }}
        >
            <DialogTitle sx={{ pb: 1 }}>
                <Typography variant="h6" fontWeight={600}>
                    Configure Claude Desktop
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 1: Enable Developer Mode
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
                            Download Claude Desktop from <Link href="https://claude.com/download" target="_blank" underline="hover">claude.com/download</Link>
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            Launch the app, then enable developer mode:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 0.5, bgcolor: 'background.default', p: 1.5, borderRadius: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                Help → Troubleshooting → Enable Developer Mode
                            </Typography>
                        </Box>
                    </Box>

                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 2: Configure Third-Party Inference
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1.5 }}>
                            Go to:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 1.5, bgcolor: 'background.default', p: 1.5, borderRadius: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                Developer → Configure third-party inference
                            </Typography>
                        </Box>
                        <Typography variant="subtitle2" sx={{ mb: 1 }}>
                            In the configuration dialog:
                        </Typography>
                        <Box sx={{ pl: 2, mb: 0.5 }}>
                            <Typography variant="subtitle2">
                                <strong>Connection:</strong> Select "Gateway"
                            </Typography>
                            <Typography variant="subtitle2">
                                <strong>Gateway base URL:</strong>
                            </Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', mb: 1 }}>
                                {baseUrl}/tingly/claude_desktop
                            </Typography>
                            <Typography variant="subtitle2">
                                <strong>Gateway API key:</strong>
                            </Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                {token.slice(0, 16)}...
                            </Typography>
                        </Box>
                    </Box>

                    <Stack direction="row" spacing={1}>
                        <Button
                            variant="outlined"
                            size="small"
                            onClick={() => copyToClipboard(`${baseUrl}/tingly/claude_desktop`, 'URL')}
                            sx={{ flex: 1 }}
                        >
                            Copy URL
                        </Button>
                        <Button
                            variant="outlined"
                            size="small"
                            onClick={() => copyToClipboard(token, 'API Key')}
                            sx={{ flex: 1 }}
                        >
                            Copy API Key
                        </Button>
                    </Stack>

                    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                            Step 3: Configure Models
                        </Typography>
                        <Typography variant="body2" sx={{ mb: 1, color: 'text.secondary' }}>
                            Add these models in Claude Desktop's model picker. Model names must start with <code>claude-</code>.
                        </Typography>

                        {modelRules.length > 0 ? (
                            <List dense disablePadding sx={{ mb: 1 }}>
                                {modelRules.map((rule) => (
                                    <ListItem
                                        key={rule.uuid}
                                        disableGutters
                                        sx={{
                                            bgcolor: 'background.default',
                                            borderRadius: 1,
                                            mb: 0.5,
                                            px: 1.5,
                                        }}
                                        secondaryAction={
                                            <IconButton
                                                edge="end"
                                                size="small"
                                                onClick={() => handleDelete(rule.uuid)}
                                                disabled={deletingUuid === rule.uuid}
                                            >
                                                {deletingUuid === rule.uuid
                                                    ? <CircularProgress size={14} />
                                                    : <DeleteIcon fontSize="small" />
                                                }
                                            </IconButton>
                                        }
                                    >
                                        <ListItemText
                                            primary={rule.request_model}
                                            primaryTypographyProps={{ sx: { fontFamily: 'monospace', fontSize: '0.85rem' } }}
                                        />
                                    </ListItem>
                                ))}
                            </List>
                        ) : (
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1, fontStyle: 'italic' }}>
                                No models configured yet.
                            </Typography>
                        )}

                        <Stack direction="row" spacing={1} alignItems="flex-start">
                            <TextField
                                size="small"
                                placeholder="claude-sonnet-4-6"
                                value={newModelName}
                                onChange={handleNewModelChange}
                                onKeyDown={handleKeyDown}
                                error={Boolean(error)}
                                helperText={error}
                                disabled={adding}
                                sx={{ flex: 1 }}
                                inputProps={{ style: { fontFamily: 'monospace', fontSize: '0.85rem' } }}
                            />
                            <IconButton
                                color="primary"
                                onClick={() => void handleAdd()}
                                disabled={adding || !newModelName.trim()}
                                sx={{ mt: 0.5 }}
                            >
                                {adding ? <CircularProgress size={20} /> : <AddIcon />}
                            </IconButton>
                        </Stack>

                        <Typography variant="body2" sx={{ mt: 1, color: 'text.secondary' }}>
                            You can create multiple configurations and switch between them as needed.
                        </Typography>
                    </Box>
                </Stack>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
                <Button onClick={onClose} variant="contained">
                    Done
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ClaudeDesktopConfigModal;
