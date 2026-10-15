import { useEffect, useState } from 'react';
import { Box, Button, Card, CardActionArea, InputAdornment, Skeleton, Tab, Tabs, TextField, Typography } from '@mui/material';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ProjectSummary } from '../client';
import { Add, Branch, Folder, Pause, Search, Star } from '../components/icons';
import { NewProjectDialog } from '../components/project/NewProjectDialog';
import { useNotify } from '../hooks/useNotify';
import { useProjectsHost } from '../host/ProjectsHost';
import { timeAgo } from '../utils/format';
import { EmptyState, PageHeader } from '@tingly/ui';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { toneColor } from '../components/project/threadStatus';

function ProjectCard({ p }: { p: ProjectSummary }) {
    const { t, i18n } = useTranslation('projects');
    const navigate = useNavigate();
    const { paths: routes } = useProjectsHost();
    const c = p.threads;
    const counts: { key: string; n: number; color: 'warning' | 'success' | 'info' | 'default' }[] = [
        { key: 'waiting', n: c.waiting, color: 'warning' },
        { key: 'ready', n: c.ready, color: 'success' },
        { key: 'working', n: c.working, color: 'info' },
    ];
    return (
        <Card variant="outlined" sx={{ borderRadius: 3, '&:hover': { borderColor: 'text.disabled' } }}>
            <CardActionArea onClick={() => navigate(routes.project(p.id))} sx={{ p: 2.25, height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'stretch', justifyContent: 'flex-start' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="h5" noWrap sx={{ flex: 1 }}>{p.name}</Typography>
                    {p.paused && <Pause sx={{ fontSize: 16 }} color="warning" />}
                    {p.starred && <Star sx={{ fontSize: 16 }} color="primary" />}
                </Box>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, minHeight: 40, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>
                    {p.goal || t('projects.noGoal')}
                </Typography>
                {/* What needs you, in one quiet line: a dot per state that has threads. */}
                <Box sx={{ display: 'flex', columnGap: 1.5, rowGap: 0.5, flexWrap: 'wrap', mt: 1.5, minHeight: 20, fontSize: fontSizes.sm }}>
                    {counts.filter((x) => x.n > 0).map((x) => (
                        <Box key={x.key} component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, color: x.key === 'waiting' ? toneColor(x.color) : 'text.secondary', fontWeight: x.key === 'waiting' ? 600 : 400 }}>
                            <Box component="span" sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: toneColor(x.color) }} />
                            {x.n} {t(`overview.group.${x.key}`)}
                        </Box>
                    ))}
                </Box>
                {p.repos.length > 0 && (
                    <Typography variant="caption" color="text.secondary" noWrap sx={{ mt: 0.75, display: 'flex', alignItems: 'center', gap: 0.5 }}>
                        <Branch sx={{ fontSize: 13 }} />{p.repos.map((r) => r.path.split('/').pop()).join(' · ')}
                    </Typography>
                )}
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>{t('projects.updated', { time: timeAgo(p.updated_at, i18n.language) })}</Typography>
            </CardActionArea>
        </Card>
    );
}

export default function ProjectsPage() {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const notify = useNotify();
    const { onProjectsChanged: refresh, paths: routes, embedded } = useProjectsHost();
    const [tab, setTab] = useState<'active' | 'archived'>('active');
    const [q, setQ] = useState('');
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
                .then((r) => alive && setProjects(r.projects))
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

                <TextField fullWidth placeholder={t('projects.search')} value={q} onChange={(e) => setQ(e.target.value)}
                    slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }}
                    sx={{ mb: 2, '& .MuiOutlinedInput-root': { bgcolor: 'background.paper' } }} />
                <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 3, minHeight: 40, borderBottom: 1, borderColor: 'divider', '& .MuiTab-root': { minHeight: 40 } }}>
                    <Tab value="active" label={t('projects.tabActive')} />
                    <Tab value="archived" label={t('projects.tabArchived')} />
                </Tabs>

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
