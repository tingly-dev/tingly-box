import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField } from '@mui/material';
import { api } from '@/services/api';
import MCPToolParameters from './MCPToolParameters';
import type { MCPCatalogTool } from './types';

export default function MCPToolTestDialog({
    tool,
    enabled,
    onClose,
}: {
    tool: MCPCatalogTool;
    enabled: boolean;
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
            const response = await api.callMCPTool({
                source_id: tool.source_id,
                tool_name: tool.name,
                arguments: parsed as Record<string, unknown>,
            });
            setResult(response);
            if (!response?.success) throw new Error(response?.error || 'Tool test failed');
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
                    <MCPToolParameters tool={tool} expanded />
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
                <Button variant="contained" disabled={busy || !enabled || !tool.enabled} onClick={() => void run()}>
                    {label('runTest', 'Run test')}
                </Button>
            </DialogActions>
        </Dialog>
    );
}
