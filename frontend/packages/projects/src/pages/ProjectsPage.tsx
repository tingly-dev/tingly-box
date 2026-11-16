import { useEffect, useState } from 'react';
import { Box, Button, InputAdornment, Skeleton, Tab, Tabs, TextField } from '@mui/material';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, isFolder, unwrap, type ProjectSummary } from '../client';
import { Add, Folder, Search } from '../components/icons';
import { ProjectCard } from '../components/project/ProjectCard';
import { NewProjectDialog } from '../components/project/NewProjectDialog';
import { useNotify } from '../hooks/useNotify';
import { useProjectsHost } from '../host/ProjectsHost';
import { EmptyState, PageHeader } from '@tingly/ui';

// Search shows once a tab lists more projects than this.
const SEARCH_FROM = 6;

export default function ProjectsPage() {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const notify = useNotify();
    const { onProjectsChanged: refresh, paths: routes, embedded } = useProjectsHost();
    const [tab, setTab] = useState<'active' | 'archived'>('active');
    const [q, setQ] = useState('');
    const [many, setMany] = useState(false);
    const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
    const [params, setParams] = useSearchParams();
    const [creating, setCreating] = useState(false);

    // The sidebar's "New project" lands here with ?new=1.
    useEffect(() => {
        if (params.get('new')) {
            setCreating(true);
            setParams({}, { replace: true });
        }
    }, [params, setParams]);

    useEffect(() => {
        let alive = true;
        const timer = window.setTimeout(() => {
            unwrap(api.GET('/api/v1/projects', { params: { query: { q: q || undefined, archived: tab === 'archived' || undefined } } }))
                .then((r) => {
                    if (!alive) return;
                    // Folders live in the sidebar's Folders group, not here.
                    const list = r.projects.filter((p) => !isFolder(p));
                    setProjects(list);
                    if (!q) setMany(list.length > SEARCH_FROM);
                })
                .catch((e: Error) => alive && notify(e.message, 'error'));
        }, q ? 200 : 0);
        return () => {
            alive = false;
            window.clearTimeout(timer);
        };
    }, [q, tab, notify]);

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            {/* Standalone, the top-left corner belongs to the floating sidebar
                button; a host's layout keeps its own navigation clear. */}
            <Box sx={{ maxWidth: 1040, mx: 'auto', px: { xs: 2, md: embedded ? 3 : 4 }, pt: embedded ? { xs: 2, md: 3 } : { xs: 7, md: 5 }, pb: 6 }}>
                <PageHeader
                    title={t('projects.title')}
                    subtitle={t('projects.intro')}
                    actions={<Button variant="contained" startIcon={<Add />} onClick={() => setCreating(true)}>{t('projects.new')}</Button>}
                    sx={{ mb: 3 }}
                />

                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3, borderBottom: 1, borderColor: 'divider' }}>
                    <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ flex: 1, minHeight: 40, '& .MuiTab-root': { minHeight: 40 } }}>
                        <Tab value="active" label={t('projects.tabActive')} />
                        <Tab value="archived" label={t('projects.tabArchived')} />
                    </Tabs>
                    {/* A handful of projects is read at a glance; search only once there are more. */}
                    {(many || q) && (
                        <TextField size="small" placeholder={t('projects.search')} value={q} onChange={(e) => setQ(e.target.value)}
                            slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }}
                            sx={{ width: { xs: 160, sm: 240 }, mb: 0.5 }} />
                    )}
                </Box>

                {projects === null ? (
                    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr', lg: '1fr 1fr 1fr' }, gap: 2 }}>
                        {[0, 1, 2].map((i) => <Skeleton key={i} variant="rounded" height={150} />)}
                    </Box>
                ) : projects.length === 0 ? (
                    <EmptyState
                        icon={<Folder sx={{ fontSize: 40 }} />}
                        title={q ? t('projects.noneFound') : tab === 'archived' ? t('projects.noArchived') : t('projects.emptyTitle')}
                        description={!q && tab === 'active' ? t('projects.emptyBody') : undefined}
                        primaryAction={!q && tab === 'active' ? { label: t('projects.new'), onClick: () => setCreating(true) } : undefined}
                    />
                ) : (
                    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr', lg: '1fr 1fr 1fr' }, gap: 2 }}>
                        {projects.map((p) => <ProjectCard key={p.id} p={p} />)}
                    </Box>
                )}
            </Box>
            <NewProjectDialog
                open={creating}
                onClose={() => setCreating(false)}
                onCreate={async (v) => {
                    try {
                        const p = await unwrap(api.POST('/api/v1/projects', { body: v }));
                        refresh();
                        navigate(routes.project(p.id));
                    } catch (e) {
                        notify((e as Error).message, 'error');
                        throw e;
                    }
                }}
            />
        </Box>
    );
}
