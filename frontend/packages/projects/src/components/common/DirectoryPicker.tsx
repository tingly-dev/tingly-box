import { useEffect, useState } from 'react';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, List, ListItemButton, ListItemText, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { apiFetch } from '../../client';

interface Listing { home: string; path: string; directories: string[] }

/** Browse directories on the server; selection alone never opens or changes one. */
export function DirectoryPicker({ onClose, onSelect }: { onClose: () => void; onSelect: (path: string) => void }) {
    const { t } = useTranslation('projects');
    const [path, setPath] = useState('.');
    const [listing, setListing] = useState<Listing | null>(null);
    const [error, setError] = useState('');
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        const controller = new AbortController();
        void apiFetch('/api/v1/projects/directories', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path }), signal: controller.signal })
            .then(async (response) => { if (!response.ok) throw new Error(t(response.status === 401 ? 'directory.unauthorized' : response.status === 403 ? 'directory.blocked' : 'directory.failed'));  return response.json() as Promise<Listing>; })
            .then(setListing)
            .catch((e: Error) => { if (e.name !== 'AbortError') setError(e.message || t('directory.failed'));  });
        return () => controller.abort();
    }, [path, t, attempt]);
    const navigate = (next: string) => { setListing(null); setError(''); setPath(next); };
    const separator = listing?.home.includes('\\') ? '\\' : '/';
    const fullPath = listing ? listing.path === '.' ? listing.home : listing.home.replace(/[/\\]+$/, '') + separator + listing.path.replace(/[/\\]/g, separator) : '';
    return (
        <Dialog open onClose={onClose} fullWidth maxWidth="sm" aria-labelledby="directory-picker-title">
            <DialogTitle id="directory-picker-title">{t('directory.title')}</DialogTitle>
            <DialogContent>
                <DialogContentText>{t('directory.help')}</DialogContentText>
                {listing && <Typography sx={{ my: 1, overflowWrap: 'anywhere' }}>{fullPath}</Typography>}
                {error && <Alert severity="error" action={<Button color="inherit" onClick={() => { setError(''); setAttempt((n) => n + 1); }}>{t('directory.retry')}</Button>}>{error}</Alert>}
                {!listing && !error && <Typography>{t('directory.loading')}</Typography>}
                {listing && <Box sx={{ maxHeight: 300, overflow: 'auto' }}>
                    <List dense>
                        {path !== '.' && <ListItemButton onClick={() => navigate(path.split('/').slice(0, -1).join('/') || '.')}><ListItemText primary=".." /></ListItemButton>}
                        {listing.directories.map((name) => <ListItemButton key={name} onClick={() => navigate(path === '.' ? name : `${path}/${name}`)}><ListItemText primary={name} /></ListItemButton>)}
                    </List>
                    {listing.directories.length === 0 && <Typography color="text.secondary">{t('directory.empty')}</Typography>}
                </Box>}
            </DialogContent>
            <DialogActions><Button onClick={onClose}>{t('common.cancel')}</Button><Button variant="contained" disabled={!listing} onClick={() => onSelect(fullPath)}>{t('directory.select')}</Button></DialogActions>
        </Dialog>
    );
}
