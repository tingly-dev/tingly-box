import { useEffect, useState } from 'react';
import { Alert, Box, Breadcrumbs, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono } from '@tingly/ui/theme/fonts';
import { apiFetch } from '../../client';
import { ArrowUpward, ChevronRight, Folder, Home } from '../icons';

interface Listing { home: string; path: string; directories: string[] }

/** Browse directories on the server; selection alone never opens or changes one. */
export function DirectoryPicker({ onClose, onSelect }: { onClose: () => void; onSelect: (path: string) => void }) {
    const { t } = useTranslation('projects');
    // Paths sent to the server are relative to its home, with / between components.
    const [request, setRequest] = useState({ path: '.' });
    const [listing, setListing] = useState<Listing | null>(null);
    const [error, setError] = useState('');
    const navigate = (path: string) => { setListing(null); setError(''); setRequest({ path }); };
    useEffect(() => {
        const controller = new AbortController();
        void apiFetch('/api/v1/projects/directories', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: request.path }), signal: controller.signal })
            .then(async (response) => {
                if (!response.ok) throw new Error(t(response.status === 401 ? 'directory.unauthorized' : response.status === 403 ? 'directory.blocked' : 'directory.failed'));
                return response.json() as Promise<Listing>;
            })
            .then((result) => { if (!controller.signal.aborted) setListing(result); })
            .catch((e: Error) => { if (!controller.signal.aborted) setError(e.message || t('directory.failed')); });
        return () => controller.abort();
    }, [request, t]);

    const parts = request.path === '.' ? [] : request.path.split('/');
    const parent = parts.length <= 1 ? '.' : parts.slice(0, -1).join('/');
    // On Unix, a backslash can be part of a directory name; only translate it for Windows paths.
    const windows = !!listing && (/^[a-zA-Z]:[\\/]/.test(listing.home) || listing.home.startsWith('\\\\'));
    const relative = listing && windows ? listing.path.replace(/\//g, '\\') : listing?.path;
    const fullPath = listing ? listing.path === '.' ? listing.home : listing.home.replace(windows ? /[\\/]+$/ : /\/+$/, '') + (windows ? '\\' : '/') + relative : '';
    return (
        <Dialog open onClose={onClose} fullWidth maxWidth="sm" aria-labelledby="directory-picker-title"
            slotProps={{ paper: { sx: { height: { xs: 'min(620px, 90dvh)', sm: 560 }, maxHeight: '90dvh' } } }}>
            <DialogTitle id="directory-picker-title" sx={{ pb: 1.5 }}>{t('directory.title')}</DialogTitle>
            <DialogContent sx={{ p: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                <Box sx={{ display: 'flex', flex: 1, minHeight: 0, borderTop: 1, borderBottom: 1, borderColor: 'divider' }}>
                    <Box component="nav" aria-label={t('directory.places')} sx={{ width: { xs: 56, sm: 150 }, flexShrink: 0, bgcolor: 'action.hover', borderRight: 1, borderColor: 'divider', p: 1 }}>
                        <Typography variant="overline" color="text.secondary" sx={{ px: 1, display: { xs: 'none', sm: 'block' }, lineHeight: 2 }}>{t('directory.places')}</Typography>
                        <ListItemButton onClick={() => navigate('.')} selected={parts.length === 0} aria-label={t('directory.home')} sx={{ borderRadius: 1, gap: 1, px: 1, minHeight: 36 }}>
                            <Home fontSize="small" /><Typography variant="body2" sx={{ display: { xs: 'none', sm: 'block' } }}>{t('directory.home')}</Typography>
                        </ListItemButton>
                    </Box>
                    <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, px: 1, py: 0.75, borderBottom: 1, borderColor: 'divider' }}>
                            <Tooltip title={t('directory.parent')}><span><IconButton size="small" aria-label={t('directory.parent')} disabled={parts.length === 0} onClick={() => navigate(parent)}><ArrowUpward fontSize="small" /></IconButton></span></Tooltip>
                            <Breadcrumbs aria-label={t('directory.currentLocation')} maxItems={4} sx={{ flex: 1, minWidth: 0, '& ol': { flexWrap: 'nowrap' }, overflow: 'auto' }}>
                                <Button size="small" onClick={() => navigate('.')} sx={{ minWidth: 0, whiteSpace: 'nowrap' }}>{t('directory.home')}</Button>
                                {parts.map((part, i) => <Button size="small" key={parts.slice(0, i + 1).join('/')} onClick={() => navigate(parts.slice(0, i + 1).join('/'))} sx={{ minWidth: 0, whiteSpace: 'nowrap' }}>{part}</Button>)}
                            </Breadcrumbs>
                        </Box>
                        <Box sx={{ flex: 1, minHeight: 0, overflow: 'auto', p: 1 }}>
                            {error && <Alert severity="error" action={<Button color="inherit" onClick={() => navigate(request.path)}>{t('directory.retry')}</Button>}>{error}</Alert>}
                            {!listing && !error && <Box role="status" sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 2, color: 'text.secondary' }}><CircularProgress size={16} /><Typography variant="body2">{t('directory.loading')}</Typography></Box>}
                            {listing && (listing.directories.length === 0 ? <Typography color="text.secondary" variant="body2" sx={{ p: 2 }}>{t('directory.empty')}</Typography> : (
                                <List dense disablePadding aria-label={t('directory.folders')}>
                                    {listing.directories.map((name) => <ListItemButton key={name} onClick={() => navigate(request.path === '.' ? name : `${request.path}/${name}`)} sx={{ borderRadius: 1, minHeight: 40 }}>
                                        <ListItemIcon sx={{ minWidth: 36, color: 'primary.main' }}><Folder fontSize="small" /></ListItemIcon>
                                        <ListItemText primary={name} slotProps={{ primary: { noWrap: true, variant: 'body2' } }} />
                                        <ChevronRight fontSize="small" color="disabled" />
                                    </ListItemButton>)}
                                </List>
                            ))}
                        </Box>
                        <Typography variant="caption" color="text.secondary" sx={{ px: 2, pb: 1 }}>{t('directory.help')}</Typography>
                    </Box>
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 2, gap: 1, flexWrap: 'wrap' }}>
                <Box sx={{ flex: { xs: '1 0 100%', sm: '1 1 auto' }, minWidth: 0 }}>
                    <Typography variant="caption" color="text.secondary">{t('directory.selectedLocation')}</Typography>
                    <Typography variant="body2" noWrap title={fullPath} sx={{ fontFamily: fontMono }}>{fullPath || '—'}</Typography>
                </Box>
                <Button onClick={onClose}>{t('common.cancel')}</Button>
                <Button variant="contained" disabled={!listing} onClick={() => onSelect(fullPath)}>{t('directory.select')}</Button>
            </DialogActions>
        </Dialog>
    );
}
