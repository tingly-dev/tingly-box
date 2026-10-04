import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Autocomplete,
    Button,
    Card,
    CardContent,
    Checkbox,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { api } from '@/services/api';
import AgentInstallCard from './AgentInstallCard';
import type { MCPClientProfile, MCPConfigResponse, MCPRuntimeConfig, MCPSourceStatus } from './types';

export default function MCPClientsPanel({
    config,
    onSaved,
}: {
    config: MCPRuntimeConfig;
    onSaved: (response: MCPConfigResponse) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.center.${key}`, { defaultValue: fallback });
    const [profile, setProfile] = useState<MCPClientProfile | null>(null);
    const [isNew, setIsNew] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [catalog, setCatalog] = useState<MCPSourceStatus[]>([]);
    const [selected, setSelected] = useState('tb');
    const profiles = config.client_profiles || [];
    const profileOpen = profile !== null;
    useEffect(() => {
        if (!profileOpen) return;
        let active = true;
        api.getMCPCatalog()
            .then((response) => {
                if (active && response?.success) setCatalog(response.sources || []);
            })
            .catch((e) => {
                if (active) setError(e instanceof Error ? e.message : String(e));
            });
        return () => {
            active = false;
        };
    }, [profileOpen]);
    const run = async (action: () => Promise<void>) => {
        setBusy(true);
        setError('');
        try {
            await action();
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };
    return (
        <Stack spacing={2}>
            <Typography color="text.secondary">
                {label(
                    'clientsHint',
                    'Grant each client access to selected servers and tools, independently of upstream connections.'
                )}
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
            {!config.client_profiles_configured && profiles.length === 0 && (
                <Alert severity="info">
                    {label(
                        'legacyHint',
                        'The default tb endpoint exposes client-enabled tools. Creating an explicit profile switches access to configured profiles only.'
                    )}
                </Alert>
            )}
            <Button
                sx={{ alignSelf: 'flex-start' }}
                variant="contained"
                disabled={busy}
                onClick={() => {
                    setIsNew(true);
                    setProfile({ id: '', name: '', enabled: true, sources: ['*'], tools: ['*'] });
                }}
            >
                {label('addClient', 'Add client profile')}
            </Button>
            {profiles.map((p) => (
                <Card variant="outlined" key={p.id}>
                    <CardContent>
                        <Stack spacing={1}>
                            <Typography variant="h6">{p.name || p.id}</Typography>
                            <Typography variant="body2">
                                {p.id} · {(p.sources || []).join(', ')} · {(p.tools || []).join(', ')}
                            </Typography>
                            <Stack direction="row">
                                <Button
                                    disabled={busy}
                                    onClick={() => {
                                        setIsNew(false);
                                        setProfile(p);
                                    }}
                                >
                                    {label('edit', 'Edit')}
                                </Button>
                                <Button onClick={() => setSelected(p.id)}>
                                    {label('install', 'Connection instructions')}
                                </Button>
                                <Button
                                    color="error"
                                    disabled={busy}
                                    onClick={() =>
                                        void run(async () => onSaved(await api.deleteMCPClientProfile(p.id)))
                                    }
                                >
                                    {label('delete', 'Delete')}
                                </Button>
                            </Stack>
                        </Stack>
                    </CardContent>
                </Card>
            ))}
            {(profiles.some((p) => p.id === selected) ||
                (!config.client_profiles_configured && profiles.length === 0)) && (
                <AgentInstallCard clientId={selected} heading={label('install', 'Connection instructions')} />
            )}
            <Dialog open={!!profile} onClose={() => !busy && setProfile(null)} maxWidth="md" fullWidth>
                <DialogTitle>{label('clientProfile', 'Client profile')}</DialogTitle>
                <DialogContent dividers>
                    {profile && (
                        <Stack spacing={2}>
                            {error && <Alert severity="error">{error}</Alert>}
                            <TextField
                                label={label('clientId', 'Client ID')}
                                value={profile.id}
                                disabled={!isNew}
                                onChange={(e) => setProfile({ ...profile, id: e.target.value })}
                            />
                            <TextField
                                label={label('name', 'Name')}
                                value={profile.name}
                                onChange={(e) => setProfile({ ...profile, name: e.target.value })}
                            />
                            <Autocomplete
                                multiple
                                options={['*', ...(config.sources || []).map((s) => s.id!)]}
                                value={profile.sources || []}
                                onChange={(_, values) => setProfile({ ...profile, sources: values })}
                                renderInput={(params) => (
                                    <TextField {...params} label={label('allowedServers', 'Allowed servers')} />
                                )}
                            />
                            <Autocomplete
                                multiple
                                options={[
                                    '*',
                                    ...catalog
                                        .flatMap((s) => s.tools || [])
                                        .filter((t) => t.usage.client)
                                        .map((t) => t.normalized_name),
                                ]}
                                value={profile.tools || []}
                                onChange={(_, values) => setProfile({ ...profile, tools: values })}
                                renderInput={(params) => (
                                    <TextField
                                        {...params}
                                        label={label('allowedTools', 'Allowed tools')}
                                        helperText={label(
                                            'emptyGrants',
                                            'Empty selections grant no access; * explicitly grants all eligible tools.'
                                        )}
                                    />
                                )}
                            />
                            <FormControlLabel
                                control={
                                    <Checkbox
                                        checked={profile.enabled !== false}
                                        onChange={(e) => setProfile({ ...profile, enabled: e.target.checked })}
                                    />
                                }
                                label={label('enabled', 'Enabled')}
                            />
                        </Stack>
                    )}
                </DialogContent>
                <DialogActions>
                    <Button disabled={busy} onClick={() => setProfile(null)}>
                        {label('cancel', 'Cancel')}
                    </Button>
                    <Button
                        disabled={busy || !profile?.id.trim()}
                        onClick={() =>
                            void run(async () => {
                                if (isNew && profiles.some((p) => p.id === profile!.id))
                                    throw new Error(label('duplicateId', 'This ID already exists.'));
                                onSaved(await api.saveMCPClientProfile(profile!));
                                setSelected(profile!.id);
                                setProfile(null);
                            })
                        }
                    >
                        {label('save', 'Save')}
                    </Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );
}
