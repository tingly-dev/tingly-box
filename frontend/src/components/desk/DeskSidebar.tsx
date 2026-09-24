import {Add, Search} from '@/components/icons';
import type {SessionInfo} from '@/services/deskApi';
import {Box, CircularProgress, IconButton, InputBase, List, ListItemButton, Tooltip, Typography} from '@mui/material';
import {useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {folderName, groupSessionsByFolder, isBusyStatus, sessionTitle} from './deskUtils';

interface DeskSidebarProps {
    sessions: SessionInfo[];
    selectedId: string | null;
    onSelect: (id: string) => void;
    // folder is set when starting from a folder group's "+".
    onNew: (folder?: string) => void;
}

// StatusMark says only what needs attention: a spinner while a turn runs, a
// red dot when the last turn failed. A finished session needs no mark.
const StatusMark = ({status}: {status: string}) => {
    if (isBusyStatus(status)) return <CircularProgress size={10} thickness={6}/>;
    if (status === 'failed') return <Box sx={{width: 7, height: 7, borderRadius: '50%', bgcolor: 'error.main'}}/>;
    return null;
};

// Rows use primary text (the theme's body text defaults to secondary grey,
// which read as disabled here); selection keeps the theme's own highlight.
const rowSx = {
    borderRadius: 1.5,
    gap: 1,
    color: 'text.primary',
    '&.Mui-selected': {fontWeight: 500},
} as const;

const DeskSidebar = ({sessions, selectedId, onSelect, onNew}: DeskSidebarProps) => {
    const {t} = useTranslation();
    const [query, setQuery] = useState('');

    const groups = useMemo(() => {
        const q = query.trim().toLowerCase();
        const visible = q
            ? sessions.filter((s) => sessionTitle(s).toLowerCase().includes(q) || s.project.toLowerCase().includes(q))
            : sessions;
        return groupSessionsByFolder(visible);
    }, [sessions, query]);

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0}}>
            <Box sx={{px: 1, pt: 1, pb: 0.5}}>
                <ListItemButton onClick={() => onNew()} selected={selectedId === null} sx={{...rowSx, py: 0.75}}>
                    <Add sx={{fontSize: 18}}/>
                    <Typography variant="body2" sx={{color: 'inherit'}}>{t('desk.newSession', {defaultValue: 'New session'})}</Typography>
                </ListItemButton>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.5, mt: 0.5, borderRadius: 1.5, bgcolor: 'action.hover'}}>
                    <Search sx={{fontSize: 16, color: 'text.secondary'}}/>
                    <InputBase
                        fullWidth
                        placeholder={t('desk.search', {defaultValue: 'Search'})}
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        sx={{fontSize: '0.875rem'}}
                    />
                </Box>
            </Box>

            <Box sx={{flex: 1, minHeight: 0, overflowY: 'auto', px: 1, pb: 1}}>
                {groups.length === 0 && (
                    <Typography variant="body2" color="text.secondary" sx={{px: 1.5, py: 2}}>
                        {query
                            ? t('desk.noMatches', {defaultValue: 'No matching sessions'})
                            : t('desk.noSessions', {defaultValue: 'No sessions yet'})}
                    </Typography>
                )}
                {groups.map((g) => (
                    <Box key={g.path} sx={{mt: 1.5}}>
                        <Box sx={{display: 'flex', alignItems: 'center', px: 1.5, mb: 0.25, '&:hover .desk-folder-add': {opacity: 1}}}>
                            <Tooltip title={g.path} placement="right">
                                <Typography variant="body2" noWrap sx={{flex: 1, fontWeight: 600, fontSize: '0.75rem', color: 'text.secondary'}}>
                                    {folderName(g.path)}
                                </Typography>
                            </Tooltip>
                            <Tooltip title={t('desk.newInFolder', {defaultValue: 'New session in this folder'})}>
                                <IconButton
                                    className="desk-folder-add"
                                    size="small"
                                    onClick={() => onNew(g.path)}
                                    sx={{p: 0.25, opacity: 0.6}}
                                >
                                    <Add sx={{fontSize: 16}}/>
                                </IconButton>
                            </Tooltip>
                        </Box>
                        <List dense disablePadding>
                            {g.sessions.map((s) => (
                                <ListItemButton
                                    key={s.id}
                                    selected={s.id === selectedId}
                                    onClick={() => onSelect(s.id)}
                                    sx={{...rowSx, py: 0.5, opacity: s.status === 'closed' ? 0.55 : 1}}
                                >
                                    <Typography variant="body2" noWrap sx={{flex: 1, color: 'inherit', fontSize: '0.8125rem'}}>{sessionTitle(s)}</Typography>
                                    <StatusMark status={s.status}/>
                                </ListItemButton>
                            ))}
                        </List>
                    </Box>
                ))}
            </Box>
        </Box>
    );
};

export default DeskSidebar;
