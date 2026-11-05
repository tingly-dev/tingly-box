import { useEffect, useState } from 'react';
import { Box, ButtonBase, Dialog, DialogContent, DialogTitle, IconButton, Typography } from '@mui/material';
import { Close } from '@tingly/ui/icons';
import { useTranslation } from 'react-i18next';
import { apiFetch, type Attachment } from '../client';
import { Description } from './icons';
import { radius } from '@tingly/ui/theme/radius';

/** A paste longer than this becomes a file instead of filling the box. */
export const PASTE_AS_FILE_CHARS = 4000;
export const PASTE_AS_FILE_LINES = 40;

export const lineCount = (text: string): number => text.split('\n').length;

export const isLongText = (text: string): boolean => text.length > PASTE_AS_FILE_CHARS || lineCount(text) > PASTE_AS_FILE_LINES;

/** The text file a long paste is stored as. */
export const pastedTextFile = (text: string): File => new File([text], 'pasted-text.txt', { type: 'text/plain' });

export const formatSize = (n: number): string => (n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : n >= 1 << 10 ? `${Math.round(n / (1 << 10))} KB` : `${n} B`);

/**
 * Whose uploads: a project's (its library), a folder thread's own, or a
 * folder's draft area (the first message of a thread that is not started).
 */
export interface AttachOwner {
    projectId?: string;
    threadId?: string;
    /** The folder project whose next thread the files are for. */
    draftFolderId?: string;
}

const base = (o: AttachOwner) => (o.threadId ? `/api/v1/threads/${o.threadId}` : `/api/v1/projects/${o.projectId ?? o.draftFolderId ?? ''}`);

const fileUrl = (o: AttachOwner, path: string) => (o.threadId
    ? `${base(o)}/attachments/file?path=${encodeURIComponent(path)}`
    : `${base(o)}/library/file?path=${encodeURIComponent(path)}`);

/** Stores files for a message and returns what to send with it. */
export async function uploadAttachments(owner: AttachOwner, files: File[]): Promise<Attachment[]> {
    const form = new FormData();
    files.forEach((f) => form.append('files', f, f.name));
    const res = await apiFetch(`${base(owner)}/attachments`, { method: 'POST', body: form });
    const body = (await res.json().catch(() => ({}))) as { error?: string; attachments?: Attachment[] };
    if (!res.ok) throw new Error(body.error ?? res.statusText);
    return body.attachments ?? [];
}

// Only these raster types are drawn. SVG is a document that can carry script and
// external references, so it stays a download chip like any other file.
const PREVIEWABLE_IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/bmp']);
export const isPreviewableImageType = (type: string) => PREVIEWABLE_IMAGE_TYPES.has(type.toLowerCase());
const isImage = (a: { type: string }) => isPreviewableImageType(a.type);

/** Loads a stored file through the host's fetch (so its auth applies); null once it is gone. */
function useStoredFile(owner: AttachOwner, path: string, enabled: boolean): string | null | undefined {
    const [url, setUrl] = useState<string | null | undefined>(undefined);
    const source = fileUrl(owner, path);
    useEffect(() => {
        if (!enabled) return;
        let made: string | undefined;
        let live = true;
        apiFetch(source)
            .then((r) => (r.ok ? r.blob() : Promise.reject(new Error(r.statusText))))
            .then((b) => { if (live) { made = URL.createObjectURL(b); setUrl(made); } })
            .catch(() => { if (live) setUrl(null); });
        return () => { live = false; if (made) URL.revokeObjectURL(made); };
    }, [source, enabled]);
    return url;
}

function AttachmentChip({ owner, item }: { owner: AttachOwner; item: Attachment }) {
    const { t } = useTranslation('projects');
    const [previewOpen, setPreviewOpen] = useState(false);
    const image = isImage(item);
    const src = useStoredFile(owner, item.path, image);
    const open = async () => {
        const res = await apiFetch(fileUrl(owner, item.path)).catch(() => null);
        if (!res?.ok) return;
        window.open(URL.createObjectURL(await res.blob()), '_blank', 'noopener');
    };
    if (image && src) {
        return (
            <>
                <ButtonBase onClick={() => setPreviewOpen(true)} aria-label={item.name} sx={{ borderRadius: radius.row, overflow: 'hidden', border: 1, borderColor: 'divider' }}>
                    <Box component="img" src={src} alt={item.name} sx={{ display: 'block', maxHeight: 120, maxWidth: 200, objectFit: 'cover' }} />
                </ButtonBase>
                <Dialog open={previewOpen} onClose={() => setPreviewOpen(false)} maxWidth="lg" fullWidth aria-labelledby="attachment-preview-title">
                    <DialogTitle id="attachment-preview-title" sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 1 }}>
                        <Typography component="span" noWrap sx={{ flex: 1 }}>{item.name}</Typography>
                        <IconButton onClick={() => setPreviewOpen(false)} aria-label={t('common.close')}><Close /></IconButton>
                    </DialogTitle>
                    <DialogContent sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', bgcolor: 'action.hover', minHeight: 200 }}>
                        <Box component="img" src={src} alt={item.name} sx={{ display: 'block', maxWidth: '100%', maxHeight: '75vh', objectFit: 'contain' }} />
                    </DialogContent>
                </Dialog>
            </>
        );
    }
    const gone = src === null;
    return (
        <ButtonBase
            onClick={() => void open()}
            disabled={gone}
            sx={{ display: 'flex', alignItems: 'center', gap: 0.75, px: 1, py: 0.5, borderRadius: radius.row, border: 1, borderColor: 'divider', maxWidth: 240, textAlign: 'left' }}
        >
            <Description fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
            <Typography variant="caption" noWrap sx={{ minWidth: 0 }}>{gone ? t('attachments.removed', { name: item.name }) : item.name}</Typography>
            {!gone && <Typography variant="caption" color="text.disabled" sx={{ flexShrink: 0 }}>{formatSize(item.size)}</Typography>}
        </ButtonBase>
    );
}

/** The files a sent message carries: thumbnails for images, chips for the rest. */
export function AttachmentChips({ owner, items, align = 'flex-end' }: { owner: AttachOwner; items: Attachment[]; align?: 'flex-start' | 'flex-end' }) {
    if (items.length === 0) return null;
    return (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75, justifyContent: align, mb: 0.75 }}>
            {items.map((a) => <AttachmentChip key={a.path} owner={owner} item={a} />)}
        </Box>
    );
}
