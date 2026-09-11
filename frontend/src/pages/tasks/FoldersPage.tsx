// Folders the agent may work in. This list is the allowlist: only folders
// added here (or started a task in) can be listed by the picker, and the
// agent works in them in place — nothing is cloned, nothing is pushed,
// nothing is deleted. Repositories (clone → branch → push) are a later
// phase; that UI is parked in SourcesPage.tsx until the local flow is solid.
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
import {Add as IconAdd, FolderOpen as IconFolder, GitHub as IconRepo, LinkOff as IconRemove} from '@/components/icons';
import {useNotify} from '@/hooks/useNotify';
import {agentApi, type AgentSource, type RecentFolder} from '@/services/agentApi';

const isAbsolutePath = (v: string): boolean => /^(\/|[A-Za-z]:[\\/])/.test(v.trim());

const FoldersPage = () => {
    const {t} = useTranslation();
    const notify = useNotify();
    const [folders, setFolders] = useState<AgentSource[]>([]);
    const [repoFlags, setRepoFlags] = useState<Record<string, boolean>>({});
    const [loading, setLoading] = useState(true);
    const [adding, setAdding] = useState(false);
    const [path, setPath] = useState('');
    const [saving, setSaving] = useState(false);
    const [removing, setRemoving] = useState<AgentSource>();
    const [removeBusy, setRemoveBusy] = useState(false);

    const load = useCallback(async () => {
        const [src, recent] = await Promise.all([agentApi.listSources(), agentApi.recentFolders()]);
        if (src.ok) setFolders((src.data.sources ?? []).filter((s) => s.kind === 'local'));
        else notify.error(src.error);
        if (recent.ok) {
            const flags: Record<string, boolean> = {};
            for (const f of (recent.data.folders ?? []) as RecentFolder[]) flags[f.path] = f.is_repo;
            setRepoFlags(flags);
        }
        setLoading(false);
    }, [notify]);

    useEffect(() => {
        load();
    }, [load]);

    const add = async () => {
        setSaving(true);
        const res = await agentApi.createSource({url: path.trim(), name: '', default_branch: '', credential_id: ''});
        setSaving(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        notify.success(t('tasks.folders.added'));
        setAdding(false);
        setPath('');
        load();
    };

    const remove = async () => {
        if (!removing) return;
        setRemoveBusy(true);
        const res = await agentApi.deleteSource(removing.id);
        setRemoveBusy(false);
        if (!res.ok) {
            notify.error(res.error);
            return;
        }
        notify.success(t('tasks.folders.removed'));
        setRemoving(undefined);
        load();
    };

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <PageHeader
                    title={t('tasks.folders.title')}
                    subtitle={t('tasks.folders.subtitle')}
                    icon={<IconFolder />}
                    actions={
                        <Button variant="contained" startIcon={<IconAdd />} onClick={() => setAdding(true)}>
                            {t('tasks.folders.add')}
                        </Button>
                    }
                />
                {folders.length === 0 ? (
                    <EmptyState
                        compact
                        icon={<IconFolder />}
                        title={t('tasks.folders.empty')}
                        description={t('tasks.folders.emptyHint')}
                        primaryAction={{label: t('tasks.folders.add'), onClick: () => setAdding(true), icon: <IconAdd />}}
                    />
                ) : (
                    <Stack spacing={1.25}>
                        {folders.map((f) => (
                            <Card key={f.id} variant="outlined" sx={{p: {xs: 1.5, sm: 2}}}>
                                <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                                    <Stack sx={{flex: 1, minWidth: 0}}>
                                        <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                                            <Typography variant="subtitle1" sx={{fontWeight: 600}}>{f.name}</Typography>
                                            {repoFlags[f.url] && (
                                                <Chip size="small" variant="outlined" icon={<IconRepo />} label={t('tasks.folder.gitRepo')} />
                                            )}
                                        </Stack>
                                        <Typography variant="caption" color="text.secondary" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>
                                            {f.url}
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">{t('tasks.folders.inPlace')}</Typography>
                                    </Stack>
                                    <Tooltip title={t('tasks.sources.remove')}>
                                        <IconButton size="small" onClick={() => setRemoving(f)} aria-label={t('tasks.sources.remove')}><IconRemove fontSize="small" /></IconButton>
                                    </Tooltip>
                                </Stack>
                            </Card>
                        ))}
                    </Stack>
                )}
            </Stack>

            <Dialog open={adding} onClose={() => setAdding(false)} fullWidth maxWidth="sm" aria-labelledby="add-folder-title">
                <DialogHeader titleId="add-folder-title" title={t('tasks.folders.add')} closeLabel={t('common.close')} onClose={() => setAdding(false)} />
                <DialogContent>
                    <Stack spacing={2} sx={{pt: 1}}>
                        <TextField
                            label={t('tasks.folder.path')}
                            helperText={t('tasks.folders.addHelp')}
                            value={path}
                            onChange={(e) => setPath(e.target.value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter' && isAbsolutePath(path) && !saving) add();
                            }}
                            fullWidth
                            autoFocus
                            slotProps={{input: {sx: {fontFamily: 'monospace'}}}}
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setAdding(false)}>{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={add} disabled={saving || !isAbsolutePath(path)}>{t('tasks.folders.add')}</Button>
                </DialogActions>
            </Dialog>

            <ConfirmDialog
                open={!!removing}
                title={t('tasks.sources.removeLocalTitle')}
                description={t('tasks.sources.removeLocalConfirm')}
                confirmLabel={t('tasks.sources.remove')}
                loading={removeBusy}
                onClose={() => setRemoving(undefined)}
                onConfirm={remove}
            />
        </PageLayout>
    );
};

export default FoldersPage;
