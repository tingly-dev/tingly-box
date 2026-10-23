import { Box, Button, IconButton, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { ExpandLess, ExpandMore } from '../icons';
import { STATUS_ORDER, ThreadBadges, ThreadSubtitle } from '../project/threadStatus';
import { ListRow } from '../ListRow';
import { StatusHeading } from './StatusGroups';
import { timeAgo } from '../../utils/format';
import { fontSizes } from '@tingly/ui/theme/fonts';

interface Props {
    threads: Thread[];
    selectedId?: string;
    open: boolean;
    onToggle: () => void;
    onOpen: (id: string) => void;
    onLibrary: () => void;
}

/**
 * The Overview as a band under the header: one column per status, what needs
 * the user first. Closed, it is one line of counts. Finished work stays out
 * of the way: the Overview window lists it.
 */
export function OverviewBand({ threads, selectedId, open, onToggle, onOpen, onLibrary }: Props) {
    const { t, i18n } = useTranslation('projects');
    const groups = STATUS_ORDER.map((s) => ({ key: s, list: threads.filter((th) => th.status === s) })).filter((g) => g.list.length > 0);
    const shown = groups.filter((g) => g.key !== 'resolved');
    const resolved = groups.find((g) => g.key === 'resolved')?.list.length ?? 0;

    return (
        <Box component="section" aria-label={t('project.overview')} sx={{ flexShrink: 0, borderBottom: 1, borderColor: 'divider', bgcolor: 'background.paper' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, px: 2, minHeight: 36 }}>
                {/* Closed: the counts stand in for the columns. */}
                <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 2, overflow: 'hidden' }}>
                    {groups.length === 0 && <Typography sx={{ fontSize: fontSizes.base, color: 'text.secondary' }}>{t('overview.noThreads')}</Typography>}
                    {!open && groups.map((g) => <StatusHeading key={g.key} label={t(`overview.group.${g.key}`)} status={g.key} count={g.list.length} />)}
                </Box>
                <Button size="small" onClick={onLibrary}>{t('overview.library')}</Button>
                <Tooltip title={open ? t('band.collapse') : t('band.expand')}>
                    <IconButton size="small" onClick={onToggle} aria-expanded={open} aria-label={open ? t('band.collapse') : t('band.expand')}>
                        {open ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
                    </IconButton>
                </Tooltip>
            </Box>
            {open && shown.length > 0 && (
                <Box sx={{ display: 'flex', gap: 1.5, px: 2, pb: 1.5, overflowX: 'auto' }}>
                    {shown.map((g) => (
                        <Box key={g.key} sx={{ flex: '1 1 0', minWidth: 220, maxWidth: 360 }}>
                            <Box sx={{ mb: 0.75 }}><StatusHeading label={t(`overview.group.${g.key}`)} status={g.key} count={g.list.length} /></Box>
                            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, maxHeight: 152, overflowY: 'auto' }}>
                                {g.list.map((th) => (
                                    <ListRow key={th.id} outlined selected={th.id === selectedId} onClick={() => onOpen(th.id)}
                                        title={th.title} subtitle={<ThreadSubtitle thread={th} />} badges={<ThreadBadges thread={th} />} meta={timeAgo(th.updated_at, i18n.language)} />
                                ))}
                            </Box>
                        </Box>
                    ))}
                    {resolved > 0 && (
                        <Box sx={{ flex: '0 0 auto', minWidth: 120 }}>
                            <StatusHeading label={t('overview.group.resolved')} status="resolved" count={resolved} />
                        </Box>
                    )}
                </Box>
            )}
        </Box>
    );
}
