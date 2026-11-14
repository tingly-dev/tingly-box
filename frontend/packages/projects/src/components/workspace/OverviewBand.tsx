import { useLayoutEffect, useRef, useState } from 'react';
import { Box, Button, IconButton, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { DisclosureChevron } from '../common/Disclosure';
import { STATUS_ORDER } from '../project/threadStatus';
import { StatusHeading } from './StatusGroups';
import { StatusColumns } from './StatusColumns';
import { BAND_DEFAULT, BandResizer, clampBand } from '../project/PaneResizer';
import { readLocal, removeLocal, writeLocal } from '../../utils/storage';
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
    /** Opens a thread's running tasks from its card's count pill. */
    onTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    /** Opens a status's thread list from its heading or closed count. */
    onStatusList?: (status: string, anchor: HTMLElement) => void;
    onLibrary: () => void;
}

/**
 * The Overview as a band under the header: one column per status, what runs
 * first. Closed, it is one line of counts. Finished work stays out
 * of the way: the Overview window lists it.
 */
export function OverviewBand({ threads, selectedId, open, onToggle, onOpen, onTasks, onStatusList, onLibrary }: Props) {
    const { t } = useTranslation('projects');
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

    return (
        <Box component="section" aria-label={t('project.overview')} sx={{ position: 'relative', flexShrink: 0, borderBottom: 1, borderColor: 'divider', bgcolor: 'background.paper' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, px: 2, minHeight: 36 }}>
                {/* Closed: the counts stand in for the columns. */}
                <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 2, overflow: 'hidden' }}>
                    {groups.length === 0 && <Typography sx={{ fontSize: fontSizes.base, color: 'text.secondary' }}>{t('overview.noThreads')}</Typography>}
                    {!open && groups.map((g) => <StatusHeading key={g.key} label={t(`overview.group.${g.key}`)} status={g.key} count={g.list.length}
                        onClick={onStatusList && ((e) => { e.stopPropagation(); onStatusList(g.key, e.currentTarget); })} />)}
                </Box>
                <Button size="small" onClick={onLibrary}>{t('overview.library')}</Button>
                <Tooltip title={open ? t('band.collapse') : t('band.expand')}>
                    <IconButton size="small" onClick={onToggle} aria-expanded={open} aria-label={open ? t('band.collapse') : t('band.expand')}>
                        <DisclosureChevron open={open} fontSize={20} />
                    </IconButton>
                </Tooltip>
            </Box>
            {open && shown.length > 0 && (
                <StatusColumns threads={threads} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} onStatus={onStatusList}
                    rowRef={columns} rowSx={height != null ? { height } : undefined} listMaxHeight={height == null ? 152 : undefined} />
            )}
            {open && shown.length > 0 && <BandResizer height={height ?? fit} onHeight={setStored} onCommit={commit} />}
        </Box>
    );
}
