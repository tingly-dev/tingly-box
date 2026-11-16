import { useState, type ReactNode } from 'react';
import { Box, Collapse, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { Project, Thread } from '../../client';
import { DisclosureRow } from '../common/Disclosure';
import { formatTokens } from '../../utils/format';
import { contextFill } from '../../utils/usage';
import { readLocal, writeLocal } from '../../utils/storage';
import { Fact, RailCard, StatusCard, UsageCard, shortSession } from './infoRail';
import { ThreadStateMark } from './threadStatus';

interface Props {
    project: Project;
    /** The project's main thread, with the status the server gives it (working, idle, or waiting on a recovery decision). */
    coordinator?: Thread;
    /** Inputs waiting behind the running turn. */
    queued: number;
    /** Cards the host adds (SessionSlots.railCards), after Usage. */
    extra?: ReactNode;
}

const KEY = 'coordinator.status';

/**
 * The coordinator's status, at the top of the coordinator's conversation: one
 * line while folded (its state, how full its context is, its turns, model and
 * usage), the same Status and Usage cards a thread's rail has plus its model
 * once open. It sits with the conversation it describes, not in the
 * Overview, which lists what the project holds (threads, files).
 */
export function CoordinatorStatus({ project, coordinator, queued, extra }: Props) {
    const { t } = useTranslation('projects');
    const [open, setOpen] = useState(() => readLocal(KEY) === '1');
    const toggle = () => {
        writeLocal(KEY, open ? '0' : '1');
        setOpen(!open);
    };
    if (!coordinator) return null;
    const fill = contextFill(coordinator);
    const total = coordinator.input_tokens + coordinator.output_tokens;
    const model = project.coordinator_model || coordinator.reported_model;
    const state = coordinator.status === 'working' && coordinator.queued ? 'queued' : coordinator.status;
    const fact = (label: string, value: string) => (
        <Typography component="span" noWrap sx={{ fontSize: fontSizes.sm, color: 'text.secondary', minWidth: 0, flexShrink: 1 }}>
            {label} <Box component="span" sx={{ color: 'text.primary' }}>{value}</Box>
        </Typography>
    );
    return (
        <Box component="section" aria-label={t('rail.coordinatorStatus')} sx={{ mx: 2, mt: 1.25, flexShrink: 0, border: 1, borderColor: 'divider', borderRadius: radius.card, bgcolor: 'background.paper', overflow: 'hidden' }}>
            <DisclosureRow open={open} onToggle={toggle} aria-label={t(open ? 'rail.coordinatorStatusHide' : 'rail.coordinatorStatusShow')} sx={{ gap: 1.5 }}>
                <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, flexShrink: 0 }}>
                    <ThreadStateMark status={coordinator.status} queued={coordinator.queued} />
                    <Typography component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600, color: 'text.primary' }}>{t(`thread.status.${state}`)}</Typography>
                </Box>
                <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'baseline', gap: 1.5, overflow: 'hidden' }}>
                    {fill?.pct != null && fact(t('rail.context'), `${fill.pct}%`)}
                    {coordinator.turns > 0 && fact(t('rail.turns'), String(coordinator.turns))}
                    {model && fact(t('rail.model'), model)}
                    {total > 0 && fact(t('rail.usage'), formatTokens(total))}
                </Box>
            </DisclosureRow>
            <Collapse in={open} unmountOnExit>
                <Box sx={{ p: 1.25, display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 1.25, alignItems: 'start', bgcolor: 'background.default', borderTop: 1, borderColor: 'divider' }}>
                    <StatusCard thread={coordinator}>
                        {queued > 0 && <Fact label={t('rail.queued')} title={t('rail.queuedTitle')}>{queued}</Fact>}
                    </StatusCard>
                    <RailCard id="coordinator" title={t('rail.model')}>
                        {project.coordinator_model && <Fact label={t('threadSettings.configuredModel')}>{project.coordinator_model}</Fact>}
                        {coordinator.reported_model && <Fact label={t('threadSettings.lastModel')}>{coordinator.reported_model}</Fact>}
                        <Fact label={t('rail.effort')}>{project.coordinator_effort ? t(`effort.${project.coordinator_effort}`) : t('effort.default')}</Fact>
                        {coordinator.session_id && <Fact label={t('rail.session')} title={coordinator.session_id}>{shortSession(coordinator.session_id)}</Fact>}
                    </RailCard>
                    <UsageCard thread={coordinator} />
                    {extra}
                </Box>
            </Collapse>
        </Box>
    );
}
