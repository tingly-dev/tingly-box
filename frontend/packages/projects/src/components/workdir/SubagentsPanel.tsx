import { useEffect, useState } from 'react';
import { Box, Button, CircularProgress, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { getReadableAccent } from '@tingly/ui/theme/status';
import type { Subagent } from '../../client';
import { Block, Cancel, CheckCircle, Robot } from '../icons';
import { useNotify } from '../../hooks/useNotify';
import { timeAgo } from '../../utils/format';

interface Props {
    /** Reads the session's subagents (every one, not only those on a loaded page). */
    load: () => Promise<Subagent[]>;
    /** The session's turn runs: a foreground subagent still running without it was cut off. */
    turnLive: boolean;
    /** Opens a subagent on its own. */
    onOpen: (agent: Subagent) => void;
    /** Shows a subagent's card in the conversation, loading the conversation back to it. */
    onShow: (agent: Subagent) => void;
}

/** A subagent's state as its card shows it. */
export const subagentStatus = (agent: Subagent, turnLive: boolean): string =>
    agent.status === 'running' && !turnLive && !agent.background ? 'stopped' : agent.status;

/**
 * Every subagent a session ran, oldest first, so one can be found without
 * scrolling a long transcript: opened on its own, or shown where it sits in
 * the conversation.
 */
export function SubagentsPanel({ load, turnLive, onOpen, onShow }: Props) {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const [list, setList] = useState<Subagent[] | null>(null);
    useEffect(() => {
        load().then(setList).catch((e: Error) => {
            notify(e.message, 'error');
            setList([]);
        });
    }, [load, notify]);

    // One width whatever the state: the popover is placed once, from its first size.
    const frame = { width: 480, maxWidth: '92vw' };
    if (list === null) {
        return <Stack direction="row" spacing={1} sx={{ ...frame, p: 2, alignItems: 'center', color: 'text.secondary' }}><CircularProgress size={14} /><Typography variant="body2" sx={{ color: 'inherit' }}>{t('subagents.loading')}</Typography></Stack>;
    }
    if (list.length === 0) {
        return (
            <Box sx={{ ...frame, p: 2 }}>
                <Typography variant="body2" sx={{ color: 'text.primary', fontWeight: 600, mb: 0.5 }}>{t('subagents.none')}</Typography>
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('subagents.hint')}</Typography>
            </Box>
        );
    }
    return (
        <Box sx={{ ...frame, maxHeight: '75vh', overflowY: 'auto', py: 0.5 }}>
            {list.map((agent) => {
                const status = subagentStatus(agent, turnLive);
                const label = { running: t('tasks.statusRunning'), completed: t('tasks.statusDone'), stopped: t('tasks.statusStopped'), failed: t('tasks.statusFailed') }[status] ?? status;
                const icon = status === 'running'
                    ? <CircularProgress size={14} />
                    : status === 'completed'
                        ? <CheckCircle sx={{ fontSize: 16, color: (theme) => getReadableAccent(theme, 'success') }} />
                        : status === 'failed'
                            ? <Cancel sx={{ fontSize: 16, color: (theme) => getReadableAccent(theme, 'error') }} />
                            : <Block sx={{ fontSize: 16, color: 'text.secondary' }} />;
                return (
                    <Box key={agent.call} role="button" data-subagent={agent.call} onClick={() => onOpen(agent)}
                        sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.75, cursor: 'default', '&:hover': { bgcolor: 'action.hover' }, '&:hover .show': { visibility: 'visible' } }}>
                        <Robot sx={{ fontSize: 18, color: 'text.secondary', flexShrink: 0 }} />
                        <Box sx={{ flex: 1, minWidth: 0 }}>
                            <Stack direction="row" spacing={0.75} sx={{ alignItems: 'baseline', minWidth: 0 }}>
                                <Typography variant="body2" noWrap sx={{ fontWeight: 600, color: 'text.primary', minWidth: 0 }}>{agent.description || t('subagents.title')}</Typography>
                                {agent.subagent_type && <Typography variant="caption" sx={{ color: 'text.secondary', flexShrink: 0 }}>{agent.subagent_type}</Typography>}
                            </Stack>
                            <Typography variant="caption" component="div" noWrap sx={{ color: 'text.secondary' }}>
                                {[label, t('subagents.steps', { count: agent.steps }), timeAgo(agent.created_at, i18n.language)].join(' · ')}
                            </Typography>
                        </Box>
                        <Button className="show" size="small" onClick={(e) => { e.stopPropagation(); onShow(agent); }}
                            sx={{ visibility: 'hidden', flexShrink: 0, whiteSpace: 'nowrap' }}>
                            {t('subagents.show')}
                        </Button>
                        <Box sx={{ width: 16, display: 'flex', justifyContent: 'center', flexShrink: 0 }} title={label}>{icon}</Box>
                    </Box>
                );
            })}
        </Box>
    );
}
