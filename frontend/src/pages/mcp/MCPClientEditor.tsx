import {
    Box,
    Button,
    Checkbox,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
    Alert,
    Tabs,
    Tab,
} from '@mui/material';
import {
    Add as AddIcon,
    DeleteOutline as DeleteOutlineIcon,
} from '@/components/icons';
import { useEffect, useState } from 'react';
import type { MCPClient, MCPClientFormValue } from './localTypes';
import {
    clientToFormValue,
    defaultMCPClientFormValue,
    formValueToClientRequest,
    validateClientForm,
} from './localTypes';

interface MCPClientEditorProps {
    open: boolean;
    client: MCPClient | null;
    onClose: () => void;
    onSave: (data: any) => void;
    saving: boolean;
}

const sectionSx = {
    border: '1px solid',
    borderColor: 'divider',
    borderRadius: 2,
    p: 2,
} as const;

const MCPClientEditor = ({
    open,
    client,
    onClose,
    onSave,
    saving,
}: MCPClientEditorProps) => {
    const [form, setForm] = useState<MCPClientFormValue>(defaultMCPClientFormValue());
    const [activeTab, setActiveTab] = useState(0);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (open) {
            setForm(clientToFormValue(client || undefined));
            setActiveTab(0);
            setError(null);
        }
    }, [open, client]);

    const set = (patch: Partial<MCPClientFormValue>) => {
        setForm((prev) => ({ ...prev, ...patch }));
        setError(null);
    };

    const handleSave = () => {
        const validationError = validateClientForm(form);
        if (validationError) {
            setError(validationError);
            return;
        }

        const request = formValueToClientRequest(form);
        onSave(request);
    };

    const isEditing = !!client;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth scroll="paper">
            <DialogTitle>
                {isEditing ? 'Edit MCP Client' : 'Add MCP Client'}
            </DialogTitle>
            <DialogContent dividers>
                <Stack spacing={3}>
                    {error && (
                        <Alert severity="error" onClose={() => setError(null)}>
                            {error}
                        </Alert>
                    )}

                    {/* Basic Settings */}
                    <Box sx={sectionSx}>
                        <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
                            Basic Settings
                        </Typography>
                        <Stack spacing={2}>
                            <TextField
                                fullWidth
                                label="Client Name"
                                placeholder="myClient"
                                value={form.name}
                                onChange={(e) => set({ name: e.target.value })}
                                disabled={isEditing}
                                helperText="ASCII only, no spaces/hyphens, cannot start with a number"
                                error={form.name.length > 0 && !/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(form.name)}
                            />
                            <FormControlLabel
                                control={
                                    <Checkbox
                                        checked={form.enabled}
                                        onChange={(e) => set({ enabled: e.target.checked })}
                                    />
                                }
                                label="Enabled"
                            />
                        </Stack>
                    </Box>

                    {/* Connection Type */}
                    <Box sx={sectionSx}>
                        <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
                            Connection Type
                        </Typography>
                        <ToggleButtonGroup
                            value={form.connection_type}
                            exclusive
                            fullWidth
                            onChange={(_, value) => value && set({ connection_type: value })}
                        >
                            <ToggleButton value="stdio">STDIO</ToggleButton>
                            <ToggleButton value="http">HTTP</ToggleButton>
                            <ToggleButton value="sse">SSE</ToggleButton>
                        </ToggleButtonGroup>
                    </Box>

                    {/* Connection Settings */}
                    {form.connection_type === 'stdio' ? (
                        <Box sx={sectionSx}>
                            <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
                                STDIO Configuration
                            </Typography>
                            <Stack spacing={2}>
                                <TextField
                                    fullWidth
                                    label="Command"
                                    placeholder="python3"
                                    value={form.stdio_command}
                                    onChange={(e) => set({ stdio_command: e.target.value })}
                                />
                                <Box>
                                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                                        Arguments
                                    </Typography>
                                    <Stack spacing={1}>
                                        {form.stdio_args.map((arg, idx) => (
                                            <Stack key={`arg-${idx}`} direction="row" spacing={1} alignItems="center">
                                                <TextField
                                                    fullWidth
                                                    size="small"
                                                    placeholder={`Argument ${idx + 1}`}
                                                    value={arg}
                                                    onChange={(e) => {
                                                        const args = [...form.stdio_args];
                                                        args[idx] = e.target.value;
                                                        set({ stdio_args: args });
                                                    }}
                                                />
                                                <IconButton
                                                    size="small"
                                                    onClick={() => {
                                                        const args = form.stdio_args.filter((_, i) => i !== idx);
                                                        set({ stdio_args: args });
                                                    }}
                                                >
                                                    <DeleteOutlineIcon />
                                                </IconButton>
                                            </Stack>
                                        ))}
                                        <Button
                                            variant="text"
                                            size="small"
                                            startIcon={<AddIcon />}
                                            onClick={() => set({ stdio_args: [...form.stdio_args, ''] })}
                                        >
                                            Add Argument
                                        </Button>
                                    </Stack>
                                </Box>
                                <TextField
                                    fullWidth
                                    label="Working Directory"
                                    placeholder="~/.tingly-box/mcp"
                                    value={form.stdio_cwd}
                                    onChange={(e) => set({ stdio_cwd: e.target.value })}
                                />
                                <Box>
                                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                                        Environment Variables
                                    </Typography>
                                    <Stack spacing={1}>
                                        {form.stdio_env.map((env, idx) => (
                                            <Stack key={`env-${idx}`} direction="row" spacing={1} alignItems="center">
                                                <TextField
                                                    size="small"
                                                    placeholder="Key"
                                                    value={env.key}
                                                    onChange={(e) => {
                                                        const stdio_env = [...form.stdio_env];
                                                        stdio_env[idx] = { ...env, key: e.target.value };
                                                        set({ stdio_env });
                                                    }}
                                                    sx={{ flex: 1 }}
                                                />
                                                <TextField
                                                    size="small"
                                                    placeholder="Value"
                                                    value={env.value}
                                                    onChange={(e) => {
                                                        const stdio_env = [...form.stdio_env];
                                                        stdio_env[idx] = { ...env, value: e.target.value };
                                                        set({ stdio_env });
                                                    }}
                                                    sx={{ flex: 1 }}
                                                />
                                                <IconButton
                                                    size="small"
                                                    onClick={() => {
                                                        const stdio_env = form.stdio_env.filter((_, i) => i !== idx);
                                                        set({ stdio_env });
                                                    }}
                                                >
                                                    <DeleteOutlineIcon />
                                                </IconButton>
                                            </Stack>
                                        ))}
                                        <Button
                                            variant="text"
                                            size="small"
                                            startIcon={<AddIcon />}
                                            onClick={() => set({ stdio_env: [...form.stdio_env, { key: '', value: '' }] })}
                                        >
                                            Add Environment Variable
                                        </Button>
                                    </Stack>
                                </Box>
                            </Stack>
                        </Box>
                    ) : (
                        <Box sx={sectionSx}>
                            <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
                                {form.connection_type === 'http' ? 'HTTP' : 'SSE'} Configuration
                            </Typography>
                            <Stack spacing={2}>
                                <TextField
                                    fullWidth
                                    label="Endpoint URL"
                                    placeholder="http://localhost:3000/mcp"
                                    value={form.endpoint}
                                    onChange={(e) => set({ endpoint: e.target.value })}
                                />
                                <Box>
                                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                                        Headers
                                    </Typography>
                                    <Stack spacing={1}>
                                        {form.headers.map((header, idx) => (
                                            <Stack key={`header-${idx}`} direction="row" spacing={1} alignItems="center">
                                                <TextField
                                                    size="small"
                                                    placeholder="Key"
                                                    value={header.key}
                                                    onChange={(e) => {
                                                        const headers = [...form.headers];
                                                        headers[idx] = { ...header, key: e.target.value };
                                                        set({ headers });
                                                    }}
                                                    sx={{ flex: 1 }}
                                                />
                                                <TextField
                                                    size="small"
                                                    placeholder="Value"
                                                    value={header.value}
                                                    onChange={(e) => {
                                                        const headers = [...form.headers];
                                                        headers[idx] = { ...header, value: e.target.value };
                                                        set({ headers });
                                                    }}
                                                    sx={{ flex: 1 }}
                                                />
                                                <IconButton
                                                    size="small"
                                                    onClick={() => {
                                                        const headers = form.headers.filter((_, i) => i !== idx);
                                                        set({ headers });
                                                    }}
                                                >
                                                    <DeleteOutlineIcon />
                                                </IconButton>
                                            </Stack>
                                        ))}
                                        <Button
                                            variant="text"
                                            size="small"
                                            startIcon={<AddIcon />}
                                            onClick={() => set({ headers: [...form.headers, { key: '', value: '' }] })}
                                        >
                                            Add Header
                                        </Button>
                                    </Stack>
                                </Box>
                            </Stack>
                        </Box>
                    )}

                    {/* Authentication */}
                    <Box sx={sectionSx}>
                        <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
                            Authentication
                        </Typography>
                        <ToggleButtonGroup
                            value={form.auth_type}
                            exclusive
                            fullWidth
                            onChange={(_, value) => value && set({ auth_type: value })}
                            sx={{ mb: 2 }}
                        >
                            <ToggleButton value="none">None</ToggleButton>
                            <ToggleButton value="headers">Headers</ToggleButton>
                            <ToggleButton value="oauth">OAuth</ToggleButton>
                        </ToggleButtonGroup>

                        {form.auth_type === 'oauth' && (
                            <Stack spacing={2}>
                                <TextField
                                    fullWidth
                                    label="Client ID"
                                    value={form.oauth_client_id}
                                    onChange={(e) => set({ oauth_client_id: e.target.value })}
                                />
                                <TextField
                                    fullWidth
                                    label="Client Secret"
                                    type="password"
                                    value={form.oauth_client_secret}
                                    onChange={(e) => set({ oauth_client_secret: e.target.value })}
                                />
                                <TextField
                                    fullWidth
                                    label="Authorize URL"
                                    placeholder="https://example.com/oauth/authorize"
                                    value={form.oauth_authorize_url}
                                    onChange={(e) => set({ oauth_authorize_url: e.target.value })}
                                />
                                <TextField
                                    fullWidth
                                    label="Token URL"
                                    placeholder="https://example.com/oauth/token"
                                    value={form.oauth_token_url}
                                    onChange={(e) => set({ oauth_token_url: e.target.value })}
                                />
                                <TextField
                                    fullWidth
                                    label="Scopes"
                                    placeholder="read write"
                                    value={form.oauth_scopes.join(' ')}
                                    onChange={(e) => set({ oauth_scopes: e.target.value.split(/\s+/).filter(Boolean) })}
                                    helperText="Space-separated list of scopes"
                                />
                            </Stack>
                        )}
                    </Box>

                    {/* Tools Configuration */}
                    <Box sx={sectionSx}>
                        <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
                            Tools Configuration
                        </Typography>
                        <TextField
                            fullWidth
                            label="Tools to Expose"
                            placeholder="* or tool1 tool2 tool3"
                            value={form.tools_to_execute.join(' ')}
                            onChange={(e) => set({ tools_to_execute: e.target.value.split(/\s+/).filter(Boolean) })}
                            helperText="Use * for all tools, or list specific tool names separated by spaces"
                            sx={{ mb: 2 }}
                        />
                        <TextField
                            fullWidth
                            label="Auto-Execute Tools"
                            placeholder="tool1 tool2"
                            value={form.tools_to_auto_execute.join(' ')}
                            onChange={(e) => set({ tools_to_auto_execute: e.target.value.split(/\s+/).filter(Boolean) })}
                            helperText="Tools that can be executed automatically without confirmation"
                        />
                    </Box>

                    {/* Advanced Settings */}
                    <Box sx={sectionSx}>
                        <Typography variant="h6" fontWeight={700} sx={{ mb: 2 }}>
                            Advanced Settings
                        </Typography>
                        <Stack spacing={2}>
                            <TextField
                                fullWidth
                                label="Proxy URL"
                                placeholder="http://127.0.0.1:7897"
                                value={form.proxy_url}
                                onChange={(e) => set({ proxy_url: e.target.value })}
                                helperText="HTTP proxy for outgoing requests"
                            />
                            <TextField
                                fullWidth
                                label="Allowed Extra Headers"
                                placeholder="X-Custom-Header X-Request-ID"
                                value={form.allowed_extra_headers.join(' ')}
                                onChange={(e) => set({ allowed_extra_headers: e.target.value.split(/\s+/).filter(Boolean) })}
                                helperText="Additional headers to forward from client requests"
                            />
                        </Stack>
                    </Box>
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose} disabled={saving}>
                    Cancel
                </Button>
                <Button
                    variant="contained"
                    onClick={handleSave}
                    disabled={saving}
                >
                    {saving ? 'Saving...' : isEditing ? 'Update' : 'Create'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default MCPClientEditor;
