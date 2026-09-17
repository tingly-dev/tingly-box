import {Folder as FolderIcon, FolderOpen} from '@/components/icons';
import * as managedAgentApi from '@/services/managedAgentApi';
import type {DirEntry, RecentFolder} from '@/services/managedAgentApi';
import {Autocomplete, Box, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Popover, TextField, Typography} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';

interface FolderPickerProps {
    value: string;
    onChange: (path: string) => void;
    recentFolders: RecentFolder[];
}

// FolderPicker is one field, not a wizard (ux-principles #2): type a path
// directly, pick a recently-used one, or click Browse to descend through
// real directories via GET /managed-agent/fs/dirs — never a separate
// "choose mode then choose folder" step.
const FolderPicker = ({value, onChange, recentFolders}: FolderPickerProps) => {
    const {t} = useTranslation();
    const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
    const [browsePath, setBrowsePath] = useState('');
    const [entries, setEntries] = useState<DirEntry[]>([]);
    const [loading, setLoading] = useState(false);

    const openBrowser = (el: HTMLElement) => {
        setAnchorEl(el);
        loadDir(value || undefined);
    };

    const loadDir = async (path?: string) => {
        setLoading(true);
        try {
            const res = await managedAgentApi.listDirs(path);
            setBrowsePath(res.path);
            setEntries(res.entries);
        } catch {
            // A bad path just shows an empty listing; the text field still
            // carries whatever the user typed.
            setEntries([]);
        } finally {
            setLoading(false);
        }
    };

    return (
        <Box sx={{display: 'flex', alignItems: 'flex-start', gap: 1}}>
            <Autocomplete
                freeSolo
                fullWidth
                size="small"
                options={recentFolders.map((f) => f.path)}
                inputValue={value}
                onInputChange={(_e, newValue) => onChange(newValue)}
                renderOption={(props, option) => {
                    const folder = recentFolders.find((f) => f.path === option);
                    return (
                        <Box component="li" {...props} key={option}>
                            <ListItemIcon sx={{minWidth: 32}}><FolderIcon fontSize="small"/></ListItemIcon>
                            <Box>
                                <Typography variant="body2">{folder ? folder.name : option}</Typography>
                                <Typography variant="caption" color="text.secondary">{option}</Typography>
                            </Box>
                        </Box>
                    );
                }}
                renderInput={(params) => (
                    <TextField
                        {...params}
                        label={t('managedAgent.folderPath', {defaultValue: 'Folder'})}
                        placeholder="/path/to/project"
                        slotProps={{
                            ...params.slotProps,
                            htmlInput: {...params.slotProps.htmlInput, style: {fontFamily: 'monospace'}},
                        }}
                    />
                )}
            />
            <IconButton
                sx={{mt: 0.5}}
                onClick={(e) => openBrowser(e.currentTarget)}
                title={t('managedAgent.browse', {defaultValue: 'Browse folders'})}
            >
                <FolderOpen/>
            </IconButton>
            <Popover
                open={Boolean(anchorEl)}
                anchorEl={anchorEl}
                onClose={() => setAnchorEl(null)}
                anchorOrigin={{vertical: 'bottom', horizontal: 'left'}}
            >
                <Box sx={{width: 420, maxHeight: 420, display: 'flex', flexDirection: 'column'}}>
                    <Box sx={{p: 1.5, pb: 1}}>
                        <Typography variant="caption" color="text.secondary" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>
                            {browsePath || t('managedAgent.loading', {defaultValue: 'Loading…'})}
                        </Typography>
                    </Box>
                    <List dense sx={{overflowY: 'auto', flex: 1, pt: 0}}>
                        {browsePath && (
                            <ListItemButton onClick={() => loadDir(browsePath.replace(/\/[^/]+\/?$/, '') || '/')}>
                                <ListItemIcon sx={{minWidth: 32}}>…</ListItemIcon>
                                <ListItemText primary={t('managedAgent.parentFolder', {defaultValue: 'Parent folder'})}/>
                            </ListItemButton>
                        )}
                        {!loading && entries.length === 0 && (
                            <Typography variant="body2" color="text.secondary" sx={{px: 2, py: 1}}>
                                {t('managedAgent.noSubfolders', {defaultValue: 'No subfolders'})}
                            </Typography>
                        )}
                        {entries.filter((e) => !e.hidden).map((entry) => (
                            <ListItemButton key={entry.path} onClick={() => loadDir(entry.path)}>
                                <ListItemIcon sx={{minWidth: 32}}><FolderIcon fontSize="small" color={entry.is_repo ? 'primary' : 'inherit'}/></ListItemIcon>
                                <ListItemText primary={entry.name} secondary={entry.is_repo ? 'git' : undefined}/>
                            </ListItemButton>
                        ))}
                    </List>
                    <Box sx={{p: 1, borderTop: 1, borderColor: 'divider', display: 'flex', justifyContent: 'flex-end'}}>
                        <IconButton
                            color="primary"
                            title={t('managedAgent.selectFolder', {defaultValue: 'Use this folder'})}
                            onClick={() => {
                                onChange(browsePath);
                                setAnchorEl(null);
                            }}
                        >
                            <FolderOpen/>
                        </IconButton>
                    </Box>
                </Box>
            </Popover>
        </Box>
    );
};

export default FolderPicker;
