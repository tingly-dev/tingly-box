import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { Alert, Box, Button, CircularProgress, Dialog, DialogContent, IconButton, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { apiFetch } from '../../client';
import { ChevronLeft, ChevronRight, Close, Download } from '../icons';
import { Markdown } from '../Markdown';
import { formatBytes } from '../../utils/format';

export interface PreviewFile { path: string; name: string; size: number }

/** Largest file fetched for a preview; above it only the download is offered. */
export const PREVIEW_LIMIT = { image: 25 * 1024 * 1024, text: 512 * 1024 };

// The previewable types are an allowlist. Raster images only: SVG is a
// document that can carry script and external references, so it stays
// download-only. Text is limited to formats that are plain text by nature.
const IMAGE_TYPES: Record<string, string> = {
    png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', webp: 'image/webp',
};
const TEXT = new Set(['md', 'markdown', 'txt', 'log', 'json', 'csv', 'yaml', 'yml', 'toml', 'ini']);

const extOf = (name: string) => {
    const i = name.lastIndexOf('.');
    return i > 0 ? name.slice(i + 1).toLowerCase() : '';
};

/** Whether a text file is shown rendered; the other text types stay plain text. */
export const isMarkdown = (name: string) => ['md', 'markdown'].includes(extOf(name));

/** What a library file previews as, or null when it is download-only. */
export function previewKind(name: string): 'image' | 'text' | null {
    const ext = extOf(name);
    if (ext in IMAGE_TYPES) return 'image';
    return TEXT.has(ext) ? 'text' : null;
}

const ascii = (b: Uint8Array, at: number, s: string) => [...s].every((c, i) => b[at + i] === c.charCodeAt(0));

/** Whether the file's first bytes are the image format its extension claims. */
export function sniffImage(head: Uint8Array, ext: string): boolean {
    switch (ext) {
        case 'png': return head[0] === 0x89 && ascii(head, 1, 'PNG\r\n') && head[6] === 0x1a && head[7] === 0x0a;
        case 'jpg': case 'jpeg': return head[0] === 0xff && head[1] === 0xd8 && head[2] === 0xff;
        case 'gif': return ascii(head, 0, 'GIF87a') || ascii(head, 0, 'GIF89a');
        case 'webp': return ascii(head, 0, 'RIFF') && ascii(head, 8, 'WEBP');
        default: return false;
    }
}

const NO_SIBLINGS: PreviewFile[] = [];

type Body = { state: 'loading' } | { state: 'error'; message: string } | { state: 'image'; url: string } | { state: 'text'; text: string; markdown: boolean } | { state: 'large' };

/**
 * A modal preview of one library file: an image, a Markdown file rendered
 * like a conversation reply (safe mode: no HTML, no remote images, only
 * http/https/mailto links), or another text file as plain text. With
 * `siblings` (the folder's images, in tree order) an image preview steps
 * through them with Left/Right/Up/Down or the header buttons, stopping at
 * the first and last. The bytes come through `apiFetch` from the library file endpoint (so
 * the host's authentication and the endpoint's path checks apply). An image
 * is shown through `<img>` from a blob typed by its extension, after its
 * first bytes are checked to be that format, so a file that only has an image
 * name is never handed to the browser; text is set as a React text node.
 */
export function LibraryPreview({ projectId, file, siblings = NO_SIBLINGS, onClose, onDownload, onNavigate }: {
    projectId: string;
    file: PreviewFile | null;
    siblings?: PreviewFile[];
    onClose: () => void;
    onDownload: (path: string) => void;
    onNavigate?: (file: PreviewFile) => void;
}) {
    const { t } = useTranslation('projects');
    // The file stays in the dialog while it fades out.
    const [shown, setShown] = useState<PreviewFile | null>(file);
    if (file && file.path !== shown?.path) setShown(file);
    const kind = shown ? previewKind(shown.name) : null;
    const tooLarge = !!shown && !!kind && shown.size > PREVIEW_LIMIT[kind];
    const [body, setBody] = useState<Body>({ state: 'loading' });
    const paper = useRef<HTMLDivElement>(null);
    const index = shown && kind === 'image' ? siblings.findIndex((x) => x.path === shown.path) : -1;
    const nav = index >= 0 && siblings.length > 1 && !!onNavigate;
    const go = (to: number) => { if (nav && to >= 0 && to < siblings.length) onNavigate(siblings[to]); };

    const onKeyDown = (e: KeyboardEvent) => {
        if (!nav || e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
        const step = e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : 0;
        if (!step) return;
        e.preventDefault();
        go(index + step);
    };

    // Stepping onto the first or last image disables the button that was just used;
    // keep the focus in the dialog so the arrow keys and Esc still reach it.
    useEffect(() => {
        const active = document.activeElement as HTMLButtonElement | null;
        if (shown && paper.current && (!paper.current.contains(active) || active?.disabled)) paper.current.focus();
    }, [shown, index]);

    useEffect(() => {
        if (!shown || !kind) return;
        if (tooLarge) { setBody({ state: 'large' }); return; }
        setBody({ state: 'loading' });
        let live = true;
        let url = '';
        (async () => {
            const res = await apiFetch(`/api/v1/projects/${projectId}/library/file?path=${encodeURIComponent(shown.path)}`);
            if (!res.ok) throw new Error(((await res.json().catch(() => ({}))) as { error?: string }).error ?? res.statusText);
            if (kind === 'text') {
                const text = await res.text();
                if (text.includes('\u0000')) throw new Error('');
                return { state: 'text', text, markdown: isMarkdown(shown.name) } as const;
            }
            const blob = await res.blob();
            const ext = extOf(shown.name);
            if (!sniffImage(new Uint8Array(await blob.slice(0, 16).arrayBuffer()), ext)) throw new Error('');
            url = URL.createObjectURL(blob.slice(0, blob.size, IMAGE_TYPES[ext]));
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
        <Dialog open={!!file} onClose={onClose} maxWidth="lg" fullWidth aria-labelledby="library-preview-title" onKeyDown={onKeyDown}
            slotProps={{ transition: { onExited: () => setShown(null) }, paper: { ref: paper, tabIndex: -1 } }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, pl: 3, pr: 1.5, pt: 1.5, pb: 1 }}>
                <Typography id="library-preview-title" component="h2" noWrap title={shown?.path} sx={{ flex: 1, minWidth: 0, fontFamily: fontMono, fontSize: fontSizes.base, fontWeight: 600 }}>
                    {shown?.name}
                </Typography>
                {shown && <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0, mr: 0.5 }}>{formatBytes(shown.size)}</Typography>}
                {nav && (
                    <>
                        <Tooltip title={`${t('library.previousImage')} (←)`}>
                            <span><IconButton size="small" aria-label={t('library.previousImage')} disabled={index === 0} onClick={() => go(index - 1)}><ChevronLeft fontSize="small" /></IconButton></span>
                        </Tooltip>
                        <Typography variant="caption" color="text.secondary" aria-live="polite" sx={{ flexShrink: 0, minWidth: 40, textAlign: 'center', fontVariantNumeric: 'tabular-nums' }}>{index + 1} / {siblings.length}</Typography>
                        <Tooltip title={`${t('library.nextImage')} (→)`}>
                            <span><IconButton size="small" aria-label={t('library.nextImage')} disabled={index === siblings.length - 1} onClick={() => go(index + 1)}><ChevronRight fontSize="small" /></IconButton></span>
                        </Tooltip>
                    </>
                )}
                <Tooltip title={t('library.download')}>
                    <IconButton size="small" aria-label={t('library.download')} onClick={() => shown && onDownload(shown.path)}><Download fontSize="small" /></IconButton>
                </Tooltip>
                <Tooltip title={t('common.close')}>
                    <IconButton size="small" aria-label={t('common.close')} onClick={onClose}><Close fontSize="small" /></IconButton>
                </Tooltip>
            </Box>
            <DialogContent sx={{ pt: 0.5, display: 'flex', justifyContent: 'center', alignItems: body.state === 'text' ? 'flex-start' : 'center', minHeight: 160 }}>
                {body.state === 'loading' && <CircularProgress size={28} aria-label={t('tree.loading')} />}
                {body.state === 'error' && <Alert severity="error" sx={{ width: '100%' }}>{body.message || t('library.notPreviewable')}</Alert>}
                {body.state === 'large' && shown && (
                    <Box sx={{ textAlign: 'center', color: 'text.secondary' }}>
                        <Typography variant="body2" sx={{ mb: 1.5 }}>{t('library.tooLarge')}</Typography>
                        <Button variant="outlined" size="small" startIcon={<Download />} onClick={() => onDownload(shown.path)}>{t('library.download')}</Button>
                    </Box>
                )}
                {body.state === 'image' && shown && (
                    <Box component="img" src={body.url} alt={shown.name} onError={() => setBody({ state: 'error', message: '' })}
                        sx={{ display: 'block', maxWidth: '100%', maxHeight: '72vh', objectFit: 'contain', borderRadius: radius.card, bgcolor: 'action.hover' }} />
                )}
                {body.state === 'text' && body.markdown && (
                    <Box sx={{ width: '100%', maxWidth: 880, maxHeight: '72vh', overflow: 'auto', p: 1.5 }}>
                        <Markdown conversation safe text={body.text} />
                    </Box>
                )}
                {body.state === 'text' && !body.markdown && (
                    <Box component="pre" sx={{ m: 0, p: 1.5, width: '100%', maxHeight: '72vh', overflow: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: fontMono, fontSize: fontSizes.base, lineHeight: 1.55, bgcolor: 'action.hover', borderRadius: radius.card }}>{body.text}</Box>
                )}
            </DialogContent>
        </Dialog>
    );
}
