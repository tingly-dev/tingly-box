import {
    Box,
    Button,
    Checkbox,
    FormControlLabel,
    IconButton,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
} from '@mui/material';
import { Add, DeleteOutline } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import type { MCPKVPair, MCPSourceFormValue } from './types';

interface MCPSourceEditorProps {
    title?: string;
    value: MCPSourceFormValue;
    onChange: (next: MCPSourceFormValue) => void;
    lockId?: boolean;
    hideTools?: boolean;
    onUseExample?: () => void;
}

export default function MCPSourceEditor({ value, onChange, lockId = false, hideTools = false }: MCPSourceEditorProps) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.center.${key}`, { defaultValue: fallback });
    const set = (patch: Partial<MCPSourceFormValue>) => onChange({ ...value, ...patch });
    const pairs = (field: 'env' | 'headers', title: string) => (
        <Stack spacing={1}>
            <Typography variant="subtitle2">{title}</Typography>
            {value[field].map((row, index) => (
                <Stack direction="row" spacing={1} key={index}>
                    <TextField
                        size="small"
                        label={label('key', 'Key')}
                        value={row.key}
                        onChange={(e) =>
                            set({
                                [field]: value[field].map((item, i) =>
                                    i === index ? { ...item, key: e.target.value } : item
                                ),
                            })
                        }
                    />
                    <TextField
                        size="small"
                        fullWidth
                        label={label('value', 'Value')}
                        type="password"
                        value={row.value}
                        onChange={(e) =>
                            set({
                                [field]: value[field].map((item, i) =>
                                    i === index ? { ...item, value: e.target.value } : item
                                ),
                            })
                        }
                    />
                    <IconButton
                        aria-label={label('remove', 'Remove')}
                        onClick={() => set({ [field]: value[field].filter((_, i) => i !== index) })}
                    >
                        <DeleteOutline />
                    </IconButton>
                </Stack>
            ))}
            <Button
                startIcon={<Add />}
                onClick={() => set({ [field]: [...value[field], { key: '', value: '' } as MCPKVPair] })}
            >
                {label('addField', 'Add field')}
            </Button>
        </Stack>
    );
    return (
        <Stack spacing={2.5}>
            <TextField
                label={label('sourceId', 'Server ID')}
                value={value.id}
                disabled={lockId}
                onChange={(e) => set({ id: e.target.value })}
                helperText={label(
                    'idHint',
                    'Use letters, numbers, underscores or hyphens. This ID remains stable after creation.'
                )}
            />
            <ToggleButtonGroup
                exclusive
                fullWidth
                value={value.transport}
                onChange={(_, transport) => transport && set({ transport })}
            >
                <ToggleButton value="stdio">STDIO</ToggleButton>
                <ToggleButton value="http">Streamable HTTP</ToggleButton>
                <ToggleButton value="sse">SSE</ToggleButton>
            </ToggleButtonGroup>
            {value.transport === 'stdio' ? (
                <Stack spacing={2}>
                    <TextField
                        label={label('command', 'Command')}
                        value={value.command}
                        onChange={(e) => set({ command: e.target.value })}
                        placeholder="npx"
                    />
                    <Stack spacing={1}>
                        <Typography variant="subtitle2">{label('arguments', 'Arguments')}</Typography>
                        {value.args.map((arg, index) => (
                            <Stack direction="row" spacing={1} key={index}>
                                <TextField
                                    fullWidth
                                    size="small"
                                    value={arg}
                                    onChange={(e) =>
                                        set({
                                            args: value.args.map((item, i) => (i === index ? e.target.value : item)),
                                        })
                                    }
                                />
                                <IconButton
                                    aria-label={label('remove', 'Remove')}
                                    onClick={() => set({ args: value.args.filter((_, i) => i !== index) })}
                                >
                                    <DeleteOutline />
                                </IconButton>
                            </Stack>
                        ))}
                        <Button startIcon={<Add />} onClick={() => set({ args: [...value.args, ''] })}>
                            {label('addArgument', 'Add argument')}
                        </Button>
                    </Stack>
                    <TextField
                        label={label('cwd', 'Working directory')}
                        value={value.cwd}
                        onChange={(e) => set({ cwd: e.target.value })}
                    />
                </Stack>
            ) : (
                <Stack spacing={2}>
                    <TextField
                        label={label('endpoint', 'Endpoint URL')}
                        value={value.endpoint}
                        onChange={(e) => set({ endpoint: e.target.value })}
                        placeholder="https://example.com/mcp"
                    />
                    {pairs('headers', label('headers', 'Authentication headers'))}
                </Stack>
            )}
            {pairs('env', label('env', 'Environment variables'))}
            <TextField
                label={label('envPassthrough', 'Inherit environment variables')}
                value={value.envPassthrough.join(' ')}
                onChange={(e) => set({ envPassthrough: e.target.value.split(/\s+/).filter(Boolean) })}
                helperText={label('envHint', 'Space-separated names; referenced variables must exist on the server.')}
            />
            <Box>
                <FormControlLabel
                    control={
                        <Checkbox
                            checked={value.useGlobalProxy}
                            onChange={(e) => set({ useGlobalProxy: e.target.checked })}
                        />
                    }
                    label={label('globalProxy', 'Use global proxy')}
                />
                {!value.useGlobalProxy && (
                    <TextField
                        fullWidth
                        label={label('proxy', 'Proxy URL')}
                        value={value.proxyUrl}
                        onChange={(e) => set({ proxyUrl: e.target.value })}
                    />
                )}
            </Box>
            <Stack>
                <Typography variant="subtitle2">{label('usage', 'Where can these tools be used?')}</Typography>
                <FormControlLabel
                    control={
                        <Checkbox
                            checked={value.usage.client}
                            onChange={(e) => set({ usage: { ...value.usage, client: e.target.checked } })}
                        />
                    }
                    label={label('clientUsage', 'MCP clients')}
                />
                <FormControlLabel
                    control={
                        <Checkbox
                            checked={value.usage.gateway}
                            onChange={(e) => set({ usage: { ...value.usage, gateway: e.target.checked } })}
                        />
                    }
                    label={label('gatewayUsage', 'Gateway model calls')}
                />
            </Stack>
            {!hideTools && (
                <TextField
                    label={label('allowlist', 'Allowed tools')}
                    value={value.tools.join(' ')}
                    onChange={(e) => set({ tools: e.target.value.split(/\s+/).filter(Boolean) })}
                    helperText={label(
                        'allowlistHint',
                        'Use * for all tools. Fine-tune individual tools in the Capabilities tab.'
                    )}
                />
            )}
            <FormControlLabel
                control={<Checkbox checked={value.enabled} onChange={(e) => set({ enabled: e.target.checked })} />}
                label={label('enabled', 'Enabled')}
            />
        </Stack>
    );
}
