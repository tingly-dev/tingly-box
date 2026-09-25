import { useEffect, useRef, useState } from 'react';
import { fetchBlob } from '@tingly/vision';

// Small copies of the session's images, for every place that shows one at a
// fraction of its size: overview tiles, reference badges, the source strip.
//
// Pointing a 190px tile at the original is what made a long session crawl:
// every tile decodes a full 1024–4096px PNG (tens of MB of bitmap each), all
// held in memory at once, and the decodes land on the main thread as the grid
// scrolls. A thumbnail is decoded once, off a blob URL a few dozen KB long.
//
// Everything here is best-effort: when the browser cannot decode or re-encode
// (no createImageBitmap, a cross-origin URL that will not fetch), the caller
// gets the original back and the image still shows — just not as cheaply.

// Longest edge, in CSS px × a 2× display. The grid's tiles top out near 260px.
export const THUMB_EDGE_TILE = 512;
export const THUMB_EDGE_BADGE = 96;

// Plenty for a long session; past it the least recently asked-for thumbnail
// is dropped (and its blob URL revoked) rather than growing without bound.
const MAX_ENTRIES = 600;
// Decodes are the expensive part and each holds a full-size bitmap while it
// runs: a couple at a time keeps a grid of hundreds from spiking memory.
const MAX_CONCURRENT = 2;

// Keyed per edge, then by the source string itself. The sources are the same
// string instances every render (resultSrc caches them), so the lookup hashes
// each multi-megabyte data URL once, not per render.
const caches = new Map<number, Map<string, Promise<string>>>();
const created = new Set<string>();

let active = 0;
const queue: Array<() => void> = [];
const schedule = <T>(task: () => Promise<T>): Promise<T> => new Promise<T>((resolve, reject) => {
    const run = () => {
        active += 1;
        task().then(resolve, reject).finally(() => {
            active -= 1;
            queue.shift()?.();
        });
    };
    if (active < MAX_CONCURRENT) run();
    else queue.push(run);
});

const canThumbnail = (): boolean => typeof createImageBitmap === 'function'
    && typeof document !== 'undefined'
    && typeof URL.createObjectURL === 'function';

const render = async (src: string, edge: number): Promise<string> => {
    const bitmap = await createImageBitmap(await fetchBlob(src));
    try {
        const scale = edge / Math.max(bitmap.width, bitmap.height);
        // Already small: the original is the thumbnail.
        if (scale >= 1) return src;
        const canvas = document.createElement('canvas');
        canvas.width = Math.max(1, Math.round(bitmap.width * scale));
        canvas.height = Math.max(1, Math.round(bitmap.height * scale));
        const context = canvas.getContext('2d');
        if (!context) return src;
        context.imageSmoothingQuality = 'high';
        context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
        const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/webp', 0.86));
        if (!blob) return src;
        const url = URL.createObjectURL(blob);
        created.add(url);
        return url;
    } finally {
        bitmap.close();
    }
};

/** A downscaled copy of `src` whose longest edge is at most `edge`. */
export const getThumbnail = (src: string, edge: number): Promise<string> => {
    if (!src || !canThumbnail()) return Promise.resolve(src);
    let cache = caches.get(edge);
    if (!cache) {
        cache = new Map();
        caches.set(edge, cache);
    }
    const hit = cache.get(src);
    if (hit) {
        // Refresh its place in the LRU order.
        cache.delete(src);
        cache.set(src, hit);
        return hit;
    }
    const pending = schedule(() => render(src, edge)).catch(() => src);
    cache.set(src, pending);
    while (cache.size > MAX_ENTRIES) {
        const [oldestSrc, oldest] = cache.entries().next().value as [string, Promise<string>];
        cache.delete(oldestSrc);
        void oldest.then((url) => {
            if (created.delete(url)) URL.revokeObjectURL(url);
        });
    }
    return pending;
};

/**
 * The thumbnail for `src`, once the element holding it comes near the
 * viewport. Until then (and while it is being made) it is `''`: the caller
 * shows its placeholder and never touches the original. Returns the ref to
 * attach to that element.
 */
export const useLazyThumbnail = <E extends Element>(src: string, edge: number) => {
    const ref = useRef<E>(null);
    const [near, setNear] = useState(false);
    const [thumb, setThumb] = useState<{ src: string; url: string } | null>(null);

    useEffect(() => {
        if (near) return undefined;
        const element = ref.current;
        if (!element || typeof IntersectionObserver === 'undefined') {
            setNear(true);
            return undefined;
        }
        const observer = new IntersectionObserver((entries) => {
            if (entries.some((entry) => entry.isIntersecting)) {
                setNear(true);
                observer.disconnect();
            }
        }, { rootMargin: '600px 0px' });
        observer.observe(element);
        return () => observer.disconnect();
    }, [near]);

    useEffect(() => {
        if (!near || !src) return undefined;
        let cancelled = false;
        void getThumbnail(src, edge).then((url) => {
            if (!cancelled) setThumb({ src, url });
        });
        return () => { cancelled = true; };
    }, [edge, near, src]);

    return { ref, url: thumb?.src === src ? thumb.url : '' };
};
