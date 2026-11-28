import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { Alert, Button, Chip, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { radius } from '@tingly/ui/theme/radius';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';
import type { MCPCatalogTool, MCPSourceConfig } from '../../features/mcp/types';

type Operation = 'search' | 'fetch';
const backendNames: Record<string, string> = { deepseek: 'DeepSeek', serper: 'Serper', jina: 'Jina Reader' };
const emptyTools: MCPCatalogTool[] = [];
const backendKey = (op: Operation) => `WEB_${op.toUpperCase()}_BACKEND`;
const providerKey = (op: Operation) => `WEB_${op.toUpperCase()}_PROVIDER_UUID`;
const savedBackend = (source: MCPSourceConfig, op: Operation) => {
    const value = source.env?.[backendKey(op)]?.trim().toLowerCase();
    return !value || value === 'auto' ? (op === 'search' ? 'serper' : 'jina') : value;
};
const eligibleCredential = (provider: Provider) => {
    if (!provider.enabled || !provider.token || (provider.auth_type && provider.auth_type !== 'api_key')) return false;
    try {
        const url = new URL(provider.api_base_openai || provider.api_base);
        return url.protocol === 'https:' && url.hostname === 'api.deepseek.com' && !url.username && !url.password;
    } catch {
        return false;
    }
};

export default function WebtoolsSettings({ source, tools = emptyTools, onSave }: {
    source: MCPSourceConfig;
    tools?: MCPCatalogTool[];
    onSave: (patch: MCPSourceConfig) => Promise<void>;
}) {
    const { t } = useTranslation();
    const id = useId();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [key, setKey] = useState(source.env?.SERPER_API_KEY || '');
    const [backends, setBackends] = useState({ search: savedBackend(source, 'search'), fetch: savedBackend(source, 'fetch') });
    const [selected, setSelected] = useState({ search: source.env?.WEB_SEARCH_PROVIDER_UUID || '', fetch: source.env?.WEB_FETCH_PROVIDER_UUID || '' });
    const [providers, setProviders] = useState<Provider[]>([]);
    const [loadingProviders, setLoadingProviders] = useState(true);
    const [providerError, setProviderError] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const providerGeneration = useRef(0);
    const refreshProviders = useCallback(() => {
        const generation = ++providerGeneration.current;
        return api.getProviders().then((response) => {
            if (!response?.success || !Array.isArray(response.data)) throw new Error('Could not load credentials');
            if (generation === providerGeneration.current) setProviders(response.data);
        }).catch((err) => {
            console.error('Error loading web tool credentials:', err);
            if (generation === providerGeneration.current) { setProviderError(true); setProviders([]); }
        }).finally(() => {
            if (generation === providerGeneration.current) setLoadingProviders(false);
        });
    }, []);
    useEffect(() => {
        void refreshProviders();
        return () => { providerGeneration.current++; };
    }, [refreshProviders]);
    const credentials = providers.filter(eligibleCredential);
    const operations: Operation[] = ['search', 'fetch'];
    const dirty = key !== (source.env?.SERPER_API_KEY || '') || operations.some((op) =>
        backends[op] !== savedBackend(source, op) || selected[op] !== (source.env?.[providerKey(op)] || ''));
    const invalid = operations.some((op) => !['deepseek', op === 'search' ? 'serper' : 'jina'].includes(backends[op]) ||
        (backends[op] === 'deepseek' && (loadingProviders || !credentials.some((p) => p.uuid === selected[op]))));
    const save = async () => {
        setSaving(true);
        setError('');
        try {
            const env = { ...source.env };
            if (key.trim()) env.SERPER_API_KEY = key.trim();
            else delete env.SERPER_API_KEY;
            for (const op of operations) {
                env[backendKey(op)] = backends[op];
                if (selected[op]) env[providerKey(op)] = selected[op];
                else delete env[providerKey(op)];
            }
            await onSave({ id: source.id, env });
            setKey(key.trim());
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setSaving(false);
        }
    };
    const credentialSource = (op: Operation, backend?: string) => {
        if (backend === 'serper') return label('webtoolsSerperCredential', 'SERPER_API_KEY in this connection’s settings.');
        if (backend === 'jina') return label('webtoolsJinaCredential', 'No configuration required. Jina Reader uses anonymous access.');
        if (backend === 'deepseek') {
            const uuid = source.env?.[providerKey(op)];
            const provider = providers.find((p) => p.uuid === uuid);
            return provider?.name || uuid || label('webtoolsNoCredential', 'No credential selected');
        }
        return label('webtoolsUnknown', 'Not checked');
    };
    return (
        <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
                {label('webtoolsSetupHint', 'Choose a source for each tool, then configure its authentication. Search and Fetch are independent; no automatic switching.')}
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
            {dirty && <Alert severity="info">{label('webtoolsUnsaved', 'Unsaved changes. Status below describes the saved configuration, not these edits.')}</Alert>}
            {source.enabled === false && <Alert severity="warning">{label('webtoolsSourceOff', 'This source is off. Save settings here, then enable the tools to use them.')}</Alert>}
            {operations.map((op) => {
                const value = backends[op];
                const choices = op === 'search' ? ['deepseek', 'serper'] : ['deepseek', 'jina'];
                const tool = tools.find((item) => item.name === `mcp_web_${op}`);
                const unavailable = selected[op] && !credentials.some((p) => p.uuid === selected[op]);
                return (
                    <Stack key={op} spacing={1.5} component="section" aria-label={op === 'search' ? 'Search' : 'Fetch'}
                        sx={{ p: 2, border: '1px solid', borderColor: 'divider', borderRadius: radius.row }}>
                        <Typography variant="subtitle2">{op === 'search' ? 'Search' : 'Fetch'}</Typography>
                        <TextField select fullWidth id={`${id}-${op}`} value={value} disabled={saving}
                            label={op === 'search' ? label('webtoolsSearchBackend', 'Search source') : label('webtoolsFetchBackend', 'Fetch source')}
                            onChange={(e) => setBackends({ ...backends, [op]: e.target.value })}>
                            {choices.map((choice) => <MenuItem key={choice} value={choice}>{backendNames[choice]}</MenuItem>)}
                            {!choices.includes(value) && <MenuItem value={value} disabled>{value}</MenuItem>}
                        </TextField>
                        {value === 'deepseek' && <Stack spacing={1}>
                            <TextField select fullWidth id={`${id}-${op}-credential`} value={selected[op]} disabled={saving || loadingProviders}
                                label={label('webtoolsDeepSeekCredential', 'DeepSeek credential')}
                                onChange={(e) => setSelected({ ...selected, [op]: e.target.value })}
                                error={!!unavailable && !loadingProviders}
                                helperText={label('webtoolsCredentialHint', 'Select an enabled first-party DeepSeek API-key credential. Only its reference is saved; no key is copied.')}>
                                <MenuItem value="" disabled>{label('webtoolsNoCredential', 'No credential selected')}</MenuItem>
                                {credentials.map((provider) => <MenuItem key={provider.uuid} value={provider.uuid}>
                                    {provider.name} · {provider.uuid.slice(0, 8)}
                                </MenuItem>)}
                                {unavailable && <MenuItem value={selected[op]} disabled>
                                    {selected[op]} · {label('webtoolsUnavailableCredential', 'Unavailable')}
                                </MenuItem>}
                            </TextField>
                            {!loadingProviders && (providerError || credentials.length === 0) && <Alert severity="warning">
                                {providerError ? label('webtoolsCredentialsFailed', 'Could not load credentials. Retry below.')
                                    : label('webtoolsNoCredentials', 'No eligible DeepSeek credentials. Add an API key in Credentials, then refresh.')}
                            </Alert>}
                            <Stack direction="row" sx={{ gap: 1, flexWrap: 'wrap' }}>
                                <Button href="/credentials" target="_blank" rel="noopener noreferrer" size="small">
                                    {label('webtoolsManageCredentials', 'Manage credentials ↗')}
                                </Button>
                                <Button size="small" disabled={saving || loadingProviders} onClick={() => {
                                    setLoadingProviders(true);
                                    setProviderError(false);
                                    void refreshProviders();
                                }}>
                                    {label('webtoolsRefreshCredentials', 'Refresh credentials')}
                                </Button>
                            </Stack>
                        </Stack>}
                        {value === 'serper' && <TextField
                            id={`${id}-serper`} label={label('serperAPIKey', 'Serper API key')}
                            type="password" autoComplete="off" value={key}
                            onChange={(e) => setKey(e.target.value)}
                            helperText={label('serperAPIKeyHint', 'Only used by Serper Search. Kept when switching sources; never affects Fetch.')}
                            disabled={saving} fullWidth
                        />}
                        {value === 'jina' && <Typography variant="body2" color="text.secondary">{credentialSource(op, 'jina')}</Typography>}
                        <Stack spacing={0.75} sx={{ pt: 1, borderTop: '1px solid', borderColor: 'divider' }}>
                            <Stack direction="row" sx={{ gap: 1, alignItems: 'center', flexWrap: 'wrap' }}>
                                <Typography variant="body2">
                                    {label('webtoolsAppliedBackend', 'Actual source (saved)')}: {backendNames[tool?.backend || ''] || '—'}
                                </Typography>
                                <Chip size="small" variant="outlined"
                                    color={tool?.ready === true ? 'success' : tool?.ready === false ? 'warning' : 'default'}
                                    label={tool?.ready === true ? label('webtoolsReady', 'Configuration ready') : tool?.ready === false
                                        ? label('webtoolsNotReady', 'Not ready') : label('webtoolsUnknown', 'Not checked')} />
                            </Stack>
                            <Typography variant="caption" color="text.secondary">
                                {label('webtoolsCredentialSource', 'Credential source')}: {credentialSource(op, tool?.backend)}
                            </Typography>
                        </Stack>
                    </Stack>
                );
            })}
            <Typography variant="caption" color="text.secondary">
                {label('webtoolsReadinessHint', 'Readiness checks configuration, not balance or connectivity. Use Test tool to verify a live request. Enable each tool separately.')}
            </Typography>
            <Alert severity="info">
                {label('webtoolsPrivacyHint', 'Queries and URLs are sent to the selected backend (DeepSeek, Serper or Jina Reader). DeepSeek is billed per successful request. Only explicitly provided original questions are sent to search.')}
            </Alert>
            <Button variant="contained" disabled={saving || invalid} onClick={() => void save()}>
                {label('saveWebtools', 'Save web tool settings')}
            </Button>
        </Stack>
    );
}
