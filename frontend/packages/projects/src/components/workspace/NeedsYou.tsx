// The top of the workspace home: what waits on the user across every project
// and folder, answered where it is shown. An approval or a question is the
// thread page's own card (the same one a thread's card opens); a failed turn
// or a report that needs a reply is a row that opens the thread.
import { useState } from 'react';
import { Box, Button, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { getReadableAccent } from '@tingly/ui/theme/status';
import type { ProjectSummary, Thread } from '../../client';
import { ThreadRequestsProvider } from '../../hooks/threadRequests';
import { useNeedsYou } from '../../hooks/useNeedsYou';
import { useProjectsHost } from '../../host/ProjectsHost';
import { threadLocation } from '../../notifications/engine';
import { folderName } from '../../utils/format';
import { ArrowForward } from '../icons';
import { DisclosureChevron, DisclosureRow } from '../common/Disclosure';
import { RequestCards, ThreadRequestSubtitle } from '../project/ThreadRequests';
import { ThreadStateMark, stateColor } from '../project/threadStatus';

/** How many threads show before "Show all"; the rest wait behind it. */
export const NEEDS_YOU_SHOWN = 3;

const placeName = (p: ProjectSummary | undefined) => (p ? p.name || folderName(p.path ?? '') : '');

export function NeedsYou() {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const { paths } = useProjectsHost();
    const { threads, projects, requests } = useNeedsYou();
    const [all, setAll] = useState(false);
    // What the user opened or folded; until they touch a row, the first one with something to answer stays open.
    const [toggled, setToggled] = useState<Record<string, boolean>>({});
    if (threads.length === 0) return null;
    const first = threads.find((th) => (requests.byThread[th.id]?.length ?? 0) > 0);
    const shown = all ? threads : threads.slice(0, NEEDS_YOU_SHOWN);
    const open = (thread: Thread) => {
        const project = projects.get(thread.project_id);
        if (project) navigate(threadLocation(paths, project, thread));
    };

    return (
        <ThreadRequestsProvider value={requests}>
            <Box component="section" aria-label={t('needsYou.title')} sx={{ mb: 5, p: { xs: 1.5, sm: 2 }, border: 1, borderColor: 'divider', borderRadius: radius.card, bgcolor: 'background.paper' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1.5 }}>
                    <Typography variant="h5" component="h2" sx={{ flex: 1 }}>{t('needsYou.title')}</Typography>
                    <Box component="span" sx={{ color: (theme) => getReadableAccent(theme, 'warning'), fontSize: fontSizes.sm, fontWeight: 600 }}>{threads.length}</Box>
                </Box>
                <Box sx={{ display: 'grid', gap: 1.25 }}>
                    {shown.map((thread) => {
                        const pending = requests.byThread[thread.id] ?? [];
                        const place = placeName(projects.get(thread.project_id));
                        const foldable = pending.length > 0;
                        const expanded = foldable && (toggled[thread.id] ?? thread.id === first?.id);
                        const failed = thread.status === 'failed';
                        return (
                            <Box key={thread.id} sx={{ border: 1, borderColor: 'divider', borderRadius: radius.row, minWidth: 0 }}>
                                <DisclosureRow
                                    variant="header"
                                    chevron={false}
                                    open={expanded}
                                    onToggle={foldable ? () => setToggled({ ...toggled, [thread.id]: !expanded }) : undefined}
                                    aria-label={thread.title}
                                    sx={{ px: 1.25, py: 1, gap: 1, alignItems: 'flex-start' }}
                                >
                                    <Box sx={{ pt: 0.5, color: stateColor(thread.status) }}><ThreadStateMark status={thread.status} /></Box>
                                    <Box sx={{ flex: 1, minWidth: 0 }}>
                                        <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 0.75, minWidth: 0 }}>
                                            <Typography noWrap title={thread.title} sx={{ flexShrink: 1, minWidth: 0, fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary' }}>{thread.title}</Typography>
                                            {place && <Typography noWrap title={place} sx={{ flex: '0 1 auto', minWidth: 0, fontSize: fontSizes.sm, color: 'text.secondary' }}>· {place}</Typography>}
                                        </Box>
                                        <Typography noWrap component="div" sx={{ mt: 0.25, fontSize: fontSizes.sm, color: 'text.secondary' }}>
                                            {failed && <Box component="span" sx={{ mr: 0.75, color: (theme) => getReadableAccent(theme, 'error'), fontWeight: 500 }}>{t('needsYou.failed')}</Box>}
                                            <ThreadRequestSubtitle thread={thread} />
                                        </Typography>
                                    </Box>
                                    <Button
                                        size="small"
                                        color="inherit"
                                        endIcon={<ArrowForward sx={{ fontSize: 14 }} />}
                                        sx={{ flexShrink: 0, color: 'text.secondary' }}
                                        onClick={(e) => { e.stopPropagation(); open(thread); }}
                                    >
                                        {t('needsYou.open')}
                                    </Button>
                                    {foldable && <DisclosureChevron open={expanded} sx={{ mt: 0.75 }} />}
                                </DisclosureRow>
                                {expanded && <Box sx={{ px: 1.25, pb: 1.25 }}><RequestCards thread={thread} requests={pending} /></Box>}
                            </Box>
                        );
                    })}
                </Box>
                {threads.length > NEEDS_YOU_SHOWN && (
                    <Button size="small" color="inherit" sx={{ mt: 1, color: 'text.secondary' }} onClick={() => setAll(!all)}>
                        {all ? t('needsYou.fewer') : t('needsYou.more', { count: threads.length - NEEDS_YOU_SHOWN })}
                    </Button>
                )}
            </Box>
        </ThreadRequestsProvider>
    );
}
