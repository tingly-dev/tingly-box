import {Add, ExpandMore, FolderOpen} from '@/components/icons';
import type {RecentFolder} from '@/services/deskApi';
import {Box, ButtonBase, Divider, InputBase, Menu, MenuItem, Typography} from '@mui/material';
import {composerPickerLabelSx, composerPickerSx} from '@tingly/projects';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {fontMono, fontSizes} from '@/theme/fonts';

interface FolderPickerProps {
    value: string;
    onChange: (path: string) => void;
    recentFolders: RecentFolder[];
    /** Shows "Add working folder" at the end of the list. */
    onAddFolder?: () => void;
}

const baseName = (path: string) => path.replace(/\/+$/, '').split('/').pop() || path;

// FolderPicker is the first picker in the message box's strip: the folder a
// session runs in, named by its folder name (the full path on hover and in
// the list). Pick a known folder or type a path; there is deliberately no
// server-side directory browser, which would mean an API that lists
// arbitrary filesystem paths on request (see .design/desk.md §6).
const FolderPicker = ({value, onChange, recentFolders, onAddFolder}: FolderPickerProps) => {
    const {t} = useTranslation();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [typed, setTyped] = useState('');
    const close = () => setAnchor(null);
    const pick = (path: string) => {
        onChange(path);
        close();
    };
    const known = recentFolders.find((f) => f.path === value);
    const label = value ? known?.name ?? baseName(value) : t('desk.chooseFolder', {defaultValue: 'Choose a working folder'});

    return (
        <>
            <ButtonBase
                onClick={(e) => {
                    setTyped(value);
                    setAnchor(e.currentTarget);
                }}
                title={value || undefined}
                aria-label={t('desk.projectDirectory', {defaultValue: 'Working folder'})}
                aria-haspopup="menu"
                sx={{
                    ...composerPickerSx,
                    display: 'inline-flex', alignItems: 'center', gap: 0.5, py: {xs: 0.75, md: 0.5},
                    color: value ? 'text.primary' : 'warning.main',
                    fontWeight: 500,
                }}
            >
                <FolderOpen/>
                <Typography noWrap sx={{...composerPickerLabelSx, fontWeight: 'inherit', maxWidth: 220}}>{label}</Typography>
                <ExpandMore sx={{fontSize: '16px !important'}}/>
            </ButtonBase>
            <Menu
                anchorEl={anchor}
                open={!!anchor}
                onClose={close}
                anchorOrigin={{vertical: 'top', horizontal: 'left'}}
                transformOrigin={{vertical: 'bottom', horizontal: 'left'}}
                slotProps={{paper: {sx: {minWidth: 300, maxWidth: 440}}}}
            >
                {recentFolders.map((f) => (
                    <MenuItem key={f.path} selected={f.path === value} onClick={() => pick(f.path)}>
                        <Box sx={{minWidth: 0}}>
                            <Typography variant="body2" noWrap sx={{color: 'text.primary'}}>{f.name}</Typography>
                            <Typography noWrap sx={{fontFamily: fontMono, fontSize: fontSizes.xs, color: 'text.secondary'}}>{f.path}</Typography>
                        </Box>
                    </MenuItem>
                ))}
                {recentFolders.length > 0 && <Divider/>}
                {/* Any folder by path: Enter uses it. */}
                <Box sx={{px: 2, py: 0.75}} onKeyDown={(e) => e.stopPropagation()}>
                    <InputBase
                        fullWidth
                        value={typed}
                        onChange={(e) => setTyped(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter' && typed.trim() !== '' && !e.nativeEvent.isComposing) pick(typed.trim());
                        }}
                        placeholder={t('desk.folderPlaceholder', {defaultValue: '/path/to/folder'})}
                        inputProps={{'aria-label': t('desk.folderPlaceholder', {defaultValue: '/path/to/folder'}), spellCheck: false}}
                        sx={{fontFamily: fontMono, fontSize: fontSizes.md, px: 1, py: 0.25, border: 1, borderColor: 'divider', borderRadius: 1}}
                    />
                </Box>
                {onAddFolder && (
                    <MenuItem onClick={() => {
                        close();
                        onAddFolder();
                    }}>
                        <Add fontSize="small" sx={{mr: 1, color: 'text.secondary'}}/>
                        <Typography variant="body2" sx={{color: 'text.primary'}}>{t('desk.addProject', {defaultValue: 'Add working folder'})}</Typography>
                    </MenuItem>
                )}
            </Menu>
        </>
    );
};

export default FolderPicker;
