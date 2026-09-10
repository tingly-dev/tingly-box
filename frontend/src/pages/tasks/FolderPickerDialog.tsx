// Pick a folder on the host to work in directly. Two ways in: the folders
// you have worked in before (this control plane's local tasks and Claude
// Code's own project history) and a plain directory browser. Typing an
// absolute path is always allowed — the browser is a convenience, not a
// gate.
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Box, Breadcrumbs, Button, Chip, CircularProgress, Dialog, DialogActions, DialogContent, IconButton,
    Link, List, ListItemButton, ListItemIcon, ListItemText, Stack, TextField, Typography,
} from '@mui/material';
import DialogHeader from '@/components/DialogHeader';
import {ArrowBack, FolderOpen as IconFolder, GitHub as IconRepo} from '@/components/icons';
import {agentApi, type DirListing, type RecentFolder} from '@/services/agentApi';

interface Props {
    open: boolean;
    onClose: () => void;
    onPick: (path: string) => void;
}

const FolderPickerDialog = ({open, onClose, onPick}: Props) => {
    const {t} = useTranslation();
    const [recent, setRecent] = useState<RecentFolder[]>([]);
    const [listing, setListing] = useState<DirListing>();
    const [path, setPath] = useState('');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string>();

    const browse = useCallback(async (target: string) => {
        setLoading(true);
        setError(undefined);
        const res = await agentApi.browseDirs(target);
        setLoading(false);
        if (!res.ok) {
            setError(res.error);
            return;
        }
        setListing(res.data);
        setPath(res.data.path);
    }, []);

    useEffect(() => {
        if (!open) return;
        agentApi.recentFolders().then((res) => {
            if (res.ok) setRecent(res.data.folders ?? []);
        });
        browse('');
    }, [open, browse]);

    const crumbs = listing ? listing.path.split(/[\\/]/).filter(Boolean) : [];
    const crumbPath = (i: number) => (listing?.path.startsWith('/') ? '/' : '') + crumbs.slice(0, i + 1).join('/');

    return (
        <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm" aria-labelledby="folder-picker-title">
            <DialogHeader titleId="folder-picker-title" title={t('tasks.folder.title')} closeLabel={t('common.close')} onClose={onClose} />
            <DialogContent>
                <Stack spacing={2}>
                    {recent.length > 0 && (
                        <Box>
                            <Typography variant="overline" color="text.secondary">{t('tasks.folder.recent')}</Typography>
                            <Stack direction="row" spacing={1} sx={{flexWrap: 'wrap', rowGap: 1}}>
                                {recent.map((f) => (
                                    <Chip
                                        key={f.path}
                                        icon={f.is_repo ? <IconRepo /> : <IconFolder />}
                                        label={f.name}
                                        title={f.path}
                                        onClick={() => onPick(f.path)}
                                        variant="outlined"
                                    />
                                ))}
                            </Stack>
                        </Box>
                    )}

                    <TextField
                        size="small"
                        fullWidth
                        label={t('tasks.folder.path')}
                        value={path}
                        onChange={(e) => setPath(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') browse(path);
                        }}
                        error={!!error}
                        helperText={error ?? t('tasks.folder.pathHelp')}
                        slotProps={{input: {sx: {fontFamily: 'monospace'}}}}
                    />

                    {listing && (
                        <Box>
                            <Stack direction="row" spacing={1} sx={{alignItems: 'center', minWidth: 0}}>
                                <IconButton size="small" disabled={!listing.parent} onClick={() => listing.parent && browse(listing.parent)} aria-label={t('tasks.folder.up')}>
                                    <ArrowBack fontSize="small" />
                                </IconButton>
                                <Breadcrumbs maxItems={4} sx={{minWidth: 0, overflow: 'hidden', '& ol': {flexWrap: 'nowrap'}}}>
                                    {crumbs.map((c, i) => (
                                        <Link key={i} component="button" underline="hover" color="inherit" onClick={() => browse(crumbPath(i))} sx={{fontFamily: 'monospace'}}>
                                            {c}
                                        </Link>
                                    ))}
                                </Breadcrumbs>
                                {loading && <CircularProgress size={14} />}
                            </Stack>
                            <List dense sx={{maxHeight: 280, overflowY: 'auto', border: '1px solid', borderColor: 'divider', borderRadius: 1, mt: 1}}>
                                {listing.entries.length === 0 && (
                                    <Typography variant="body2" color="text.secondary" sx={{p: 1.5}}>{t('tasks.folder.empty')}</Typography>
                                )}
                                {listing.entries.map((e) => (
                                    <ListItemButton key={e.path} onClick={() => browse(e.path)} onDoubleClick={() => onPick(e.path)}>
                                        <ListItemIcon sx={{minWidth: 32}}>{e.is_repo ? <IconRepo fontSize="small" /> : <IconFolder fontSize="small" />}</ListItemIcon>
                                        <ListItemText primary={e.name} secondary={e.is_repo ? t('tasks.folder.gitRepo') : undefined} />
                                    </ListItemButton>
                                ))}
                            </List>
                        </Box>
                    )}
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose}>{t('common.cancel')}</Button>
                <Button variant="contained" disabled={!path.trim().startsWith('/')} onClick={() => onPick(path.trim())}>
                    {t('tasks.folder.use')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default FolderPickerDialog;
