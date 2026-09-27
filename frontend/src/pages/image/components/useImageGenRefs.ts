import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchBlob, parseImageSize } from '@tingly/vision';
import { addReferences, reorderReferences } from './imageGenSession';
import { fileToDataUrl, readImageSize } from './imageFiles';
import type { ReferenceMask } from './ImageGenPlayground.types';
import type { SketchLayers, SketchResult } from './SketchCanvasDialog';
import { MAX_EDIT_REFERENCE_IMAGES, type ReferenceImage } from './ImageGenReferenceImages';

// An image that is already a data URL (an output, an import, a kept library
// image) as a reference. The src doubles as the preview, so it is never
// re-encoded.
const srcToReference = async (src: string, name: string): Promise<ReferenceImage> => {
    const blob = await fetchBlob(src);
    return {
        file: new File([blob], name, { type: blob.type || 'image/png' }),
        previewUrl: src,
        source: 'upload',
        ...(await readImageSize(src) ?? {}),
    };
};

// Which sketch the canvas dialog is working on: `null` closed, `index: null`
// a new sketch, otherwise the reference image being redrawn.
type SketchTarget = { index: number | null } | null;

interface UseImageGenRefsParams {
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    // The panel's size selection — a new sketch starts at the canvas size the
    // request would use.
    size: string;
}

// Everything waiting in the request's reference row: the images themselves,
// their drag-to-reorder state, the sketch canvas that is one of the ways an
// image gets here, and the handlers that put images into (and move them
// around) the row.
export const useImageGenRefs = ({ showNotification, size }: UseImageGenRefsParams) => {
    const { t } = useTranslation();
    const [referenceImages, setReferenceImages] = useState<ReferenceImage[]>([]);
    // The thumbnail being dragged and the one it is hovering over. Order is
    // part of the request — providers read the reference list in order — so it
    // has to be editable in place rather than by removing and re-adding.
    const [draggingReference, setDraggingReference] = useState<number | null>(null);
    const [dragOverReference, setDragOverReference] = useState<number | null>(null);
    const [sketchTarget, setSketchTarget] = useState<SketchTarget>(null);
    // Which reference image's mask editor is open. An index rather than a
    // boolean: a mask belongs to one specific image, and saying which one is
    // the whole point.
    const [maskTarget, setMaskTarget] = useState<number | null>(null);

    const notifyCapReached = useCallback((ignored: number) => showNotification(
        t('playground.referenceCapReached', {
            defaultValue: 'Only {{max}} reference images fit — {{ignored}} were left out',
            max: MAX_EDIT_REFERENCE_IMAGES,
            ignored,
        }),
        'warning',
    ), [showNotification, t]);

    // Appends newly picked/dropped files (image/* only) up to the reference
    // cap, converting each to a data URL up front so thumbnails and the
    // eventual run history render through the same representation.
    const handleAddReferenceImages = useCallback(async (files: FileList | File[]) => {
        const incoming = Array.from(files).filter((file) => file.type.startsWith('image/'));
        if (incoming.length === 0) return;
        const { next: accepted, ignored } = addReferences(
            [] as File[],
            incoming,
            Math.max(0, MAX_EDIT_REFERENCE_IMAGES - referenceImages.length),
            'ignore',
        );
        // Dropping images on the floor without saying so leaves the user
        // believing a request carries pictures it does not.
        if (ignored > 0) notifyCapReached(ignored);
        if (accepted.length === 0) return;
        const withPreviews = await Promise.all(accepted.map(async (file): Promise<ReferenceImage> => {
            const previewUrl = await fileToDataUrl(file);
            return { file, previewUrl, source: 'upload', ...(await readImageSize(previewUrl) ?? {}) };
        }));
        setReferenceImages((current) => addReferences(current, withPreviews, MAX_EDIT_REFERENCE_IMAGES, 'ignore').next);
    }, [notifyCapReached, referenceImages.length]);

    const handleRemoveReferenceImage = useCallback((index: number) => {
        setReferenceImages((current) => current.filter((_, i) => i !== index));
    }, []);

    // Moves one reference to another slot, keeping every other image's relative
    // order — the same result as dragging a card in a list.
    const handleReorderReference = useCallback((from: number, to: number) => {
        setReferenceImages((current) => reorderReferences(current, from, to));
    }, []);

    // The keyboard's version of the same drag: with a thumbnail focused, the
    // arrow keys walk it along the row. Focus follows the image it moved, not
    // the slot it left.
    const handleReferenceKeyDown = useCallback((event: React.KeyboardEvent, index: number) => {
        const delta = event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowRight' ? 1 : 0;
        if (delta === 0) return;
        const target = index + delta;
        if (target < 0 || target >= referenceImages.length) return;
        event.preventDefault();
        event.stopPropagation();
        handleReorderReference(index, target);
        requestAnimationFrame(() => {
            const moved = document.querySelector<HTMLElement>(`[data-reference-index="${target}"]`);
            moved?.focus();
        });
    }, [handleReorderReference, referenceImages.length]);

    const notifyLoadFailed = useCallback(() => showNotification(
        t('playground.referenceLoadFailed', { defaultValue: 'Could not use this image as a reference' }),
        'error',
    ), [showNotification, t]);

    // Hands a completed output straight back in as the next run's reference —
    // the artifact for the next action, not just a notification that one
    // exists.
    //
    // It joins the references rather than replacing them: the row holds up
    // to five, and "use this one too" is the common case. At the cap the
    // oldest makes room — said out loud, because a reference vanishing from
    // the row unannounced is the request quietly changing behind the user's
    // back.
    const handleUseAsReference = useCallback(async (src: string) => {
        try {
            const next = await srcToReference(src, `reference-${Date.now()}.png`);
            if (referenceImages.length >= MAX_EDIT_REFERENCE_IMAGES) {
                showNotification(
                    t('playground.referenceEvicted', {
                        defaultValue: 'Added as a reference — the oldest one made room (max {{max}})',
                        max: MAX_EDIT_REFERENCE_IMAGES,
                    }),
                    'info',
                );
            }
            setReferenceImages((current) => addReferences(current, [next], MAX_EDIT_REFERENCE_IMAGES, 'evict').next);
        } catch {
            notifyLoadFailed();
        }
    }, [notifyLoadFailed, referenceImages.length, showNotification, t]);

    // Several picked images at once (from the library), under their own
    // names. Unlike a single "use as reference", nothing is evicted: the
    // ones that do not fit are left out, and the row says how many.
    const handleAddImageSources = useCallback(async (items: Array<{ src: string; name: string }>) => {
        const { next: accepted, ignored } = addReferences(
            [] as typeof items,
            items,
            Math.max(0, MAX_EDIT_REFERENCE_IMAGES - referenceImages.length),
            'ignore',
        );
        if (ignored > 0) notifyCapReached(ignored);
        try {
            const next = await Promise.all(accepted.map((item) => srcToReference(item.src, item.name)));
            setReferenceImages((current) => addReferences(current, next, MAX_EDIT_REFERENCE_IMAGES, 'ignore').next);
        } catch {
            notifyLoadFailed();
        }
    }, [notifyCapReached, notifyLoadFailed, referenceImages.length]);

    // A sketch is just another way to get a reference image: it lands in the
    // same list, goes through the same request, and shows up in the run
    // history like any upload. Redrawing replaces the sketch in place so it
    // keeps its position among the other references.
    const handleOpenSketch = useCallback((index: number | null) => {
        if (index === null && referenceImages.length >= MAX_EDIT_REFERENCE_IMAGES) return;
        setSketchTarget({ index });
    }, [referenceImages.length]);

    const handleSketchSubmit = useCallback(async (result: SketchResult) => {
        const sketch: ReferenceImage = {
            file: result.file,
            previewUrl: result.previewUrl,
            source: 'sketch',
            layers: result.layers,
            ...(await readImageSize(result.previewUrl) ?? {}),
        };
        setReferenceImages((current) => {
            const index = sketchTarget?.index ?? null;
            if (index !== null && index < current.length) {
                return current.map((ref, i) => (i === index ? sketch : ref));
            }
            return [...current, sketch].slice(0, MAX_EDIT_REFERENCE_IMAGES);
        });
        setSketchTarget(null);
    }, [sketchTarget]);

    // The layers of the sketch being re-opened, or null for a new one.
    // Memoised because the canvas resets whenever this changes, and a fresh
    // object on every render would wipe what the user is drawing.
    const sketchInitial = useMemo<SketchLayers | null>(() => {
        const index = sketchTarget?.index;
        const target = index !== null && index !== undefined ? referenceImages[index] : undefined;
        if (!target) return null;
        if (target.layers) return target.layers;
        // No layers: a sketch flattened by an older build. It still opens, as
        // pixels to keep drawing on, just not as strokes or a posable figure.
        return { size: parseImageSize(size), strokes: [], figures: [], backdrop: target.previewUrl };
    }, [sketchTarget, referenceImages, size]);
    const hasSketchReference = referenceImages.some((ref) => ref.source === 'sketch');

    const handleMaskSubmit = useCallback((result: ReferenceMask) => {
        setReferenceImages((current) => current.map((ref, i) => (i === maskTarget ? { ...ref, mask: result } : ref)));
        setMaskTarget(null);
    }, [maskTarget]);

    // Removing the mask removes a region, not the image: the reference stays
    // and the next run repaints all of it.
    const handleRemoveMask = useCallback((index: number) => {
        setReferenceImages((current) => current.map((ref, i) => (i === index ? { ...ref, mask: undefined } : ref)));
    }, []);

    // Memoised for the same reason the sketch's is: the canvas resets when this
    // changes, and a new object per render would wipe live strokes.
    const maskInitial = useMemo(
        () => (maskTarget !== null ? referenceImages[maskTarget]?.mask?.layers ?? null : null),
        [maskTarget, referenceImages],
    );
    const maskedReference = maskTarget !== null ? referenceImages[maskTarget] : undefined;
    // Only the first image's mask is sent, so only that one changes what the
    // prompt is being asked to describe.
    const hasMaskedReference = referenceImages[0]?.mask !== undefined;

    return {
        referenceImages,
        setReferenceImages,
        draggingReference,
        setDraggingReference,
        dragOverReference,
        setDragOverReference,
        handleAddReferenceImages,
        handleRemoveReferenceImage,
        handleReorderReference,
        handleReferenceKeyDown,
        handleUseAsReference,
        handleAddImageSources,
        sketchTarget,
        setSketchTarget,
        handleOpenSketch,
        handleSketchSubmit,
        sketchInitial,
        hasSketchReference,
        maskTarget,
        setMaskTarget,
        handleMaskSubmit,
        handleRemoveMask,
        maskInitial,
        maskedReference,
        hasMaskedReference,
    };
};
