import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Badge, Box, Drawer, IconButton, Tooltip, useMediaQuery, useTheme } from '@mui/material';
import { OverviewPane as PaneIcon } from '../icons';
import { PANE_DEFAULT, PaneResizer, clampPane } from '../project/PaneResizer';
import { readLocal, writeLocal } from '../../utils/storage';

/**
 * The side pane of a workspace page: open or closed (a drawer on narrow
 * screens) and its width, remembered per browser under `storageKey`.
 */
export interface WorkspacePane {
    narrow: boolean;
    open: boolean;
    drawerOpen: boolean;
    width: number;
    setWidth: (w: number) => void;
    setDrawerOpen: (v: boolean) => void;
    toggle: () => void;
    /** Show the pane, e.g. because something was opened in it. */
    reveal: () => void;
    storageKey: string;
}

export function useWorkspacePane(storageKey = ''): WorkspacePane {
    const theme = useTheme();
    const narrow = useMediaQuery(theme.breakpoints.down('lg'));
    const [open, setOpen] = useState(() => readLocal(`${storageKey}overviewOpen`) !== '0');
    const [width, setWidth] = useState(() => clampPane(Number(readLocal(`${storageKey}paneWidth`)) || PANE_DEFAULT));
    // On narrow screens the pane is a drawer, closed until asked for.
    const [drawerOpen, setDrawerOpen] = useState(false);
    const toggle = () => {
        if (narrow) {
            setDrawerOpen((v) => !v);
            return;
        }
        setOpen((v) => {
            writeLocal(`${storageKey}overviewOpen`, v ? '0' : '1');
            return !v;
        });
    };
    const reveal = () => {
        if (narrow) setDrawerOpen(true);
        else if (!open) setOpen(true);
    };
    return { narrow, open, drawerOpen, width, setWidth, setDrawerOpen, toggle, reveal, storageKey };
}

interface WorkspaceLayoutProps {
    pane: WorkspacePane;
    /** The title block, left of the actions. */
    title: ReactNode;
    /** Header buttons, before the pane toggle. */
    actions?: ReactNode;
    /** The pane toggle's label. */
    paneLabel: string;
    /** Something in the pane waits on the user: a dot on the toggle. */
    paneAttention?: boolean;
    /** Above the main area (a banner). */
    notice?: ReactNode;
    main: ReactNode;
    side: ReactNode;
    /** Left padding of the header on small screens, clear of a host's or the standalone app's menu button. */
    headerInsetXs?: number;
}

/**
 * The page shell Projects' work surfaces share: a header, the main area
 * (a project's conversation, a working directory's session) and a
 * resizable side pane (what is going on in it). Hosts build their own
 * workspace pages from it.
 */
export function WorkspaceLayout({ pane, title, actions, paneLabel, paneAttention, notice, main, side, headerInsetXs = 2 }: WorkspaceLayoutProps) {
    const row = useRef<HTMLDivElement>(null);
    const { storageKey, setWidth } = pane;

    // Keep the main area usable when the window shrinks; the saved width
    // comes back once there is room again.
    useEffect(() => {
        const fit = () => setWidth(clampPane(Number(readLocal(`${storageKey}paneWidth`)) || PANE_DEFAULT, row.current?.clientWidth));
        fit();
        window.addEventListener('resize', fit);
        return () => window.removeEventListener('resize', fit);
    }, [storageKey, setWidth]);

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: { xs: headerInsetXs, md: 3 }, py: 1, minHeight: 52, borderBottom: 1, borderColor: 'divider' }}>
                <Box sx={{ minWidth: 0, flex: 1 }}>{title}</Box>
                {actions}
                <Tooltip title={paneLabel}>
                    <IconButton size="small" onClick={pane.toggle} aria-label={paneLabel} aria-expanded={pane.narrow ? pane.drawerOpen : pane.open}>
                        <Badge color="warning" variant="dot" invisible={!paneAttention}><PaneIcon fontSize="small" /></Badge>
                    </IconButton>
                </Tooltip>
            </Box>
            {notice}
            <Box ref={row} sx={{ flex: 1, display: 'flex', minHeight: 0 }}>
                <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', minHeight: 0 }}>{main}</Box>
                {pane.narrow ? (
                    <Drawer anchor="right" open={pane.drawerOpen} onClose={() => pane.setDrawerOpen(false)}
                        slotProps={{ paper: { sx: { width: { xs: '100%', sm: PANE_DEFAULT } } } }}>
                        {side}
                    </Drawer>
                ) : pane.open && (
                    <Box sx={{ position: 'relative', width: pane.width, flexShrink: 0, borderLeft: 1, borderColor: 'divider', bgcolor: 'background.paper', minHeight: 0 }}>
                        <PaneResizer width={pane.width} onWidth={pane.setWidth} onCommit={(w) => writeLocal(`${storageKey}paneWidth`, String(w))} />
                        {side}
                    </Box>
                )}
            </Box>
        </Box>
    );
}
