import { useState } from 'react';
import { Alert, Button, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { MCPSourceConfig } from './types';

export default function WebtoolsSettings({ source, onSave }: {
    source: MCPSourceConfig;
    onSave: (patch: MCPSourceConfig) => Promise<void>;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [key, setKey] = useState(source.env?.SERPER_API_KEY || '');
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const save = async () => {
        setSaving(true);
        setError('');
        try {
            const env = { ...source.env };
            if (key.trim()) env.SERPER_API_KEY = key.trim();
            else delete env.SERPER_API_KEY;
            await onSave({ id: source.id, env });
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setSaving(false);
        }
    };
    return (
        <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
                {label('webtoolsSetupHint', 'Search and Fetch reuse your configured DeepSeek API key when no Serper key is set. Otherwise they use Serper and Jina Reader. Enable each tool separately; the MCP version has independent settings.')}
            </Typography>
            <Alert severity="info">
                {label('webtoolsPrivacyHint', 'Queries and URLs are sent to the selected backend (DeepSeek, Serper or Jina Reader). DeepSeek is billed per successful request. Only explicitly provided original questions are sent to search.')}
            </Alert>
            {error && <Alert severity="error">{error}</Alert>}
            <TextField
                label={label('serperAPIKey', 'Serper API key')}
                type="password"
                autoComplete="off"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                helperText={label('serperAPIKeyHint', 'Optional SERPER_API_KEY — leave blank to use a configured DeepSeek provider. Without either key, Fetch still works through Jina Reader.')}
                disabled={saving}
                fullWidth
            />
            <Button variant="contained" disabled={saving} onClick={() => void save()}>
                {label('saveWebtools', 'Save web tool settings')}
            </Button>
        </Stack>
    );
}
