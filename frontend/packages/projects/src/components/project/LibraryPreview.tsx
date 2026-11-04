import { useEffect, useState } from 'react';
import { Alert, Box, Button, CircularProgress, Dialog, DialogContent, IconButton, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { apiFetch } from '../../client';
import { Close, Download } from '../icons';
import { formatBytes } from '../../utils/format';

export interface PreviewFile { path: string; name: string; size: number }

/** Largest file fetched for a preview; above it only the download is offered. */
export const PREVIEW_LIMIT = { image: 25 * 1024 * 1024, text: 512 * 1024 };

const IMAGE_TYPES: Record<string, string> = {
    png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', webp: 'image/webp', svg: 'image/svg+xml',
};
const TEXT = new Set(['md', 'markdown', 'txt', 'log', 'json', 'csv', 'yaml', 'yml', 'toml', 'xml', 'ini', 'go', 'py', 'ts', 'tsx', 'js', 'jsx', 'sh', 'sql', 'html', 'css']);

const extOf = (name: string) => {
    const i = name.lastIndexOf('.');
    return i > 0 ? name.slice(i + 1).toLowerCase() : '';
};

/** What a library file previews as, or null when it is download-only. */
export function previewKind(name: string): 'image' | 'text' | null {
    const ext = extOf(name);
    if (ext in IMAGE_TYPES) return 'image';
    return TEXT.has(ext) ? 'text' : null;
}

type Body = { state: 'loading' } | { state: 'error'; message: string } | { state: 'image'; url: string } | { state: 'text'; text: string } | { state: 'large' };

/**
 * A modal preview of one library file: an image, or a text file as plain
 * text. The bytes come through `apiFetch` from the library file endpoint (so
 * the host's authentication and the endpoint's path checks apply), and an
 * image is shown through `<img>` from a blob typed by its extension, so an
 * SVG is never parsed as part of the page.
 */
export function LibraryPreview({ projectId, file, onClose, onDownload }: {
    projectId: string;
    file: PreviewFile | null;
    onClose: () => void;
    onDownload: (path: string) => void;
}) {
    const { t } = useTranslation('projects');
    // The file stays in the dialog while it fades out.
    const [shown, setShown] = useState<PreviewFile | null>(file);
    if (file && file.path !== shown?.path) setShown(file);
    const kind = shown ? previewKind(shown.name) : null;
    const tooLarge = !!shown && !!kind && shown.size > PREVIEW_LIMIT[kind];
    const [body, setBody] = useState<Body>({ state: 'loading' });

    useEffect(() => {
        if (!shown || !kind) return;
        if (tooLarge) { setBody({ state: 'large' }); return; }
        setBody({ state: 'loading' });
        let live = true;
        let url = '';
        (async () => {
            const res = await apiFetch(`/api/v1/projects/${projectId}/library/file?path=${encodeURIComponent(shown.path)}`);
            if (!res.ok) throw new Error(((await res.json().catch(() => ({}))) as { error?: string }).error ?? res.statusText);
            if (kind === 'text') return { state: 'text', text: await res.text() } as const;
            const blob = await res.blob();
            url = URL.createObjectURL(blob.slice(0, blob.size, IMAGE_TYPES[extOf(shown.name)]));
            return { state: 'image', url } as const;
        })().then(
            (b) => { if (live) setBody(b); else if (url) URL.revokeObjectURL(url); },
            (e: Error) => { if (live) setBody({ state: 'error', message: e.message }); },
        );
        return () => {
            live = false;
            if (url) URL.revokeObjectURL(url);
        };
    }, [projectId, shown, kind, tooLarge]);

    return (
        <Dialog open={!!file} onClose={onClose} maxWidth="lg" fullWidth aria-labelledby="library-preview-title"
            slotProps={{ transition: { onExited: () => setShown(null) } }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, pl: 3, pr: 1.5, pt: 1.5, pb: 1 }}>
                <Typography id="library-preview-title" component="h2" noWrap title={shown?.path} sx={{ flex: 1, minWidth: 0, fontFamily: fontMono, fontSize: fontSizes.base, fontWeight: 600 }}>
                    {shown?.name}
                </Typography>
                {shown && <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0, mr: 0.5 }}>{formatBytes(shown.size)}</Typography>}
                <Tooltip title={t('library.download')}>
                    <IconButton size="small" aria-label={t('library.download')} onClick={() => shown && onDownload(shown.path)}><Download fontSize="small" /></IconButton>
                </Tooltip>
                <Tooltip title={t('common.close')}>
                    <IconButton size="small" aria-label={t('common.close')} onClick={onClose}><Close fontSize="small" /></IconButton>
                </Tooltip>
            </Box>
            <DialogContent sx={{ pt: 0.5, display: 'flex', justifyContent: 'center', alignItems: body.state === 'text' ? 'flex-start' : 'center', minHeight: 160 }}>
                {body.state === 'loading' && <CircularProgress size={28} aria-label={t('tree.loading')} />}
                {body.state === 'error' && <Alert severity="error" sx={{ width: '100%' }}>{body.message || t('library.previewFailed')}</Alert>}
                {body.state === 'large' && shown && (
                    <Box sx={{ textAlign: 'center', color: 'text.secondary' }}>
                        <Typography variant="body2" sx={{ mb: 1.5 }}>{t('library.tooLarge')}</Typography>
                        <Button variant="outlined" size="small" startIcon={<Download />} onClick={() => onDownload(shown.path)}>{t('library.download')}</Button>
                    </Box>
                )}
                {body.state === 'image' && shown && (
                    <Box component="img" src={body.url} alt={shown.name}
                        sx={{ display: 'block', maxWidth: '100%', maxHeight: '72vh', objectFit: 'contain', borderRadius: radius.card, bgcolor: 'action.hover' }} />
                )}
                {body.state === 'text' && (
                    <Box component="pre" sx={{ m: 0, p: 1.5, width: '100%', maxHeight: '72vh', overflow: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: fontMono, fontSize: fontSizes.base, lineHeight: 1.55, bgcolor: 'action.hover', borderRadius: radius.card }}>{body.text}</Box>
                )}
            </DialogContent>
        </Dialog>
    );
}
