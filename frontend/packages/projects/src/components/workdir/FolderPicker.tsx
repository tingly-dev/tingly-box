import { useState } from 'react';
import { Box, ButtonBase, Divider, InputBase, Menu, MenuItem, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { folderName } from '../../utils/format';
import { composerPickerLabelSx, composerPickerSx } from '../composerPicker';
import { Add, ExpandMore, FolderOpen } from '../icons';

interface Props {
    value: string;
    onChange: (path: string) => void;
    /** The known folders' paths. */
    folders: string[];
    /** Shows "Add working folder" at the end of the list. */
    onAddFolder?: () => void;
}

/**
 * The first picker in a new session's message box: the folder it works in,
 * named by its folder name (the full path on hover and in the list). Pick a
 * known folder or type a path. There is deliberately no server-side directory
 * browser: it would be an API that lists arbitrary paths on request.
 */
export function FolderPicker({ value, onChange, folders, onAddFolder }: Props) {
    const { t } = useTranslation('projects');
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [typed, setTyped] = useState('');
    const close = () => setAnchor(null);
    const pick = (path: string) => {
        onChange(path);
        close();
    };
    const label = value ? folderName(value) : t('session.chooseFolder');

    return (
        <>
            <ButtonBase
                onClick={(e) => {
                    setTyped(value);
                    setAnchor(e.currentTarget);
                }}
                title={value || undefined}
                aria-label={`${t('session.folder')}: ${label}`}
                aria-haspopup="menu"
                sx={{ ...composerPickerSx, display: 'inline-flex', alignItems: 'center', gap: 0.5, py: { xs: 0.75, md: 0.5 }, color: value ? 'text.primary' : 'text.secondary', fontWeight: 500 }}
            >
                <FolderOpen />
                <Typography noWrap sx={{ ...composerPickerLabelSx, fontWeight: 'inherit', maxWidth: 220 }}>{label}</Typography>
                <ExpandMore sx={{ fontSize: '16px !important' }} />
            </ButtonBase>
            <Menu
                anchorEl={anchor}
                open={!!anchor}
                onClose={close}
                anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                slotProps={{ paper: { sx: { minWidth: 300, maxWidth: 440 } } }}
            >
                {folders.map((path) => (
                    <MenuItem key={path} selected={path === value} onClick={() => pick(path)}>
                        <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" noWrap sx={{ color: 'text.primary' }}>{folderName(path)}</Typography>
                            <Typography noWrap sx={{ fontFamily: fontMono, fontSize: fontSizes.xs, color: 'text.secondary' }}>{path}</Typography>
                        </Box>
                    </MenuItem>
                ))}
                {folders.length > 0 && <Divider />}
                {/* Any folder by path: Enter uses it. */}
                <Box sx={{ px: 2, py: 0.75 }} onKeyDown={(e) => e.stopPropagation()}>
                    <InputBase
                        fullWidth
                        value={typed}
                        onChange={(e) => setTyped(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter' && typed.trim() !== '' && !e.nativeEvent.isComposing) pick(typed.trim());
                        }}
                        placeholder={t('session.folderPlaceholder')}
                        inputProps={{ 'aria-label': t('session.folderPlaceholder'), spellCheck: false }}
                        sx={{ fontFamily: fontMono, fontSize: fontSizes.md, px: 1, py: 0.25, border: 1, borderColor: 'divider', borderRadius: 1 }}
                    />
                </Box>
                {onAddFolder && (
                    <MenuItem onClick={() => { close(); onAddFolder(); }}>
                        <Add fontSize="small" sx={{ mr: 1, color: 'text.secondary' }} />
                        <Typography variant="body2" sx={{ color: 'text.primary' }}>{t('workdir.add')}</Typography>
                    </MenuItem>
                )}
            </Menu>
        </>
    );
}
