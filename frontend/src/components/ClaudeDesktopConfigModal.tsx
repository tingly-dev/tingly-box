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
    Tooltip,
} from '@mui/material';
import React, { useState } from 'react';
import { Delete as DeleteIcon } from '@/components/icons';
import { Add as AddIcon } from '@/components/icons';
import { ContentCopy as ContentCopyIcon } from '@/components/icons';
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
const buildInferenceModelsJson = (modelRules: any[]): string => {
    const entries = modelRules.map(r => {
        const label = r.description?.startsWith('label:') ? r.description.slice(6) : '';
        if (label) {
            return `    {\n      "name": "${r.request_model}",\n      "labelOverride": "${label}"\n    }`;
        }
        return `    {\n      "name": "${r.request_model}"\n    }`;
    });
    return `"inferenceModels": [\n${entries.join(',\n')}\n  ]`;
};

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
    const [newLabelOverride, setNewLabelOverride] = useState('');
    const [adding, setAdding] = useState(false);
    const [deletingUuid, setDeletingUuid] = useState<string | null>(null);
    const [error, setError] = useState('');
    const [warning, setWarning] = useState('');

    const modelRules = rules.filter(r => r.request_model && r.request_model !== '*');
    const inferenceModelsJson = buildInferenceModelsJson(modelRules);

    const validateModelName = (name: string): { error: string; warning: string } => {
        if (!name.trim()) return { error: 'Model name is required', warning: '' };
        if (modelRules.some(r => r.request_model === name.trim())) return { error: 'Already exists', warning: '' };
        if (!name.startsWith(MODEL_PREFIX)) return { error: '', warning: 'Custom names may not work in Claude Desktop' };
        return { error: '', warning: '' };
    };

    const handleAdd = async () => {
        const trimmed = newModelName.trim();
        const { error: ve, warning: vw } = validateModelName(trimmed);
        if (ve) {
            setError(ve);
            return;
        }
        setWarning(vw);
        setAdding(true);
        setError('');
        try {
            const label = newLabelOverride.trim();
            const result = await api.createRule('', {
                scenario: 'claude_desktop',
                request_model: trimmed,
                description: label ? `label:${label}` : '',
                active: true,
                services: [],
            });
            if (result?.success) {
                setNewModelName('');
                setNewLabelOverride('');
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

    const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const val = e.target.value;
        setNewModelName(val);
        if (error) setError('');
        const { warning: vw } = validateModelName(val);
        setWarning(vw);
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
            PaperProps={{ sx: { borderRadius: 3 } }}
        >
            <DialogTitle sx={{ pb: 1 }}>
                <Typography variant="h6" fontWeight={600}>
                    Configure Claude Desktop
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ pt: 1 }}>
                <Stack spacing={2}>
                    {/* Step 1 */}
                    <Box sx={{ p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 1: Enable Developer Mode
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
                            Download Claude Desktop from{' '}
                            <Link href="https://claude.com/download" target="_blank" underline="hover">
                                claude.com/download
                            </Link>
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1 }}>
                            Launch the app, then enable developer mode:
                        </Typography>
                        <Box sx={{ bgcolor: 'background.default', p: 1.5, borderRadius: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                Help → Troubleshooting → Enable Developer Mode
                            </Typography>
                        </Box>
                    </Box>

                    {/* Step 2 */}
                    <Box sx={{ p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 2: Configure Third-Party Inference
                        </Typography>
                        <Box sx={{ bgcolor: 'background.default', p: 1.5, borderRadius: 1, mb: 1.5 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                Developer → Configure third-party inference
                            </Typography>
                        </Box>
                        <Box sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px', alignItems: 'baseline' }}>
                            <Typography variant="subtitle2"><strong>Connection:</strong></Typography>
                            <Typography variant="subtitle2">Gateway</Typography>
                            <Typography variant="subtitle2"><strong>Base URL:</strong></Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
                                {baseUrl}/tingly/claude_desktop
                            </Typography>
                            <Typography variant="subtitle2"><strong>API key:</strong></Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                                {token.slice(0, 16)}…
                            </Typography>
                        </Box>
                    </Box>

                    {/* Copy buttons */}
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

                    {/* Step 3 */}
                    <Box sx={{ p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1 }}>
                            <Typography variant="subtitle2" fontWeight={600}>
                                Step 3: Configure Models
                            </Typography>
                            {modelRules.length > 0 && (
                                <Tooltip title="Copy inferenceModels JSON">
                                    <IconButton
                                        size="small"
                                        onClick={() => copyToClipboard(inferenceModelsJson, 'inferenceModels')}
                                    >
                                        <ContentCopyIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            )}
                        </Stack>

                        <Typography variant="body2" color="text.secondary" sx={{ mb: 0.5 }}>
                            <strong>Optional</strong> — Claude Desktop will auto-discover models from{' '}
                            <code>/v1/models</code> if left empty.
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                            To pin a specific list, paste the JSON below into the <em>inferenceModels</em> field.
                            Custom model names may not be recognized by Claude Desktop.
                        </Typography>

                        {/* JSON preview */}
                        <Box
                            sx={{
                                bgcolor: 'background.default',
                                borderRadius: 1,
                                p: 1.5,
                                mb: 2,
                                fontFamily: 'monospace',
                                fontSize: '0.78rem',
                                lineHeight: 1.6,
                                whiteSpace: 'pre',
                                overflowX: 'auto',
                                color: modelRules.length === 0 ? 'text.disabled' : 'text.primary',
                            }}
                        >
                            {modelRules.length === 0
                                ? '"inferenceModels": []'
                                : inferenceModelsJson}
                        </Box>

                        {/* Per-row delete */}
                        <Stack spacing={0.5} sx={{ mb: 1.5 }}>
                            {modelRules.map(rule => {
                                const label = rule.description?.startsWith('label:')
                                    ? rule.description.slice(6)
                                    : '';
                                return (
                                    <Stack
                                        key={rule.uuid}
                                        direction="row"
                                        alignItems="center"
                                        spacing={1}
                                        sx={{
                                            bgcolor: 'background.default',
                                            borderRadius: 1,
                                            px: 1.5,
                                            py: 0.5,
                                        }}
                                    >
                                        <Typography
                                            sx={{ fontFamily: 'monospace', fontSize: '0.82rem', flex: 1 }}
                                        >
                                            {rule.request_model}
                                        </Typography>
                                        {label && (
                                            <Typography
                                                variant="caption"
                                                color="text.secondary"
                                                sx={{ fontFamily: 'monospace' }}
                                            >
                                                {label}
                                            </Typography>
                                        )}
                                        <IconButton
                                            size="small"
                                            onClick={() => handleDelete(rule.uuid)}
                                            disabled={deletingUuid === rule.uuid}
                                        >
                                            {deletingUuid === rule.uuid
                                                ? <CircularProgress size={14} />
                                                : <DeleteIcon fontSize="small" />
                                            }
                                        </IconButton>
                                    </Stack>
                                );
                            })}
                        </Stack>

                        {/* Add row */}
                        <Stack direction="row" spacing={1} alignItems="flex-start">
                            <TextField
                                size="small"
                                placeholder="claude-sonnet-4-6"
                                value={newModelName}
                                onChange={handleNameChange}
                                onKeyDown={handleKeyDown}
                                error={Boolean(error)}
                                helperText={error || warning}
                                FormHelperTextProps={{ sx: error ? {} : { color: 'warning.main' } }}
                                disabled={adding}
                                sx={{ flex: 2 }}
                                inputProps={{ style: { fontFamily: 'monospace', fontSize: '0.82rem' } }}
                            />
                            <TextField
                                size="small"
                                placeholder="label (optional)"
                                value={newLabelOverride}
                                onChange={e => setNewLabelOverride(e.target.value)}
                                onKeyDown={handleKeyDown}
                                disabled={adding}
                                sx={{ flex: 1 }}
                                inputProps={{ style: { fontSize: '0.82rem' } }}
                            />
                            <IconButton
                                color="primary"
                                onClick={() => void handleAdd()}
                                disabled={adding || !newModelName.trim()}
                                sx={{ mt: error ? 0 : 0.5 }}
                            >
                                {adding ? <CircularProgress size={20} /> : <AddIcon />}
                            </IconButton>
                        </Stack>
                    </Box>
                </Stack>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
                <Button onClick={onClose} variant="contained">Done</Button>
            </DialogActions>
        </Dialog>
    );
};

export default ClaudeDesktopConfigModal;
