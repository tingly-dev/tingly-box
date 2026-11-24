import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField } from '@mui/material';
import { api } from '@/services/api';
import type { components } from '@/client';
import MCPToolParameters from './MCPToolParameters';
import { isServerWebtoolsSource } from './toolPresentation';
import type { MCPCatalogTool } from '../../features/mcp/types';

export default function MCPToolTestDialog({
    tool,
    enabled,
    profileId,
    onClose,
}: {
    tool: MCPCatalogTool;
    enabled: boolean;
    profileId?: string;
    onClose: () => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [args, setArgs] = useState('{}');
    const [result, setResult] = useState<unknown>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const run = async () => {
        setBusy(true);
        setError('');
        setResult(null);
        try {
            const parsed: unknown = JSON.parse(args);
            if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))
                throw new Error(label('objectArgs', 'Arguments must be a JSON object.'));
            const argumentsObject = parsed as Record<string, unknown>;
            const response: components['schemas']['MCPToolCallResponse'] | components['schemas']['MCPClientProbeResponse'] = profileId
                ? await api.probeMCPProfile(profileId, { tool_name: tool.normalized_name, arguments: argumentsObject })
                : await api.callMCPTool({ source_id: tool.source_id, tool_name: tool.name, arguments: argumentsObject });
            setResult(response);
            if (!response?.success || response.result?.isError) {
                const detail = response?.result?.content?.find((item) => item.type === 'text')?.text;
                throw new Error(response?.error || detail || label('toolTestFailed', 'Tool test failed'));
            }
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    return (
        <Dialog open onClose={() => !busy && onClose()} fullWidth maxWidth="md">
            <DialogTitle>
                {tool.source_id} / {tool.name}
            </DialogTitle>
            <DialogContent dividers>
                <Stack spacing={2}>
                    {error && <Alert severity="error">{error}</Alert>}
                    <MCPToolParameters tool={tool} />
                    <TextField
                        label={label('argumentsJSON', 'Arguments (JSON)')}
                        multiline
                        minRows={3}
                        value={args}
                        onChange={(e) => setArgs(e.target.value)}
                        disabled={busy}
                    />
                    {result !== null && (
                        <Box
                            component="pre"
                            data-testid="mcp-tool-test-result"
                            sx={{ m: 0, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontSize: 12 }}
                        >
                            {JSON.stringify(result, null, 2)}
                        </Box>
                    )}
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button disabled={busy} onClick={onClose}>
                    {t('mcp.center.close', { defaultValue: 'Close' })}
                </Button>
                <Button variant="contained" disabled={busy || (!enabled && !isServerWebtoolsSource({ id: tool.source_id })) || !tool.enabled} onClick={() => void run()}>
                    {label('runTest', 'Run test')}
                </Button>
            </DialogActions>
        </Dialog>
    );
}
