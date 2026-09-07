// Where the agent runs. The list shows concrete values (runtime, profile
// name) rather than "default"; the runtime picker offers docker only as a
// visibly disabled option so the future is legible without being a dead end.
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Button, Card, Chip, Dialog, DialogActions, DialogContent, FormControl, IconButton, InputLabel, MenuItem,
    Select, Stack, TextField, Tooltip, Typography,
} from '@mui/material';
import {PageLayout} from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import EmptyState from '@/components/EmptyState';
import ConfirmDialog from '@/components/ConfirmDialog';
import DialogHeader from '@/components/DialogHeader';
import {Add as IconAdd, Computer as IconEnv, Delete as IconDelete, Edit as IconEdit} from '@/components/icons';
import {useNotify} from '@/hooks/useNotify';
import {agentApi, type AgentEnvironment, type EnvironmentRequest, type PermissionMode} from '@/services/agentApi';
import {PermissionModeSelect} from './taskShared';

const emptyForm = (): EnvironmentRequest => ({
    name: '', runtime: 'local', image: '', setup_script: '', env: {}, secret_refs: [], network: 'none',
    resources: {}, cc_profile: '', permission_mode: '',
});

const envToLines = (env?: Record<string, string>): string =>
    Object.entries(env ?? {}).map(([k, v]) => `${k}=${v}`).join('\n');

const linesToEnv = (lines: string): Record<string, string> => {
    const out: Record<string, string> = {};
    for (const raw of lines.split('\n')) {
        const line = raw.trim();
        if (!line || line.startsWith('#')) continue;
        const i = line.indexOf('=');
        if (i <= 0) continue;
        out[line.slice(0, i).trim()] = line.slice(i + 1);
    }
    return out;
};

const EnvironmentsPage = () => {
    const {t} = useTranslation();
    const notify = useNotify();
    const [environments, setEnvironments] = useState<AgentEnvironment[]>([]);
    const [supported, setSupported] = useState<string[]>(['local']);
    const [loading, setLoading] = useState(true);
    const [editing, setEditing] = useState<AgentEnvironment | null | undefined>(undefined);
    const [form, setForm] = useState<EnvironmentRequest>(emptyForm());
    const [envText, setEnvText] = useState('');
    const [saving, setSaving] = useState(false);
    const [deleting, setDeleting] = useState<AgentEnvironment>();
    const [deleteBusy, setDeleteBusy] = useState(false);

    const load = useCallback(async () => {
        const res = await agentApi.listEnvironments();
        if (res.ok) {
            setEnvironments(res.data.environments ?? []);
            setSupported(res.data.supported_runtimes ?? ['local']);
        } else {
            notify.error(res.error);
        }
        setLoading(false);
    }, [notify]);

    useEffect(() => {
        load();
    }, [load]);

    const open = (env: AgentEnvironment | null) => {
        setEditing(env);
        const f = env
            ? {
                name: env.name, runtime: env.runtime, image: env.image ?? '', setup_script: env.setup_script ?? '',
                env: env.env ?? {}, secret_refs: env.secret_refs ?? [], network: env.network ?? 'none',
                resources: env.resources ?? {}, cc_profile: env.cc_profile ?? '', permission_mode: env.permission_mode ?? '',
            }
            : emptyForm();
        setForm(f);
        setEnvText(envToLines(f.env));
    };

    const save = async () => {
        setSaving(true);
        const body = {...form, env: linesToEnv(envText)};
        const res = editing ? await agentApi.updateEnvironment(editing.id, body) : await agentApi.createEnvironment(body);
        setSaving(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        notify.success(t('tasks.environments.saved'));
        setEditing(undefined);
        load();
    };

    const remove = async () => {
        if (!deleting) return;
        setDeleteBusy(true);
        const res = await agentApi.deleteEnvironment(deleting.id);
        setDeleteBusy(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        notify.success(t('tasks.environments.removed'));
        setDeleting(undefined);
        load();
    };

    const isDocker = form.runtime === 'docker';

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <PageHeader
                    title={t('tasks.environments.title')}
                    subtitle={t('tasks.environments.subtitle')}
                    icon={<IconEnv />}
                    actions={
                        <Button variant="contained" startIcon={<IconAdd />} onClick={() => open(null)}>
                            {t('tasks.environments.add')}
                        </Button>
                    }
                />
                {environments.length === 0 ? (
                    <EmptyState compact icon={<IconEnv />} title={t('tasks.environments.empty')} />
                ) : (
                    <Stack spacing={1.25}>
                        {environments.map((e) => (
                            <Card key={e.id} variant="outlined" sx={{p: {xs: 1.5, sm: 2}}}>
                                <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                                    <Stack sx={{flex: 1, minWidth: 0}} spacing={0.5}>
                                        <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                                            <Typography variant="subtitle1" sx={{fontWeight: 600}}>{e.name}</Typography>
                                            {e.is_default && <Chip size="small" label={t('tasks.environments.default')} />}
                                        </Stack>
                                        <Stack direction="row" spacing={1} sx={{flexWrap: 'wrap', rowGap: 0.5}}>
                                            <Chip size="small" variant="outlined" label={e.runtime === 'docker' ? `docker · ${e.image}` : t('tasks.environments.runtimeLocal')} />
                                            {e.cc_profile && <Chip size="small" variant="outlined" label={e.cc_profile} />}
                                            {e.permission_mode && <Chip size="small" variant="outlined" label={t(`tasks.mode.${e.permission_mode}`)} />}
                                            {e.env && Object.keys(e.env).length > 0 && (
                                                <Chip size="small" variant="outlined" label={`${Object.keys(e.env).length} env`} />
                                            )}
                                        </Stack>
                                    </Stack>
                                    <Tooltip title={t('common.edit')}>
                                        <IconButton size="small" onClick={() => open(e)}><IconEdit fontSize="small" /></IconButton>
                                    </Tooltip>
                                    {!e.is_default && (
                                        <Tooltip title={t('common.delete')}>
                                            <IconButton size="small" onClick={() => setDeleting(e)}><IconDelete fontSize="small" /></IconButton>
                                        </Tooltip>
                                    )}
                                </Stack>
                            </Card>
                        ))}
                    </Stack>
                )}
            </Stack>

            <Dialog open={editing !== undefined} onClose={() => setEditing(undefined)} fullWidth maxWidth="sm" aria-labelledby="env-dialog-title">
                <DialogHeader
                    titleId="env-dialog-title"
                    title={editing ? t('tasks.environments.edit') : t('tasks.environments.add')}
                    closeLabel={t('common.close')}
                    onClose={() => setEditing(undefined)}
                />
                <DialogContent>
                    <Stack spacing={2} sx={{pt: 1}}>
                        <TextField
                            label={t('tasks.environments.name')}
                            value={form.name}
                            onChange={(e) => setForm({...form, name: e.target.value})}
                            fullWidth
                            autoFocus
                        />
                        <FormControl fullWidth size="small">
                            <InputLabel id="env-runtime">{t('tasks.environments.runtime')}</InputLabel>
                            <Select
                                labelId="env-runtime"
                                label={t('tasks.environments.runtime')}
                                value={form.runtime}
                                onChange={(e) => setForm({...form, runtime: e.target.value as EnvironmentRequest['runtime']})}
                            >
                                <MenuItem value="local">{t('tasks.environments.runtimeLocal')}</MenuItem>
                                <MenuItem value="docker" disabled={!supported.includes('docker')}>{t('tasks.environments.runtimeDocker')}</MenuItem>
                            </Select>
                        </FormControl>
                        {isDocker && (
                            <TextField
                                label={t('tasks.environments.image')}
                                value={form.image}
                                onChange={(e) => setForm({...form, image: e.target.value})}
                                fullWidth
                            />
                        )}
                        <PermissionModeSelect
                            value={form.permission_mode ?? ''}
                            onChange={(m: PermissionMode) => setForm({...form, permission_mode: m})}
                        />
                        <TextField
                            label={t('tasks.environments.ccProfile')}
                            helperText={t('tasks.environments.ccProfileHelp')}
                            placeholder="claude_code:<profile id>"
                            value={form.cc_profile}
                            onChange={(e) => setForm({...form, cc_profile: e.target.value})}
                            fullWidth
                        />
                        <TextField
                            label={t('tasks.environments.env')}
                            helperText={t('tasks.environments.envHelp')}
                            value={envText}
                            onChange={(e) => setEnvText(e.target.value)}
                            multiline
                            minRows={2}
                            fullWidth
                            slotProps={{input: {sx: {fontFamily: 'monospace'}}}}
                        />
                        <TextField
                            label={t('tasks.environments.setupScript')}
                            helperText={t('tasks.environments.setupScriptHelp')}
                            value={form.setup_script}
                            onChange={(e) => setForm({...form, setup_script: e.target.value})}
                            multiline
                            minRows={2}
                            fullWidth
                            slotProps={{input: {sx: {fontFamily: 'monospace'}}}}
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setEditing(undefined)}>{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={save} disabled={saving || !form.name.trim()}>{t('common.save')}</Button>
                </DialogActions>
            </Dialog>

            <ConfirmDialog
                open={!!deleting}
                title={t('tasks.environments.deleteConfirmTitle')}
                description={t('tasks.environments.deleteConfirm')}
                confirmLabel={t('common.delete')}
                confirmColor="error"
                loading={deleteBusy}
                onClose={() => setDeleting(undefined)}
                onConfirm={remove}
            />
        </PageLayout>
    );
};

export default EnvironmentsPage;
