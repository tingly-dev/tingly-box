// Pick a folder on the host to work in directly. The browser is an
// allowlist: the top level is the folders you have handed to Tingly Box
// (the local sources) and you can only go down from there. Typing an
// absolute path is how a new folder is handed over — it is used as typed,
// never listed first.
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Box, Breadcrumbs, Button, CircularProgress, Dialog, DialogActions, DialogContent, IconButton,
    Link, List, ListItemButton, ListItemIcon, ListItemText, Stack, TextField, Typography,
} from '@mui/material';
import DialogHeader from '@/components/DialogHeader';
import {ArrowBack, FolderOpen as IconFolder, GitHub as IconRepo} from '@/components/icons';
import {agentApi, type DirListing} from '@/services/agentApi';

interface Props {
    open: boolean;
    onClose: () => void;
    onPick: (path: string) => void;
}

const FolderPickerDialog = ({open, onClose, onPick}: Props) => {
    const {t} = useTranslation();
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
            // Outside the allowlist (403) or not a directory: say so, but the
            // typed path stays usable — using it is what adds it.
            setError(res.error);
            return;
        }
        setListing(res.data);
        setPath(res.data.path);
    }, []);

    useEffect(() => {
        if (!open) return;
        setPath('');
        setError(undefined);
        browse('');
    }, [open, browse]);

    const atTop = !listing || listing.path === '';
    const crumbs = listing && !atTop ? listing.path.split(/[\\/]/).filter(Boolean) : [];
    const crumbPath = (i: number) => (listing?.path.startsWith('/') ? '/' : '') + crumbs.slice(0, i + 1).join('/');
    const up = () => browse(listing?.parent ?? '');

    return (
        <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm" aria-labelledby="folder-picker-title">
            <DialogHeader titleId="folder-picker-title" title={t('tasks.folder.title')} closeLabel={t('common.close')} onClose={onClose} />
            <DialogContent>
                <Stack spacing={2}>
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
                                <IconButton size="small" disabled={atTop} onClick={up} aria-label={t('tasks.folder.up')}>
                                    <ArrowBack fontSize="small" />
                                </IconButton>
                                <Breadcrumbs maxItems={4} sx={{minWidth: 0, overflow: 'hidden', '& ol': {flexWrap: 'nowrap'}}}>
                                    <Link component="button" underline="hover" color={atTop ? 'text.primary' : 'inherit'} onClick={() => browse('')}>
                                        {t('tasks.folder.places')}
                                    </Link>
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
                                    <Typography variant="body2" color="text.secondary" sx={{p: 1.5}}>
                                        {atTop ? t('tasks.folder.noPlaces') : t('tasks.folder.empty')}
                                    </Typography>
                                )}
                                {listing.entries.map((e) => (
                                    <ListItemButton key={e.path} onClick={() => browse(e.path)} onDoubleClick={() => onPick(e.path)}>
                                        <ListItemIcon sx={{minWidth: 32}}>{e.is_repo ? <IconRepo fontSize="small" /> : <IconFolder fontSize="small" />}</ListItemIcon>
                                        <ListItemText
                                            primary={e.name}
                                            secondary={atTop ? e.path : e.is_repo ? t('tasks.folder.gitRepo') : undefined}
                                            slotProps={{secondary: atTop ? {sx: {fontFamily: 'monospace', fontSize: 12}} : undefined}}
                                        />
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
