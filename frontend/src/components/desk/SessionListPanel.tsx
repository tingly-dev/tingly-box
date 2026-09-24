import {Add} from '@/components/icons';
import UnifiedCard from '@/components/UnifiedCard';
import EmptyState from '@/components/EmptyState';
import type {RecentFolder, SessionInfo} from '@/services/deskApi';
import {folderName, isBusyStatus, STATUS_COLOR, timeAgo} from './deskUtils';
import FolderPicker from './FolderPicker';
import PermissionModeSelect from './PermissionModeSelect';
import {
    Box,
    Button,
    Chip,
    List,
    ListItemButton,
    ListItemText,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';

interface SessionListPanelProps {
    sessions: SessionInfo[];
    recentFolders: RecentFolder[];
    permissionModes: string[];
    selectedId: string | null;
    onSelect: (id: string) => void;
    onCreate: (path: string, prompt: string, permissionMode: string) => Promise<void>;
    creating: boolean;
}

// SessionListPanel is the work surface's entry point: the composer to start
// a session is inline here (no "New session" dialog — ux-principles #2), and
// the list below answers "what am I already running / did I already run"
// sorted by recency so the most relevant session is always at the top.
const SessionListPanel = ({sessions, recentFolders, permissionModes, selectedId, onSelect, onCreate, creating}: SessionListPanelProps) => {
    const {t} = useTranslation();
    const [path, setPath] = useState('');
    const [prompt, setPrompt] = useState('');
    const [permissionMode, setPermissionMode] = useState('');

    const canCreate = path.trim() !== '' && prompt.trim() !== '' && !creating;

    const handleCreate = async () => {
        if (!canCreate) return;
        await onCreate(path.trim(), prompt.trim(), permissionMode);
        setPrompt('');
    };

    return (
        <Stack spacing={2}>
            <UnifiedCard
                title={t('desk.newSession', {defaultValue: 'Start a session'})}
                size="full"
            >
                <Stack spacing={1.5}>
                    <FolderPicker value={path} onChange={setPath} recentFolders={recentFolders}/>
                    {recentFolders.length > 0 && (
                        <Box sx={{display: 'flex', flexWrap: 'wrap', gap: 0.5}}>
                            {recentFolders.slice(0, 6).map((f) => (
                                <Chip
                                    key={f.path}
                                    size="small"
                                    label={f.name}
                                    variant={path === f.path ? 'filled' : 'outlined'}
                                    color={path === f.path ? 'primary' : 'default'}
                                    onClick={() => setPath(f.path)}
                                />
                            ))}
                        </Box>
                    )}
                    <TextField
                        multiline
                        minRows={2}
                        maxRows={6}
                        fullWidth
                        size="small"
                        placeholder={t('desk.promptPlaceholder', {defaultValue: 'What should the agent do?'})}
                        value={prompt}
                        onChange={(e) => setPrompt(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                                e.preventDefault();
                                void handleCreate();
                            }
                        }}
                    />
                    <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                        <PermissionModeSelect value={permissionMode} permissionModes={permissionModes} onChange={setPermissionMode}/>
                        <Box sx={{flex: 1}}/>
                        <Button
                            variant="contained"
                            startIcon={<Add/>}
                            disabled={!canCreate}
                            onClick={handleCreate}
                        >
                            {t('desk.start', {defaultValue: 'Start'})}
                        </Button>
                    </Stack>
                </Stack>
            </UnifiedCard>

            <UnifiedCard
                title={t('desk.sessions', {defaultValue: 'Sessions'})}
                size="full"
                titleMarginBottom={1}
            >
                {sessions.length === 0 ? (
                    <EmptyState
                        compact
                        title={t('desk.noSessions', {defaultValue: 'No sessions yet'})}
                        description={t('desk.noSessionsDescription', {defaultValue: 'Start one above — pick a folder and describe the task.'})}
                    />
                ) : (
                    <List dense disablePadding>
                        {sessions.map((s) => (
                            <ListItemButton
                                key={s.id}
                                selected={s.id === selectedId}
                                onClick={() => onSelect(s.id)}
                                sx={{borderRadius: 1, mb: 0.5}}
                            >
                                <ListItemText
                                    primary={(
                                        <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                                            <Typography variant="body2" noWrap sx={{maxWidth: 160, fontWeight: 600}}>
                                                {folderName(s.project)}
                                            </Typography>
                                            <Chip
                                                size="small"
                                                label={s.status}
                                                color={STATUS_COLOR[s.status] || 'default'}
                                                sx={{height: 18, '& .MuiChip-label': {px: 0.75, fontSize: '0.65rem'}}}
                                            />
                                            {isBusyStatus(s.status) && (
                                                <Box sx={{width: 6, height: 6, borderRadius: '50%', bgcolor: 'info.main'}}/>
                                            )}
                                        </Stack>
                                    )}
                                    secondary={(
                                        <Typography variant="caption" color="text.secondary" noWrap sx={{display: 'block', maxWidth: 260}}>
                                            {s.request || s.project} · {timeAgo(s.last_activity)}
                                        </Typography>
                                    )}
                                />
                            </ListItemButton>
                        ))}
                    </List>
                )}
            </UnifiedCard>
        </Stack>
    );
};

export default SessionListPanel;
