import { formatBytes } from './format';

/**
 * The most one uploaded file may weigh (200 MiB). This is the server's
 * per-file limit (`MaxUploadBytes` in
 * projects/internal/project/attachment.go); the upload routes answer a larger
 * file with 413. There is one limit and no API reads it for us, so this is a
 * copy: keep it equal — the Go test TestWebClientUploadLimitMatchesTheServer
 * fails if the two drift. If the limit ever becomes configurable, read it from
 * the server instead of this constant.
 */
export const MAX_UPLOAD_BYTES = 200 << 20;

/** "200 MB": the per-file limit as the person sees it. */
export const uploadLimitLabel = (): string => formatBytes(MAX_UPLOAD_BYTES);

/** A file the server would refuse, with how far past the limit it is. */
export interface RejectedUpload {
    file: File;
    size: number;
    /** Bytes over the limit (always > 0). */
    over: number;
}

/**
 * Splits files into what the server will take and what it would refuse, so the
 * UI can refuse an oversized one before its bytes go up instead of after a
 * full upload fails with 413. Size only: the server has no type allowlist, so
 * there is none to mirror here.
 */
export function precheckUploads(files: File[]): { accepted: File[]; rejected: RejectedUpload[] } {
    const accepted: File[] = [];
    const rejected: RejectedUpload[] = [];
    for (const file of files) {
        if (file.size > MAX_UPLOAD_BYTES) rejected.push({ file, size: file.size, over: file.size - MAX_UPLOAD_BYTES });
        else accepted.push(file);
    }
    return { accepted, rejected };
}
