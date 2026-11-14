import type { ReactNode } from 'react';
import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Project, Thread } from '../../client';
import { ProjectLinksCard } from './LinksCard';
import { Fact, RailCard, StatusCard, UsageCard, shortSession } from './infoRail';

interface Props {
    project: Project;
    /** The project's main thread, with the status the server gives it (working, idle, or waiting on a recovery decision). */
    coordinator?: Thread;
    /** Inputs waiting behind the running turn. */
    queued: number;
    threads: Thread[];
    /** Opens the running-tasks list of the threads that have tasks, from the fact that counts them. */
    onTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    /** Opens a state's thread list from the fact that counts it (the waiting count holds failed too). */
    onStatus?: (label: string, threads: Thread[], anchor: HTMLElement) => void;
    /** Cards the host adds (SessionSlots.railCards), after Usage. */
    extra?: ReactNode;
}

/**
 * The coordinator's status, as the Overview's third tab: the same Status and
 * Usage cards a thread's rail has, plus what only the coordinator has (its
 * queue, its threads by state, its model).
 */
export function CoordinatorStatus({ project, coordinator, queued, threads, onTasks, onStatus, extra }: Props) {
    const { t } = useTranslation('projects');
    const count = (...statuses: string[]) => threads.filter((x) => statuses.includes(x.status)).length;
    const running = threads.reduce((n, x) => n + (x.running_tasks ?? 0), 0);
    const busy = threads.filter((x) => x.running_tasks > 0);
    return (
        <Box sx={{ p: 1.5, display: 'flex', flexDirection: 'column', gap: 1.25, bgcolor: 'background.default', minHeight: '100%' }}>
            {coordinator && (
                <StatusCard thread={coordinator}>
                    {queued > 0 && <Fact label={t('rail.queued')} title={t('rail.queuedTitle')}>{queued}</Fact>}
                </StatusCard>
            )}
            <RailCard id="threads" title={t('rail.threads')} aside={threads.length > 0 ? String(threads.length) : undefined}>
                {threads.length === 0
                    ? <Box sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('rail.noThreads')}</Box>
                    : (
                        <>
                            <Fact label={t('thread.status.working')} onClick={onStatus && ((e) => { e.stopPropagation(); onStatus(t('thread.status.working'), threads.filter((x) => x.status === 'working'), e.currentTarget); })}>{count('working')}</Fact>
                            <Fact label={t('thread.status.waiting')} onClick={onStatus && ((e) => { e.stopPropagation(); onStatus(t('thread.status.waiting'), threads.filter((x) => x.status === 'waiting' || x.status === 'failed'), e.currentTarget); })}>{count('waiting', 'failed')}</Fact>
                            <Fact label={t('thread.status.ready')} onClick={onStatus && ((e) => { e.stopPropagation(); onStatus(t('thread.status.ready'), threads.filter((x) => x.status === 'ready'), e.currentTarget); })}>{count('ready')}</Fact>
                            <Fact label={t('thread.status.resolved')} onClick={onStatus && ((e) => { e.stopPropagation(); onStatus(t('thread.status.resolved'), threads.filter((x) => x.status === 'resolved'), e.currentTarget); })}>{count('resolved')}</Fact>
                            {running > 0 && (
                                <Fact label={t('rail.backgroundTasks')} title={t('rail.tasksInThreads', { tasks: running, count: busy.length })}
                                    onClick={onTasks && ((e) => { e.stopPropagation(); onTasks(busy, e.currentTarget); })}>
                                    {t('rail.tasksInThreads', { tasks: running, count: busy.length })}
                                </Fact>
                            )}
                        </>
                    )}
            </RailCard>
            <ProjectLinksCard threads={threads} />
            <RailCard id="coordinator" title={t('rail.model')}>
                {project.coordinator_model && <Fact label={t('threadSettings.configuredModel')}>{project.coordinator_model}</Fact>}
                {coordinator?.reported_model && <Fact label={t('threadSettings.lastModel')}>{coordinator.reported_model}</Fact>}
                <Fact label={t('rail.effort')}>{project.coordinator_effort ? t(`effort.${project.coordinator_effort}`) : t('effort.default')}</Fact>
                {coordinator?.session_id && <Fact label={t('rail.session')} title={coordinator.session_id}>{shortSession(coordinator.session_id)}</Fact>}
            </RailCard>
            {coordinator && <UsageCard thread={coordinator} />}
            {extra}
        </Box>
    );
}
