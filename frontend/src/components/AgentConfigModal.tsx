import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    TextField,
    Typography,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    Slider,
    Chip,
    Stack,
    Alert,
    CircularProgress,
} from '@mui/material';
import { useState, useEffect, useCallback } from 'react';
import type { AgentConfig } from '@/types/remoteGraph';

const DEFAULT_CONFIG: AgentConfig = {
    uuid: '',
    name: '',
    agent_type: 'claude-code',
    system_prompt: '',
    temperature: 0.7,
    max_tokens: 4096,
    tools: [],
    enabled: true,
};

interface AgentConfigModalProps {
    open: boolean;
    config: AgentConfig | null;
    onSave: (config: AgentConfig) => Promise<void>;
    onClose: () => void;
}

const AgentConfigModal: React.FC<AgentConfigModalProps> = ({
    open,
    config,
    onSave,
    onClose,
}) => {
    const [localConfig, setLocalConfig] = useState<AgentConfig>(DEFAULT_CONFIG);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        setLocalConfig(config ? { ...config } : { ...DEFAULT_CONFIG });
        setError(null);
    }, [config, open]);

    const handleChange = useCallback((field: keyof AgentConfig) => (
        event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
    ) => {
        setLocalConfig((prev: AgentConfig) => ({ ...prev, [field]: event.target.value }));
    }, []);

    const handleSliderChange = useCallback((field: 'temperature' | 'max_tokens') => (
        _event: Event | React.SyntheticEvent,
        value: number | number[]
    ) => {
        setLocalConfig((prev: AgentConfig) => ({
            ...prev,
            [field]: Array.isArray(value) ? value[0] : value,
        }));
    }, []);

    const handleSave = useCallback(async () => {
        if (!localConfig.name?.trim()) {
            setError('Agent name is required');
            return;
        }

        setSaving(true);
        setError(null);

        try {
            await onSave(localConfig);
            onClose();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to save agent configuration');
        } finally {
            setSaving(false);
        }
    }, [localConfig, onSave, onClose]);

    const removeTool = useCallback((tool: string) => {
        setLocalConfig((prev: AgentConfig) => ({
            ...prev,
            tools: prev.tools?.filter((t: string) => t !== tool) || [],
        }));
    }, []);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle>
                <Typography variant="h6">{config ? 'Edit Agent Configuration' : 'Create Agent Configuration'}</Typography>
            </DialogTitle>

            <DialogContent>
                {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}

                <Stack spacing={3} sx={{ mt: 1 }}>
                    <TextField
                        label="Agent Name"
                        fullWidth
                        value={localConfig.name}
                        onChange={handleChange('name')}
                        placeholder="e.g., Code Assistant"
                        required
                    />

                    <FormControl fullWidth>
                        <InputLabel>Agent Type</InputLabel>
                        <Select value={localConfig.agent_type} onChange={handleChange('agent_type')} label="Agent Type">
                            <MenuItem value="claude-code">Claude Code</MenuItem>
                            <MenuItem value="custom">Custom</MenuItem>
                            <MenuItem value="mock">Mock</MenuItem>
                        </Select>
                    </FormControl>

                    <TextField
                        label="System Prompt"
                        fullWidth
                        multiline
                        rows={4}
                        value={localConfig.system_prompt}
                        onChange={handleChange('system_prompt')}
                        placeholder="You are a helpful AI assistant..."
                        helperText="Optional: Define the agent's behavior and personality"
                    />

                    <Box>
                        <Typography variant="body2" gutterBottom>
                            Temperature: {(localConfig.temperature ?? 0.7).toFixed(2)}
                        </Typography>
                        <Slider
                            value={localConfig.temperature ?? 0.7}
                            onChange={handleSliderChange('temperature')}
                            min={0}
                            max={2}
                            step={0.1}
                            marks={[{ value: 0, label: '0' }, { value: 1, label: '1' }, { value: 2, label: '2' }]}
                        />
                        <Typography variant="caption" color="text.secondary">
                            Lower values make responses more focused, higher values more creative
                        </Typography>
                    </Box>

                    <Box>
                        <Typography variant="body2" gutterBottom>Max Tokens: {localConfig.max_tokens ?? 4096}</Typography>
                        <Slider
                            value={localConfig.max_tokens ?? 4096}
                            onChange={handleSliderChange('max_tokens')}
                            min={256}
                            max={32000}
                            step={256}
                            marks={[
                                { value: 256, label: '256' },
                                { value: 4096, label: '4K' },
                                { value: 8192, label: '8K' },
                                { value: 16384, label: '16K' },
                                { value: 32000, label: '32K' },
                            ]}
                        />
                        <Typography variant="caption" color="text.secondary">Maximum number of tokens in the response</Typography>
                    </Box>

                    <Box>
                        <Typography variant="body2" gutterBottom>Enabled Tools</Typography>
                        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                            {localConfig.tools?.length ? localConfig.tools.map(tool => (
                                <Chip key={tool} label={tool} onDelete={() => removeTool(tool)} />
                            )) : <Typography variant="caption" color="text.secondary">No tools enabled</Typography>}
                        </Stack>
                        <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                            Tools can be configured via the main settings page
                        </Typography>
                    </Box>
                </Stack>
            </DialogContent>

            <DialogActions>
                <Button onClick={onClose} disabled={saving}>Cancel</Button>
                <Button
                    onClick={handleSave}
                    variant="contained"
                    disabled={saving || !localConfig.name?.trim()}
                    startIcon={saving ? <CircularProgress size={16} /> : null}
                >
                    {saving ? 'Saving...' : 'Save Configuration'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default AgentConfigModal;
