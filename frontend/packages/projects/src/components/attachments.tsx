import { useCallback, useEffect, useMemo, useState } from 'react';
import { Box, ButtonBase, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { apiFetch, type Attachment } from '../client';
import { Description } from './icons';
import { radius } from '@tingly/ui/theme/radius';
import { LibraryPreview, previewKind, saveFile, type PreviewFile } from './project/LibraryPreview';
import { useNotify } from '../hooks/useNotify';

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

/** Stores files for a message and returns what to send with it. */
export async function uploadAttachments(owner: AttachOwner, files: File[]): Promise<Attachment[]> {
    const form = new FormData();
    files.forEach((f) => form.append('files', f, f.name));
    const res = await apiFetch(`${base(owner)}/attachments`, { method: 'POST', body: form });
    const body = (await res.json().catch(() => ({}))) as { error?: string; attachments?: Attachment[] };
    if (!res.ok) throw new Error(body.error ?? res.statusText);
    return body.attachments ?? [];
}

/** Hands a file the preview does not know to the browser, as before: it opens or downloads as the browser decides. */
const openStored = async (url: string) => {
    const res = await apiFetch(url).catch(() => null);
    if (!res?.ok) return;
    window.open(URL.createObjectURL(await res.blob()), '_blank', 'noopener');
};

/** Loads a stored file through the host's fetch (so its auth applies); null once it is gone. */
function useStoredFile(srcOf: (path: string) => string, path: string, enabled: boolean): string | null | undefined {
    const [url, setUrl] = useState<string | null | undefined>(undefined);
    const source = srcOf(path);
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

function AttachmentChip({ srcOf, item, onOpen }: { srcOf: (path: string) => string; item: Attachment; onOpen: (item: Attachment) => void }) {
    const { t } = useTranslation('projects');
    // The same allowlist the library tree goes by, so everything previewable there previews here too.
    const kind = previewKind(item.name);
    const src = useStoredFile(srcOf, item.path, kind === 'image');
    const gone = src === null;
    if (kind === 'image' && src) {
        return (
            <ButtonBase onClick={() => onOpen(item)} aria-label={item.name} sx={{ borderRadius: radius.row, overflow: 'hidden', border: 1, borderColor: 'divider' }}>
                <Box component="img" src={src} alt={item.name} sx={{ display: 'block', maxHeight: 120, maxWidth: 200, objectFit: 'cover' }} />
            </ButtonBase>
        );
    }
    return (
        <ButtonBase
            onClick={() => (kind ? onOpen(item) : void openStored(srcOf(item.path)))}
            disabled={gone}
            sx={{ display: 'flex', alignItems: 'center', gap: 0.75, px: 1, py: 0.5, borderRadius: radius.row, border: 1, borderColor: 'divider', maxWidth: 240, textAlign: 'left' }}
        >
            <Description fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
            <Typography variant="caption" noWrap sx={{ minWidth: 0 }}>{gone ? t('attachments.removed', { name: item.name }) : item.name}</Typography>
            {!gone && <Typography variant="caption" color="text.disabled" sx={{ flexShrink: 0 }}>{formatSize(item.size)}</Typography>}
        </ButtonBase>
    );
}

/** The files a sent message carries: thumbnails for images, a preview chip for the kinds the library previews (Markdown included), a plain download for the rest. */
export function AttachmentChips({ owner, items, align = 'flex-end' }: { owner: AttachOwner; items: Attachment[]; align?: 'flex-start' | 'flex-end' }) {
    const notify = useNotify();
    const [preview, setPreview] = useState<PreviewFile | null>(null);
    const { threadId, projectId, draftFolderId } = owner;
    // Stable across renders, so a re-render of the chips does not refetch the open preview.
    const srcOf = useCallback((path: string) => (threadId
        ? `/api/v1/threads/${threadId}/attachments/file?path=${encodeURIComponent(path)}`
        : `/api/v1/projects/${projectId ?? draftFolderId ?? ''}/library/file?path=${encodeURIComponent(path)}`),
        [threadId, projectId, draftFolderId]);
    // The images among the attachments, in order: the preview steps through them like through a library folder.
    const images = useMemo(() => items.filter((a) => previewKind(a.name) === 'image').map(({ path, name, size }) => ({ path, name, size })), [items]);
    const download = (path: string) => {
        const name = items.find((a) => a.path === path)?.name ?? path.split('/').pop() ?? path;
        void saveFile(srcOf(path), name, (m) => notify(m, 'error'));
    };
    if (items.length === 0) return null;
    return (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75, justifyContent: align, mb: 0.75 }}>
            {items.map((a) => <AttachmentChip key={a.path} srcOf={srcOf} item={a} onOpen={setPreview} />)}
            <LibraryPreview srcOf={srcOf} file={preview} siblings={images} onNavigate={setPreview} onClose={() => setPreview(null)} onDownload={download} />
        </Box>
    );
}
