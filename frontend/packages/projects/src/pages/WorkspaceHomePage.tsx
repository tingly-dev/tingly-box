import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';
import { Box, Button, Popover, Skeleton, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { PageHeader, Surface } from '@tingly/ui';
import { radius } from '@tingly/ui/theme/radius';
import type { ProjectSummary } from '../client';
import { Add, ArrowForward, ChatBubble, FolderOpen, Stack } from '../components/icons';
import { ImChatCard } from '../components/im/ImChatCard';
import { useChannelBindings, useProjectDirectory } from '../components/im/useImBindings';
import { ProjectCard } from '../components/project/ProjectCard';
import { WorkdirCard } from '../components/workdir/WorkdirCard';
import { useProjectList } from '../hooks/useProjectList';
import { useProjectsHost } from '../host/ProjectsHost';
import { readSession, writeSession } from '../utils/storage';
import { disarmWorkspaceImHint, dismissWorkspaceImHint, imHintArmed, imHintDismissed } from '../utils/imHint';

/** How many recent items each section shows before "View all". */
export const RECENT = 3;

// While the first-visit hint at the IM section is up but not dismissed, the
// rest of this visit (the tab) stays quiet — a refresh included.
const IM_HINT_THIS_VISIT = 'workspace.imHint.seen';

const recent = (list: ProjectSummary[]) => [...list].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, RECENT);

function Section({ icon, title, body, action, onAction, viewAll, onViewAll, headerRef, steps, loading, children }: {
    icon: ReactNode;
    title: string;
    body: string;
    action: string;
    onAction: () => void;
    viewAll?: string;
    onViewAll?: () => void;
    /** Where a pointing hint can anchor (the IM section's first-open hint). */
    headerRef?: RefObject<HTMLDivElement | null>;
    /** Shown instead of cards while the section is empty. */
    steps: string[] | null;
    loading: boolean;
    children?: ReactNode;
}) {
    return (
        <Box component="section" aria-label={title} sx={{ mb: 5 }}>
            <Box ref={headerRef} sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, mb: 2, flexWrap: 'wrap' }}>
                <Box sx={{ flex: 1, minWidth: 240 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, color: 'text.primary' }}>
                        {icon}
                        <Typography variant="h5" component="h2">{title}</Typography>
                    </Box>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{body}</Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    {steps === null && !loading && onViewAll && (
                        <Button size="small" color="inherit" endIcon={<ArrowForward />} onClick={onViewAll} sx={{ color: 'text.secondary' }}>{viewAll}</Button>
                    )}
                    <Button variant="contained" startIcon={<Add />} onClick={onAction}>{action}</Button>
                </Box>
            </Box>
            {loading ? (
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr', lg: '1fr 1fr 1fr' }, gap: 2 }}>
                    {[0, 1, 2].map((i) => <Skeleton key={i} variant="rounded" height={120} />)}
                </Box>
            ) : steps ? (
                <Surface variant="soft">
                    <Box component="ol" sx={{ m: 0, p: 0, listStyle: 'none', display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 2 }}>
                        {steps.map((s, i) => (
                            <Box component="li" key={i} sx={{ display: 'flex', gap: 1.25 }}>
                                <Box sx={{ flexShrink: 0, width: 22, height: 22, borderRadius: '50%', bgcolor: 'action.selected', display: 'grid', placeItems: 'center', fontSize: 12, fontWeight: 600 }}>{i + 1}</Box>
                                <Typography variant="body2" color="text.secondary">{s}</Typography>
                            </Box>
                        ))}
                    </Box>
                </Surface>
            ) : (
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr', lg: '1fr 1fr 1fr' }, gap: 2 }}>{children}</Box>
            )}
        </Box>
    );
}

/**
 * Where Workspace opens: Projects and Workdir side by side as entry points,
 * with IM binding underneath when the host has it. Each shows its most recent
 * items once the user has any, and a three-step guide until then. The full
 * pages stay as they are (.design/workspace-home.md).
 */
export default function WorkspaceHomePage() {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const { paths: routes, embedded, im } = useProjectsHost();
    const hasIm = !!(im && routes.im);
    const { projects, folders } = useProjectList();
    const { bindings } = useChannelBindings(hasIm);
    const directory = useProjectDirectory(hasIm);
    const steps = (prefix: 'projects' | 'workdir' | 'im') => [1, 2, 3].map((n) => t(`workspace.${prefix}Step${n}`));

    // The IM entry sits below the fold; when the Workspace power-up is turned
    // on, the first render of this page scrolls the section into view and a
    // small hint points at it. The enable action arms the hint; dismissed is
    // remembered for good; while it is up but not dismissed, the rest of this
    // visit stays quiet (a refresh included).
    const [imHintAnchor, setImHintAnchor] = useState<HTMLElement | null>(null);
    const imHeader = useRef<HTMLDivElement>(null);
    useEffect(() => {
        // The dismissed check here is belt and braces: armWorkspaceImHint
        // already refuses to arm a dismissed hint.
        if (!hasIm || imHintDismissed() || !imHintArmed() || readSession(IM_HINT_THIS_VISIT) === '1') return;
        const timer = window.setTimeout(() => {
            // Marked only once actually shown, so a strict double render still shows it.
            writeSession(IM_HINT_THIS_VISIT, '1');
            disarmWorkspaceImHint();
            imHeader.current?.scrollIntoView({ block: 'center', behavior: 'smooth' });
            setImHintAnchor(imHeader.current);
        }, 500);
        return () => window.clearTimeout(timer);
    }, [hasIm]);
    const closeImHint = () => {
        setImHintAnchor(null);
        dismissWorkspaceImHint();
    };

    const recentBindings = [...(bindings ?? [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, RECENT);

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box sx={{ maxWidth: 1040, mx: 'auto', px: { xs: 2, md: embedded ? 3 : 4 }, pt: embedded ? { xs: 2, md: 3 } : { xs: 7, md: 5 }, pb: 6 }}>
                <PageHeader title={t('workspace.title')} subtitle={t('workspace.intro')} sx={{ mb: 4 }} />

                <Section
                    icon={<Stack sx={{ fontSize: 22 }} />}
                    title={t('nav.projects')}
                    body={t('workspace.projectsBody')}
                    action={t('projects.new')}
                    onAction={() => navigate(`${routes.list}?new=1`)}
                    viewAll={t('workspace.viewAllProjects')}
                    onViewAll={() => navigate(routes.list)}
                    steps={projects && projects.length === 0 ? steps('projects') : null}
                    loading={projects === null}
                >
                    {recent(projects ?? []).map((p) => <ProjectCard key={p.id} p={p} />)}
                </Section>

                <Section
                    icon={<FolderOpen sx={{ fontSize: 22 }} />}
                    title={t('workdir.title')}
                    body={t('workspace.workdirBody')}
                    action={t('workdir.add')}
                    onAction={() => navigate(`${routes.folder()}${routes.folder().includes('?') ? '&' : '?'}add=1`)}
                    viewAll={t('workspace.viewAllWorkdirs')}
                    onViewAll={() => navigate(routes.folder())}
                    steps={folders && folders.length === 0 ? steps('workdir') : null}
                    loading={folders === null}
                >
                    {recent(folders ?? []).map((f) => <WorkdirCard key={f.id} f={f} />)}
                </Section>

                {hasIm && (
                    <Section
                        icon={<ChatBubble sx={{ fontSize: 22 }} />}
                        title={t('im.title')}
                        body={t('workspace.imBody')}
                        action={t('workspace.imAction')}
                        onAction={() => navigate(routes.im!)}
                        headerRef={imHeader}
                        steps={bindings !== null && bindings.length === 0 ? steps('im') : null}
                        loading={bindings === null}
                    >
                        {recentBindings.map((b) => <ImChatCard key={b.id} b={b} place={directory.get(b.project_id)} />)}
                    </Section>
                )}
            </Box>

            {hasIm && (
                <Popover
                    open={!!imHintAnchor}
                    anchorEl={imHintAnchor}
                    onClose={closeImHint}
                    anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                    transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                    slotProps={{ paper: { elevation: 8, sx: { maxWidth: 380, p: 2, borderRadius: radius.card } } }}
                >
                    <Typography sx={{ fontWeight: 600, mb: 0.5 }}>{t('workspace.imHintTitle')}</Typography>
                    <Typography variant="body2" color="text.secondary">{t('workspace.imHintBody')}</Typography>
                    <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 1.5 }}>
                        <Button size="small" onClick={closeImHint}>{t('workspace.imHintGotIt')}</Button>
                    </Box>
                </Popover>
            )}
        </Box>
    );
}
