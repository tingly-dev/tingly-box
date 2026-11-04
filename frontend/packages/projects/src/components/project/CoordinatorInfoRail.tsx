import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Project, Thread } from '../../client';
import { ThreadStateMark } from './threadStatus';
import { ContextMeter, Fact, InfoRailFrame, RailCard, TurnFacts, UsageCard, shortSession } from './infoRail';

interface Props {
    project: Project;
    /** The project's main thread: the coordinator session's turns, tokens and context. */
    coordinator?: Thread;
    busy: boolean;
    /** An interrupted input waits for the user's retry or skip. */
    needsRecovery: boolean;
    /** Inputs waiting behind the running turn. */
    queued: number;
    threads: Thread[];
    /** The connection's default model, shown when the project sets none. */
    defaultModel?: string;
    floating?: boolean;
}

/**
 * What the coordinator is, beside the project conversation: whether it is
 * working, how full its context is, what it has spent, and what its threads
 * are doing. The same cards as a thread's rail, answering the same questions.
 */
export function CoordinatorInfoRail({ project, coordinator, busy, needsRecovery, queued, threads, defaultModel, floating }: Props) {
    const { t } = useTranslation('projects');
    const state = needsRecovery ? 'recovery' : busy ? 'working' : 'idle';
    const count = (...statuses: string[]) => threads.filter((x) => statuses.includes(x.status)).length;
    const model = project.coordinator_model || defaultModel;
    return (
        <InfoRailFrame label={t('rail.coordinatorTitle')} floating={floating}>
            <RailCard id="status" title={t('rail.status')}>
                <Fact label={t('rail.state')}>
                    <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
                        <ThreadStateMark status={state === 'recovery' ? 'waiting' : state} />
                        {t(`rail.coordinatorState.${state}`)}
                    </Box>
                </Fact>
                {queued > 0 && <Fact label={t('rail.queued')} title={t('rail.queuedTitle')}>{queued}</Fact>}
                {coordinator && (
                    <>
                        <ContextMeter thread={coordinator} />
                        <TurnFacts thread={coordinator} />
                    </>
                )}
            </RailCard>
            <RailCard id="threads" title={t('rail.threads')} aside={threads.length > 0 ? String(threads.length) : undefined}>
                {threads.length === 0
                    ? <Box sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('rail.noThreads')}</Box>
                    : (
                        <>
                            <Fact label={t('thread.status.working')}>{count('working')}</Fact>
                            <Fact label={t('thread.status.waiting')}>{count('waiting', 'failed')}</Fact>
                            <Fact label={t('thread.status.ready')}>{count('ready')}</Fact>
                            <Fact label={t('thread.status.resolved')}>{count('resolved')}</Fact>
                        </>
                    )}
            </RailCard>
            <RailCard id="coordinator" title={t('rail.model')}>
                {model && <Fact label={t('threadSettings.configuredModel')}>{model}</Fact>}
                {coordinator?.reported_model && <Fact label={t('threadSettings.lastModel')}>{coordinator.reported_model}</Fact>}
                <Fact label={t('rail.effort')}>{project.coordinator_effort ? t(`effort.${project.coordinator_effort}`) : t('effort.default')}</Fact>
                {coordinator?.session_id && <Fact label={t('rail.session')} title={coordinator.session_id}>{shortSession(coordinator.session_id)}</Fact>}
            </RailCard>
            {coordinator && <UsageCard thread={coordinator} />}
        </InfoRailFrame>
    );
}
