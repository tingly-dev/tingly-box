// Repositories the agent can be pointed at. Credentials are deliberately
// not a field yet: cloning and pushing use this machine's git configuration
// (.design/managed-agent.md §5.3); a source-bound credential comes with the
// GitHub integration.
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Button, Card, Chip, Dialog, DialogActions, DialogContent, IconButton, Stack, TextField, Tooltip, Typography,
} from '@mui/material';
import {PageLayout} from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import EmptyState from '@/components/EmptyState';
import ConfirmDialog from '@/components/ConfirmDialog';
import DialogHeader from '@/components/DialogHeader';
import {Add as IconAdd, Delete as IconDelete, Edit as IconEdit, FolderOpen as IconFolder, GitHub as IconRepo} from '@/components/icons';
import {useNotify} from '@/hooks/useNotify';
import {agentApi, type AgentSource, type SourceRequest} from '@/services/agentApi';

const emptyForm: SourceRequest = {name: '', url: '', default_branch: '', credential_id: ''};

// Mirrors the backend rule: an absolute path is a local directory source.
const isAbsolutePath = (v: string): boolean => /^(\/|[A-Za-z]:[\\/])/.test(v.trim());

const SourcesPage = () => {
    const {t} = useTranslation();
    const notify = useNotify();
    const [sources, setSources] = useState<AgentSource[]>([]);
    const [loading, setLoading] = useState(true);
    const [editing, setEditing] = useState<AgentSource | null | undefined>(undefined); // undefined = closed, null = new
    const [form, setForm] = useState<SourceRequest>(emptyForm);
    const [saving, setSaving] = useState(false);
    const [deleting, setDeleting] = useState<AgentSource>();
    const [deleteBusy, setDeleteBusy] = useState(false);

    const load = useCallback(async () => {
        const res = await agentApi.listSources();
        if (res.ok) setSources(res.data.sources ?? []);
        else notify.error(res.error);
        setLoading(false);
    }, [notify]);

    useEffect(() => {
        load();
    }, [load]);

    const open = (src: AgentSource | null) => {
        setEditing(src);
        setForm(src ? {name: src.name, url: src.url, default_branch: src.default_branch, credential_id: src.credential_id ?? ''} : emptyForm);
    };

    const save = async () => {
        setSaving(true);
        const res = editing ? await agentApi.updateSource(editing.id, form) : await agentApi.createSource(form);
        setSaving(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        notify.success(t('tasks.sources.saved'));
        setEditing(undefined);
        load();
    };

    const remove = async () => {
        if (!deleting) return;
        setDeleteBusy(true);
        const res = await agentApi.deleteSource(deleting.id);
        setDeleteBusy(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        notify.success(t('tasks.sources.removed'));
        setDeleting(undefined);
        load();
    };

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <PageHeader
                    title={t('tasks.sources.title')}
                    subtitle={t('tasks.sources.subtitle')}
                    icon={<IconRepo />}
                    actions={
                        <Button variant="contained" startIcon={<IconAdd />} onClick={() => open(null)}>
                            {t('tasks.sources.add')}
                        </Button>
                    }
                />
                {sources.length === 0 ? (
                    <EmptyState
                        compact
                        icon={<IconRepo />}
                        title={t('tasks.sources.empty')}
                        description={t('tasks.sources.emptyHint')}
                        primaryAction={{label: t('tasks.sources.add'), onClick: () => open(null), icon: <IconAdd />}}
                    />
                ) : (
                    <Stack spacing={1.25}>
                        {sources.map((s) => (
                            <Card key={s.id} variant="outlined" sx={{p: {xs: 1.5, sm: 2}}}>
                                <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                                    <Stack sx={{flex: 1, minWidth: 0}}>
                                        <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                                            <Typography variant="subtitle1" sx={{fontWeight: 600}}>{s.name}</Typography>
                                            <Chip
                                                size="small"
                                                variant="outlined"
                                                icon={s.kind === 'local' ? <IconFolder /> : <IconRepo />}
                                                label={s.kind === 'local' ? t('tasks.sources.kindLocal') : t('tasks.sources.kindGit')}
                                            />
                                        </Stack>
                                        <Typography variant="caption" color="text.secondary" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>
                                            {s.url}
                                        </Typography>
                                        {s.kind === 'local' ? (
                                            <Typography variant="caption" color="text.secondary">{t('tasks.sources.inPlaceNote')}</Typography>
                                        ) : (
                                            <Typography variant="caption" color="text.secondary">
                                                {t('tasks.sources.defaultBranch')}: {s.default_branch}
                                            </Typography>
                                        )}
                                    </Stack>
                                    <Tooltip title={t('common.edit')}>
                                        <IconButton size="small" onClick={() => open(s)}><IconEdit fontSize="small" /></IconButton>
                                    </Tooltip>
                                    <Tooltip title={t('common.delete')}>
                                        <IconButton size="small" onClick={() => setDeleting(s)}><IconDelete fontSize="small" /></IconButton>
                                    </Tooltip>
                                </Stack>
                            </Card>
                        ))}
                    </Stack>
                )}
            </Stack>

            <Dialog open={editing !== undefined} onClose={() => setEditing(undefined)} fullWidth maxWidth="sm" aria-labelledby="source-dialog-title">
                <DialogHeader
                    titleId="source-dialog-title"
                    title={editing ? t('tasks.sources.edit') : t('tasks.sources.add')}
                    closeLabel={t('common.close')}
                    onClose={() => setEditing(undefined)}
                />
                <DialogContent>
                    <Stack spacing={2} sx={{pt: 1}}>
                        <TextField
                            label={t('tasks.sources.url')}
                            helperText={t('tasks.sources.urlHelp')}
                            value={form.url}
                            onChange={(e) => setForm({...form, url: e.target.value})}
                            fullWidth
                            autoFocus
                        />
                        <TextField
                            label={t('tasks.sources.name')}
                            value={form.name}
                            onChange={(e) => setForm({...form, name: e.target.value})}
                            fullWidth
                        />
                        {/* An absolute path means "work in this directory in place":
                            there is no clone and no branch, so the field disappears. */}
                        {!isAbsolutePath(form.url) && (
                            <TextField
                                label={t('tasks.sources.defaultBranch')}
                                placeholder="main"
                                value={form.default_branch}
                                onChange={(e) => setForm({...form, default_branch: e.target.value})}
                                fullWidth
                            />
                        )}
                        {isAbsolutePath(form.url) && (
                            <Typography variant="body2" color="text.secondary">{t('tasks.sources.inPlaceHint')}</Typography>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setEditing(undefined)}>{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={save} disabled={saving || !form.url.trim()}>{t('common.save')}</Button>
                </DialogActions>
            </Dialog>

            <ConfirmDialog
                open={!!deleting}
                title={t('tasks.sources.deleteConfirmTitle')}
                description={t('tasks.sources.deleteConfirm')}
                confirmLabel={t('common.delete')}
                confirmColor="error"
                loading={deleteBusy}
                onClose={() => setDeleting(undefined)}
                onConfirm={remove}
            />
        </PageLayout>
    );
};

export default SourcesPage;
