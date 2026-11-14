import { useState } from 'react';
import { Box, Tab, Tabs } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { needsUser } from './threadStatus';
import { ThreadsView } from '../workspace/ThreadsView';
import { Library } from './Library';
import { ProjectLinksCard } from './LinksCard';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

export type OverviewTab = 'threads' | 'library';

export function OverviewPane({ projectId, threads, onOpen, onTasks, onStatus, libraryKey, selectedId, initialTab = 'threads' }: { projectId: string; threads: Thread[]; onOpen: (id: string) => void; /** Opens a thread's running tasks from its card's count pill, or all of them from the toolbar's chip. */ onTasks?: (threads: Thread[], anchor: HTMLElement) => void; /** Opens a status's thread list from its group heading or column. */ onStatus?: (status: string, anchor: HTMLElement) => void; libraryKey: number; selectedId?: string; initialTab?: OverviewTab }) {
    const { t } = useTranslation('projects');
    const [tab, setTab] = useState<OverviewTab>(initialTab);
    const waiting = threads.filter((th) => needsUser(th.status)).length;
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            {/* Tabs only: a control of one view does not live in this row, where a narrow pane would squeeze it or a tab out. */}
            <Box sx={{ borderBottom: tab === 'threads' ? 0 : 1, borderColor: 'divider' }}>
                <Tabs value={tab} onChange={(_, v) => setTab(v)} variant="scrollable" scrollButtons="auto" sx={{ px: 1, minHeight: 44, '& .MuiTab-root': { minHeight: 44, minWidth: 0 } }}>
                    <Tab value="threads" aria-label={waiting > 0 ? t('overview.threadsWaiting', { count: waiting }) : undefined}
                        label={<Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
                            {t('overview.threads')}
                            {waiting > 0 && <Box component="span" aria-hidden sx={{ px: 0.75, minWidth: 18, borderRadius: radius.pill, bgcolor: 'warning.main', color: 'warning.contrastText', fontSize: fontSizes.micro, fontWeight: 600, lineHeight: '18px', textAlign: 'center' }}>{waiting}</Box>}
                        </Box>} />
                    <Tab value="library" label={t('overview.library')} />
                </Tabs>
            </Box>
            <Box sx={{ flex: 1, minHeight: 0, ...(tab === 'library' && { overflowY: 'auto', overflowX: 'hidden' }) }}>
                {tab === 'threads' ? (
                    <ThreadsView scope={projectId} threads={threads} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks}
                        onGroup={onStatus && ((key, _label, anchor) => onStatus(key, anchor))}
                        countLabel={(count) => t('overview.threadCount', { count })} empty={t('overview.noThreads')}
                        // What the project's threads touched on GitHub: the whole project's, so under the list rather than in any one thread.
                        footer={<Box sx={{ px: 1.5, pb: 1.5 }}><ProjectLinksCard threads={threads} /></Box>} />
                ) : <Library key={projectId} projectId={projectId} refreshKey={libraryKey} />}
            </Box>
        </Box>
    );
}
