// Saving a blob to the user's disk, and the naming that goes with it.
//
// Deliberately not part of any one feature: the anchor-click dance had already
// been hand-rolled twice in this codebase before this module existed, and each
// copy learned (or failed to learn) the revoke timing separately.
//
// `downloadImage` below is the one export that isn't generic: it composes the
// anchor-click save here with `fetchBlob`/`extensionForMime` from
// `@tingly/vision`, which own those two (image-specific) concerns.

import { extensionForMime, fetchBlob } from '@tingly/vision';

export const downloadBlob = (blob: Blob, fileName: string): void => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = fileName;
    document.body.appendChild(link);
    link.click();
    link.remove();
    // Revoking synchronously can cancel the download in some browsers.
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
};

export const downloadText = (content: string, fileName: string, mimeType: string): void =>
    downloadBlob(new Blob([content], { type: mimeType }), fileName);

/** Filename-safe stem derived from free text, so downloads are recognisable. */
export const slugify = (text: string, maxLength = 32): string => {
    const slug = text
        .toLowerCase()
        .replace(/[^a-z0-9一-龥]+/g, '-')
        .replace(/^-+|-+$/g, '')
        .slice(0, maxLength)
        .replace(/-+$/g, '');
    return slug || 'image';
};

/** Fetches an image and saves it under `<stem>.<its own extension>`. */
export const downloadImage = async (src: string, stem: string): Promise<void> => {
    const blob = await fetchBlob(src);
    downloadBlob(blob, `${stem}.${extensionForMime(blob.type)}`);
};
