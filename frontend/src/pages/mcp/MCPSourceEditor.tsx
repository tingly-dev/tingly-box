import {
    Add as AddIcon,
    DeleteOutline as DeleteOutlineIcon,
    OpenInNew as OpenInNewIcon,
} from '@mui/icons-material';
import {
    Box,
    Button,
    Divider,
    IconButton,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
    Checkbox,
    FormControlLabel,
} from '@mui/material';
import type { MCPSourceFormValue } from './types';

interface MCPSourceEditorProps {
    title?: string;
    value: MCPSourceFormValue;
    onChange: (next: MCPSourceFormValue) => void;
    lockId?: boolean;
    hideTools?: boolean;
    onUseExample?: () => void;
}

const sectionSx = {
    border: '1px solid',
    borderColor: 'divider',
    borderRadius: 2,
    p: 2,
} as const;

const MCPSourceEditor = ({
    title = 'Connect to a custom MCP',
    value,
    onChange,
    lockId = false,
    hideTools = false,
    onUseExample,
}: MCPSourceEditorProps) => {
    const set = (patch: Partial<MCPSourceFormValue>) => onChange({ ...value, ...patch });

    return (
        <Stack spacing={2}>
            <Stack spacing={0.5}>
                <Typography variant="h4" fontWeight={700}>{title}</Typography>
                <Stack direction="row" spacing={2} alignItems="center">
                    <Button
                        href="https://tingly-dev.github.io/"
                        target="_blank"
                        rel="noreferrer"
                        startIcon={<OpenInNewIcon />}
                        sx={{ alignSelf: 'flex-start', px: 0 }}
                    >
                        Docs
                    </Button>
                    {onUseExample && (
                        <Button variant="text" sx={{ px: 0 }} onClick={onUseExample}>
                            Use Weather Example
                        </Button>
                    )}
                </Stack>
            </Stack>

            <Box sx={sectionSx}>
                <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Name</Typography>
                <TextField
                    fullWidth
                    placeholder="MCP server name"
                    value={value.id}
                    onChange={(e) => set({ id: e.target.value })}
                    disabled={lockId}
                />
            </Box>

            <Box sx={{ ...sectionSx, p: 0, overflow: 'hidden' }}>
                    <ToggleButtonGroup
                        exclusive
                        fullWidth
                        value={value.transport}
                        onChange={(_, v) => v && set({ transport: v })}
                    sx={{ '& .MuiToggleButton-root': { border: 'none', borderRadius: 0, py: 1.25, fontWeight: 700 } }}
                >
                    <ToggleButton value="stdio">STDIO</ToggleButton>
                    <ToggleButton value="http">Streamable HTTP / SSE</ToggleButton>
                </ToggleButtonGroup>
            </Box>

            {value.transport === 'stdio' ? (
                <>
                    <Box sx={sectionSx}>
                        <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Command to launch</Typography>
                        <TextField
                            fullWidth
                            placeholder="python3"
                            value={value.command}
                            onChange={(e) => set({ command: e.target.value })}
                        />
                    </Box>

                    <Box sx={sectionSx}>
                        <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Arguments</Typography>
                        <Stack spacing={1}>
                            {value.args.map((arg, idx) => (
                                <Stack key={`arg-${idx}`} direction="row" spacing={1} alignItems="center">
                                    <TextField
                                        fullWidth
                                        value={arg}
                                        onChange={(e) => {
                                            const args = [...value.args];
                                            args[idx] = e.target.value;
                                            set({ args });
                                        }}
                                    />
                                    <IconButton
                                        onClick={() => {
                                            const args = value.args.filter((_, i) => i !== idx);
                                            set({ args });
                                        }}
                                    >
                                        <DeleteOutlineIcon />
                                    </IconButton>
                                </Stack>
                            ))}
                            <Button
                                variant="text"
                                startIcon={<AddIcon />}
                                onClick={() => set({ args: [...value.args, ''] })}
                            >
                                Add argument
                            </Button>
                        </Stack>
                    </Box>
                </>
            ) : (
                <Box sx={sectionSx}>
                    <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Streamable HTTP endpoint</Typography>
                    <TextField
                        fullWidth
                        placeholder="http://localhost:3000/mcp"
                        value={value.endpoint}
                        onChange={(e) => set({ endpoint: e.target.value })}
                    />
                </Box>
            )}

            <Box sx={sectionSx}>
                <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Environment variables</Typography>
                <Stack spacing={1}>
                    {value.env.map((row, idx) => (
                        <Stack key={`env-${idx}`} direction="row" spacing={1} alignItems="center">
                            <TextField
                                fullWidth
                                placeholder="Key"
                                value={row.key}
                                onChange={(e) => {
                                    const env = [...value.env];
                                    env[idx] = { ...env[idx], key: e.target.value };
                                    set({ env });
                                }}
                            />
                            <TextField
                                fullWidth
                                placeholder="Value"
                                value={row.value}
                                onChange={(e) => {
                                    const env = [...value.env];
                                    env[idx] = { ...env[idx], value: e.target.value };
                                    set({ env });
                                }}
                            />
                            <IconButton
                                onClick={() => {
                                    const env = value.env.filter((_, i) => i !== idx);
                                    set({ env });
                                }}
                            >
                                <DeleteOutlineIcon />
                            </IconButton>
                        </Stack>
                    ))}
                    <Button
                        variant="text"
                        startIcon={<AddIcon />}
                        onClick={() => set({ env: [...value.env, { key: '', value: '' }] })}
                    >
                        Add environment variable
                    </Button>
                </Stack>
            </Box>

            <Box sx={sectionSx}>
                <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Environment variable passthrough</Typography>
                <Stack spacing={1}>
                    {value.envPassthrough.map((item, idx) => (
                        <Stack key={`pass-${idx}`} direction="row" spacing={1} alignItems="center">
                            <TextField
                                fullWidth
                                value={item}
                                onChange={(e) => {
                                    const envPassthrough = [...value.envPassthrough];
                                    envPassthrough[idx] = e.target.value;
                                    set({ envPassthrough });
                                }}
                            />
                            <IconButton
                                onClick={() => {
                                    const envPassthrough = value.envPassthrough.filter((_, i) => i !== idx);
                                    set({ envPassthrough });
                                }}
                            >
                                <DeleteOutlineIcon />
                            </IconButton>
                        </Stack>
                    ))}
                    <Button
                        variant="text"
                        startIcon={<AddIcon />}
                        onClick={() => set({ envPassthrough: [...value.envPassthrough, ''] })}
                    >
                        Add variable
                    </Button>
                </Stack>
            </Box>

            <Box sx={sectionSx}>
                <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Proxy</Typography>
                <FormControlLabel
                    control={
                        <Checkbox
                            checked={value.useGlobalProxy}
                            onChange={(e) => {
                                const checked = e.target.checked;
                                const passthrough = new Set(value.envPassthrough);
                                if (checked) {
                                    passthrough.add('HTTP_PROXY');
                                    passthrough.add('HTTPS_PROXY');
                                    passthrough.add('NO_PROXY');
                                }
                                set({
                                    useGlobalProxy: checked,
                                    envPassthrough: Array.from(passthrough),
                                    proxyUrl: checked ? '' : value.proxyUrl,
                                });
                            }}
                        />
                    }
                    label="Use global proxy configuration"
                />
                {!value.useGlobalProxy && (
                    <TextField
                        fullWidth
                        placeholder="http://127.0.0.1:7897"
                        value={value.proxyUrl}
                        onChange={(e) => set({ proxyUrl: e.target.value })}
                        sx={{ mt: 1 }}
                    />
                )}
            </Box>

            <Box sx={sectionSx}>
                <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Working directory</Typography>
                <TextField
                    fullWidth
                    placeholder="~/code"
                    value={value.cwd}
                    onChange={(e) => set({ cwd: e.target.value })}
                    disabled={value.transport === 'http'}
                />
            </Box>

            {!hideTools && (
                <Box sx={sectionSx}>
                    <Typography variant="h6" fontWeight={700} sx={{ mb: 1 }}>Tools</Typography>
                    <TextField
                        fullWidth
                        placeholder="* or web_search web_fetch"
                        value={value.tools.join(' ')}
                        onChange={(e) => set({ tools: e.target.value.split(/\s+/).filter(Boolean) })}
                    />
                    <Divider sx={{ mt: 1, mb: 1 }} />
                    <Typography variant="caption" color="text.secondary">Use `*` for all tools, or list names separated by spaces.</Typography>
                </Box>
            )}
        </Stack>
    );
};

export default MCPSourceEditor;
