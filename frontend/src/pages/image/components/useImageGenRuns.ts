import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchBlob } from '@tingly/vision';
import { getOpenAIClient } from '@/services/modelApi';
import { loadPlaygroundSession, savePlaygroundSession } from '@/utils/playgroundSession';
import { readImageSize } from './useImageGenRefs';
import type { ReferenceImage } from './ImageGenReferenceImages';
import type { Endpoint, GenerationRun, ImportedImage, Quality, ReferenceMask } from './ImageGenPlayground.types';

const IMAGE_SCENARIO = 'imagegen';

// Keep playground output while navigating between pages in the current app session.
// This deliberately stays in memory: base64 images can quickly exceed sessionStorage quotas.
let imageGenSessionRuns: GenerationRun[] = [];
let imageGenSessionImports: ImportedImage[] = [];

export interface GenerationRequest {
    prompt: string;
    model: string;
    size: string;
    quality: Quality;
    count: number;
    sources: { file: File; previewUrl: string; mask?: ReferenceMask }[];
    // Set when re-running a failed run: its card flips back to pending
    // instead of a second card appearing.
    runId?: string;
}

// Reads a File into a base64 data URL, the same representation already used
// for generated images (`data:image/png;base64,...`) so reference thumbnails
// and outputs render through one code path.
const fileToDataUrl = (file: File): Promise<string> => new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
});

type UseImageGenRunsNotification = (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;

// The session half of the playground: the run/import history that survives
// page navigation (module-level copy) and page reloads (IndexedDB), plus the
// submission side of a run — firing the request, cancelling it, retrying a
// failed one.
export const useImageGenRuns = (showNotification: UseImageGenRunsNotification) => {
    const { t } = useTranslation();
    const [runs, setRuns] = useState<GenerationRun[]>(() => imageGenSessionRuns);
    const [imported, setImported] = useState<ImportedImage[]>(() => imageGenSessionImports);
    const historyTrackRef = useRef<HTMLDivElement>(null);
    // The in-flight request behind each pending card, so its Cancel button
    // can abort the fetch instead of leaving the user to wait out the
    // gateway's timeout on a provider that has stopped answering.
    const inFlightRef = useRef(new Map<string, AbortController>());
    const pendingCount = runs.filter((run) => run.status === 'pending').length;

    // The session's two halves have one writer each: the module-level copy (what
    // survives navigation between pages) and the React state must move together,
    // and a caller that has to remember both in the right order eventually won't.
    const updateRuns = useCallback((updater: (currentRuns: GenerationRun[]) => GenerationRun[]) => {
        imageGenSessionRuns = updater(imageGenSessionRuns);
        setRuns(imageGenSessionRuns);
    }, []);

    const updateImports = useCallback((updater: (currentImports: ImportedImage[]) => ImportedImage[]) => {
        imageGenSessionImports = updater(imageGenSessionImports);
        setImported(imageGenSessionImports);
    }, []);

    // The session outlives a reload: whatever the last visit left in
    // IndexedDB comes back on mount (only when memory is empty — navigating
    // between pages keeps the in-memory copy, which is newer), and every
    // change is written back. A run that was still pending when the page went
    // away can never complete, so it comes back as failed rather than as a
    // spinner that spins forever.
    const [sessionRestored, setSessionRestored] = useState(false);
    useEffect(() => {
        let cancelled = false;
        if (imageGenSessionRuns.length > 0 || imageGenSessionImports.length > 0) {
            setSessionRestored(true);
            return undefined;
        }
        void loadPlaygroundSession<GenerationRun, ImportedImage>().then(({ runs: savedRuns, imports: savedImports }) => {
            if (cancelled) return;
            if (savedRuns.length > 0) {
                updateRuns((current) => (current.length > 0 ? current : savedRuns.map((run) => (run.status === 'pending'
                    ? { ...run, status: 'failed', error: t('playground.interruptedByReload', { defaultValue: 'Interrupted by a page reload' }) }
                    : run))));
            }
            if (savedImports.length > 0) {
                updateImports((current) => (current.length > 0 ? current : savedImports));
            }
            setSessionRestored(true);
        });
        return () => { cancelled = true; };
    }, [t, updateImports, updateRuns]);

    useEffect(() => {
        if (!sessionRestored) return;
        void savePlaygroundSession({ runs, imports: imported });
    }, [imported, runs, sessionRestored]);

    useEffect(() => {
        const frame = requestAnimationFrame(() => {
            const track = historyTrackRef.current;
            if (track) track.scrollTo({ left: track.scrollWidth, behavior: 'smooth' });
        });
        return () => cancelAnimationFrame(frame);
    }, [imported.length, pendingCount, runs.length]);

    // One path for a fresh request and a retry, so a retry is the same call
    // the original was and not a re-implementation that drifts.
    const runGeneration = useCallback(async (request: GenerationRequest) => {
        const runId = request.runId ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
        // The endpoint is a consequence of the inputs, not a mode the user
        // picks: references present → edits, none → generations. Which
        // providers can serve either is the gateway's concern, not this
        // panel's (see .design/imageedit.md).
        const endpoint: Endpoint = request.sources.length > 0 ? 'edits' : 'generations';
        const pendingRun: GenerationRun = {
            id: runId,
            createdAt: Date.now(),
            endpoint,
            prompt: request.prompt,
            model: request.model,
            size: request.size,
            quality: request.quality,
            count: request.count,
            images: [],
            sourceImages: endpoint === 'edits' ? request.sources.map((ref) => ref.previewUrl) : undefined,
            mask: request.sources[0]?.mask,
            status: 'pending',
        };
        updateRuns((currentRuns) => (currentRuns.some((run) => run.id === runId)
            ? currentRuns.map((run) => (run.id === runId ? { ...pendingRun, createdAt: run.createdAt } : run))
            : [...currentRuns, pendingRun]));
        const controller = new AbortController();
        inFlightRef.current.set(runId, controller);
        try {
            const client = await getOpenAIClient(IMAGE_SCENARIO);
            const editFiles = request.sources.map((ref) => ref.file);
            // The mask belongs to the first reference because that is the one
            // the API applies it to; nothing here chooses which image it is.
            const mask = request.sources[0]?.mask?.file;
            const response = endpoint === 'edits'
                ? await client.images.edit({
                    image: editFiles.length === 1 ? editFiles[0] : editFiles,
                    ...(mask ? { mask } : {}),
                    model: request.model,
                    prompt: request.prompt,
                    n: request.count,
                    size: request.size as any,
                    quality: request.quality as any,
                }, { signal: controller.signal })
                : await client.images.generate({
                    model: request.model,
                    prompt: request.prompt,
                    n: request.count,
                    size: request.size as any,
                    quality: request.quality,
                }, { signal: controller.signal });
            const images = response.data ?? [];
            updateRuns((currentRuns) => currentRuns.map((run) => (
                run.id === runId ? { ...run, images, status: 'completed', error: undefined } : run
            )));
        } catch (error: any) {
            if (controller.signal.aborted) {
                // The user asked for this; the card records it so the request
                // is still there to retry, and no toast second-guesses them.
                updateRuns((currentRuns) => currentRuns.map((run) => (
                    run.id === runId
                        ? { ...run, status: 'failed', error: t('playground.cancelled', { defaultValue: 'Cancelled' }) }
                        : run
                )));
                return;
            }
            const status = error?.status ? `${error.status}: ` : '';
            const message = error?.error?.message || error?.message || t('playground.requestFailed', { defaultValue: 'Request failed' });
            updateRuns((currentRuns) => currentRuns.map((run) => (
                run.id === runId ? { ...run, status: 'failed', error: `${status}${message}` } : run
            )));
            showNotification(`${status}${message}`, 'error');
        } finally {
            inFlightRef.current.delete(runId);
        }
    }, [showNotification, t, updateRuns]);

    const handleCancelRun = useCallback((id: string) => {
        inFlightRef.current.get(id)?.abort();
    }, []);

    // A run's references only survive as data URLs, so they are read back into
    // files here — the shape both retry (same request again) and re-entry (same
    // request, editable) need. The bytes and the pixel size come from two
    // independent decodes of the same data URL, so they are awaited together
    // rather than one after the other.
    const runSourcesToReferences = useCallback((run: GenerationRun): Promise<ReferenceImage[]> => Promise.all(
        (run.sourceImages ?? []).map(async (src, index): Promise<ReferenceImage> => {
            const [blob, size] = await Promise.all([fetchBlob(src), readImageSize(src)]);
            return {
                file: new File([blob], `reference-${index + 1}.png`, { type: blob.type || 'image/png' }),
                previewUrl: src,
                source: 'upload',
                ...(size ?? {}),
                // The mask comes back with the image it was painted on, so a
                // retry is the request that failed rather than a repaint of
                // the whole image, and re-entry hands back a mask that can
                // still be edited.
                ...(index === 0 && run.mask ? { mask: run.mask } : {}),
            };
        }),
    ), []);

    const handleRetry = useCallback(async (run: GenerationRun) => {
        try {
            const sources = await runSourcesToReferences(run);
            await runGeneration({
                prompt: run.prompt,
                model: run.model,
                size: run.size,
                quality: run.quality,
                count: run.count ?? 1,
                sources,
                runId: run.id,
            });
        } catch {
            showNotification(t('playground.requestFailed', { defaultValue: 'Request failed' }), 'error');
        }
    }, [runGeneration, runSourcesToReferences, showNotification, t]);

    // Brings images into the results panel. Same decoding as a reference (data
    // URL up front, so one representation renders everywhere), different
    // destination: nothing here is sent to a model.
    const handleImportImages = useCallback(async (files: FileList | File[]) => {
        const incoming = Array.from(files).filter((file) => file.type.startsWith('image/'));
        if (incoming.length === 0) return;
        const items = await Promise.all(incoming.map(async (file): Promise<ImportedImage> => {
            const src = await fileToDataUrl(file);
            return {
                id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
                src,
                name: file.name,
                bytes: file.size,
                createdAt: Date.now(),
                ...(await readImageSize(src) ?? {}),
            };
        }));
        updateImports((current) => [...current, ...items]);
    }, [updateImports]);

    const removeRun = useCallback((id: string) => {
        updateRuns((currentRuns) => currentRuns.filter((run) => run.id !== id));
    }, [updateRuns]);

    const removeImport = useCallback((id: string) => {
        updateImports((current) => current.filter((item) => item.id !== id));
    }, [updateImports]);

    return {
        runs,
        imported,
        pendingCount,
        historyTrackRef,
        inFlightRef,
        updateRuns,
        updateImports,
        runGeneration,
        handleCancelRun,
        handleRetry,
        runSourcesToReferences,
        handleImportImages,
        removeRun,
        removeImport,
    };
};
