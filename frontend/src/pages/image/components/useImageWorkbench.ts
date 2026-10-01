import { useCallback, useEffect, useRef, useState } from 'react';
import { imageArchiveApi, type Workbench } from '@/services/imageArchiveApi';

// The focus workbench: one archived image and the description of its subject,
// plus the images derived from it. The backend archive is its only store —
// nothing here is kept in IndexedDB — so a workbench reads the same from any
// browser that opens this tingly-box. See .design/image-workbench.md.

// Which workbench this browser last had open is a per-viewer convenience, not
// state anyone else needs, so it is the one thing kept locally.
const ACTIVE_KEY = 'imagegen.activeWorkbench';
const readActiveId = (): string | null => {
    try { return localStorage.getItem(ACTIVE_KEY); } catch { return null; }
};
const writeActiveId = (id: string | null) => {
    try {
        if (id) localStorage.setItem(ACTIVE_KEY, id);
        else localStorage.removeItem(ACTIVE_KEY);
    } catch { /* storage unavailable: the focus just isn't remembered */ }
};

const blobToDataUrl = (blob: Blob): Promise<string> => new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
});

// Archived images are immutable per id, so one data URL per id serves every
// thumbnail and reference for the life of the page.
const imageSrcCache = new Map<string, Promise<string>>();
export const archivedImageSrc = (id: string): Promise<string> => {
    let cached = imageSrcCache.get(id);
    if (!cached) {
        cached = imageArchiveApi.fetchImageBlob(id).then(blobToDataUrl);
        cached.catch(() => imageSrcCache.delete(id));
        imageSrcCache.set(id, cached);
    }
    return cached;
};

// Every image the workbench holds, root first, then in the order derived.
export const workbenchImageIds = (wb: Workbench): string[] => [wb.root_image_id, ...(wb.items ?? []).map((it) => it.image_id)];

type Notify = (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;

export const useImageWorkbench = (notify: Notify) => {
    const [workbenches, setWorkbenches] = useState<Workbench[]>([]);
    const [activeId, setActiveId] = useState<string | null>(readActiveId);
    const [srcs, setSrcs] = useState<Record<string, string>>({});
    const activeIdRef = useRef(activeId);
    useEffect(() => { activeIdRef.current = activeId; }, [activeId]);

    const active = workbenches.find((wb) => wb.id === activeId) ?? null;

    const reload = useCallback(async () => {
        const result = await imageArchiveApi.listWorkbenches();
        if (!result?.success) return;
        const list: Workbench[] = result.workbenches ?? [];
        setWorkbenches(list);
        // A remembered focus whose workbench is gone (deleted elsewhere) is
        // dropped rather than leaving the panel focused on nothing.
        if (activeIdRef.current && !list.some((wb) => wb.id === activeIdRef.current)) {
            setActiveId(null);
            writeActiveId(null);
        }
    }, []);

    useEffect(() => { void reload(); }, [reload]);

    // Fetch the pixels of whatever the active workbench shows.
    useEffect(() => {
        if (!active) return undefined;
        let cancelled = false;
        workbenchImageIds(active).forEach((id) => {
            if (srcs[id]) return;
            archivedImageSrc(id)
                .then((src) => { if (!cancelled) setSrcs((current) => ({ ...current, [id]: src })); })
                .catch(() => { /* a missing file shows as an empty tile */ });
        });
        return () => { cancelled = true; };
    }, [active, srcs]);

    const replace = useCallback((wb: Workbench) => {
        setWorkbenches((current) => [wb, ...current.filter((other) => other.id !== wb.id)]);
    }, []);

    const focus = useCallback((id: string | null) => {
        setActiveId(id);
        writeActiveId(id);
    }, []);

    // Starts a workbench on an image. An image that is not archived yet (an
    // import, a reference, a URL-only result) is archived first, so the
    // workbench never points at something only this browser has.
    const create = useCallback(async (input: { name: string; description: string; imageId?: string; src?: string }) => {
        let imageId = input.imageId;
        if (!imageId && input.src) {
            const blob = await fetch(input.src).then((r) => r.blob());
            const imported = await imageArchiveApi.importImage(await blobToDataUrl(blob), input.name);
            if (!imported?.success) {
                notify(imported?.error || 'Could not save this image', 'error');
                return null;
            }
            imageId = imported.image.id as string;
        }
        if (!imageId) return null;
        const result = await imageArchiveApi.createWorkbench({
            name: input.name,
            description: input.description,
            root_image_id: imageId,
        });
        if (!result?.success) {
            notify(result?.error || 'Could not create the workbench', 'error');
            return null;
        }
        replace(result.workbench);
        focus(result.workbench.id);
        return result.workbench as Workbench;
    }, [focus, notify, replace]);

    const update = useCallback(async (id: string, body: { name?: string; description?: string }) => {
        const result = await imageArchiveApi.updateWorkbench(id, body);
        if (!result?.success) {
            notify(result?.error || 'Could not save the workbench', 'error');
            return;
        }
        replace(result.workbench);
    }, [notify, replace]);

    const remove = useCallback(async (id: string) => {
        const result = await imageArchiveApi.deleteWorkbench(id);
        if (!result?.success) {
            notify(result?.error || 'Could not delete the workbench', 'error');
            return;
        }
        setWorkbenches((current) => current.filter((wb) => wb.id !== id));
        if (activeIdRef.current === id) focus(null);
    }, [focus, notify]);

    const addItems = useCallback(async (id: string, items: { image_id: string; parent_id: string }[]) => {
        if (items.length === 0) return;
        const result = await imageArchiveApi.addWorkbenchItems(id, items);
        if (!result?.success) {
            notify(result?.error || 'Could not add the images to the workbench', 'error');
            return;
        }
        replace(result.workbench);
    }, [notify, replace]);

    const removeItem = useCallback(async (id: string, imageId: string) => {
        const result = await imageArchiveApi.removeWorkbenchItem(id, imageId);
        if (!result?.success) {
            notify(result?.error || 'Could not remove the image', 'error');
            return;
        }
        replace(result.workbench);
    }, [notify, replace]);

    return { workbenches, active, srcs, focus, create, update, remove, addItems, removeItem, reload };
};
