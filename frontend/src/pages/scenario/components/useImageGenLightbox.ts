import { useCallback, useMemo, useState } from 'react';
import { resultSrc, runImage } from './imageGenSession';
import type { GenerationRun, SelectedImage } from './ImageGenPlayground.types';

// One frame of the lightbox filmstrip: a run's originals and its outputs in
// arrival order.
export interface LightboxFrame {
    src: string;
    kind: 'source' | 'output';
    index: number;
}

// The lightbox's own slice of state: which image is open, the run behind it
// and every image that run touched — what went in, then what came out.
// Without this an output on screen says nothing about what it was made from —
// the lightbox is exactly where "what did I reference here?" gets asked, and
// closing it to go read the card is not an answer.
export const useImageGenLightbox = (runs: GenerationRun[]) => {
    const [selectedImage, setSelectedImage] = useState<SelectedImage | null>(null);

    // The run behind the image currently in the lightbox.
    const lightboxRun = useMemo(
        () => (selectedImage?.runId ? runs.find((run) => run.id === selectedImage.runId) : undefined),
        [runs, selectedImage?.runId],
    );
    const lightboxFilm = useMemo<LightboxFrame[]>(() => {
        if (!lightboxRun) return [];
        const sources = (lightboxRun.sourceImages ?? []).map((src, index) => ({ src, kind: 'source' as const, index }));
        const outputs = lightboxRun.images
            .map((image, index) => ({ src: resultSrc(image), kind: 'output' as const, index }))
            .filter((item) => item.src);
        // One image with nothing to compare it to is not a filmstrip; several
        // outputs of one request are — picking between them is why n > 1.
        return sources.length > 0 || outputs.length > 1 ? [...sources, ...outputs] : [];
    }, [lightboxRun]);

    const showLightboxFrame = useCallback((frame: LightboxFrame) => {
        if (lightboxRun) setSelectedImage(runImage(lightboxRun, frame.kind, frame.index, frame.src));
    }, [lightboxRun]);

    // ←/→ walk the filmstrip, the same gesture the reference row uses. Without
    // it, comparing an output against its original is a mouse-only move.
    const handleLightboxKeyDown = useCallback((event: React.KeyboardEvent) => {
        const delta = event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowRight' ? 1 : 0;
        if (delta === 0 || lightboxFilm.length < 2 || !selectedImage) return;
        const current = lightboxFilm.findIndex(
            (frame) => frame.kind === selectedImage.kind && frame.index === selectedImage.index,
        );
        if (current === -1) return;
        event.preventDefault();
        // Wraps, so the strip has no dead end at either edge.
        showLightboxFrame(lightboxFilm[(current + delta + lightboxFilm.length) % lightboxFilm.length]);
    }, [lightboxFilm, selectedImage, showLightboxFrame]);

    return {
        selectedImage,
        setSelectedImage,
        lightboxRun,
        lightboxFilm,
        showLightboxFrame,
        handleLightboxKeyDown,
    };
};
