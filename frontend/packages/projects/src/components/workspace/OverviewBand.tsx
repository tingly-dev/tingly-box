import { useLayoutEffect, useRef, useState } from 'react';
import { Box, Button, IconButton, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { ExpandLess, ExpandMore } from '../icons';
import { STATUS_ORDER, ThreadBadges, ThreadSubtitle } from '../project/threadStatus';
import { ListRow } from '../ListRow';
import { StatusHeading } from './StatusGroups';
import { BAND_DEFAULT, BandResizer, clampBand } from '../project/PaneResizer';
import { readLocal, removeLocal, writeLocal } from '../../utils/storage';
import { timeAgo } from '../../utils/format';
import { fontSizes } from '@tingly/ui/theme/fonts';

const HEIGHT_KEY = 'overviewBandHeight';

/** The columns' height the user dragged to, remembered per browser; null while they fit their content. */
function useBandHeight() {
    const [stored, setStored] = useState<number | null>(() => {
        const n = Number(readLocal(HEIGHT_KEY));
        return n > 0 ? clampBand(n) : null;
    });
    const commit = (h: number) => {
        // Back at the default is "fit the content" again, not a fixed height.
        if (h === BAND_DEFAULT) {
            removeLocal(HEIGHT_KEY);
            setStored(null);
        } else {
            writeLocal(HEIGHT_KEY, String(h));
            setStored(h);
        }
    };
    return { stored, setStored, commit };
}

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
    const { stored, setStored, commit } = useBandHeight();
    const columns = useRef<HTMLDivElement>(null);
    // While the columns fit their content, a drag starts from the height they have now, so the band does not jump.
    const [fit, setFit] = useState(BAND_DEFAULT);
    useLayoutEffect(() => {
        if (stored == null && columns.current) setFit(columns.current.clientHeight || BAND_DEFAULT);
    });
    // A smaller window may no longer have the room for the saved height; it comes back with the room.
    const [room, setRoom] = useState<number | undefined>(undefined);
    useLayoutEffect(() => {
        const measure = () => setRoom(columns.current?.parentElement?.parentElement?.clientHeight);
        measure();
        window.addEventListener('resize', measure);
        return () => window.removeEventListener('resize', measure);
    }, []);
    const height = stored == null ? null : clampBand(stored, room);
    const resolved = groups.find((g) => g.key === 'resolved')?.list.length ?? 0;

    return (
        <Box component="section" aria-label={t('project.overview')} sx={{ position: 'relative', flexShrink: 0, borderBottom: 1, borderColor: 'divider', bgcolor: 'background.paper' }}>
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
                <Box ref={columns} sx={{ display: 'flex', gap: 1.5, px: 2, pb: 1.5, overflowX: 'auto', ...(height != null && { height }) }}>
                    {shown.map((g) => (
                        <Box key={g.key} sx={{ flex: '1 1 0', minWidth: 220, maxWidth: 360, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                            <Box sx={{ mb: 0.75 }}><StatusHeading label={t(`overview.group.${g.key}`)} status={g.key} count={g.list.length} /></Box>
                            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, overflowY: 'auto', minHeight: 0, ...(height == null ? { maxHeight: 152 } : { flex: 1 }) }}>
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
            {open && shown.length > 0 && <BandResizer height={height ?? fit} onHeight={setStored} onCommit={commit} />}
        </Box>
    );
}
