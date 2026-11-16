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
                {label('webtoolsSetupHint', 'Search uses Serper and needs its API key. Fetch uses Jina Reader without a search key. Save the settings, then enable each tool separately. These settings do not change the MCP version.')}
            </Typography>
            <Alert severity="info">
                {label('webtoolsPrivacyHint', 'Search queries are sent to Serper; fetched URLs are sent to Jina Reader. Enable only the tools you want the gateway to run.')}
            </Alert>
            {error && <Alert severity="error">{error}</Alert>}
            <TextField
                label={label('serperAPIKey', 'Serper API key')}
                type="password"
                autoComplete="off"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                helperText={label('serperAPIKeyHint', 'SERPER_API_KEY — required only for Search. Leave blank to remove the saved key.')}
                disabled={saving}
                fullWidth
            />
            <Button variant="contained" disabled={saving} onClick={() => void save()}>
                {label('saveWebtools', 'Save web tool settings')}
            </Button>
        </Stack>
    );
}
