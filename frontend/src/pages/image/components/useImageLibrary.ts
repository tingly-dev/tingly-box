import { useCallback, useEffect, useState } from 'react';
import {
    loadLibraryPrompts,
    loadLibraryReferences,
    subscribeImageLibrary,
    type LibraryPrompt,
    type LibraryReference,
} from '@/utils/imageLibrary';

// The library as React state: loaded once, reloaded whenever any view writes
// to it. `loaded` separates "nothing kept yet" from "not read yet", so an
// empty state never flashes before the list arrives.
export const useImageLibrary = () => {
    const [prompts, setPrompts] = useState<LibraryPrompt[]>([]);
    const [references, setReferences] = useState<LibraryReference[]>([]);
    const [loaded, setLoaded] = useState(false);

    const reload = useCallback(async () => {
        const [nextPrompts, nextReferences] = await Promise.all([loadLibraryPrompts(), loadLibraryReferences()]);
        setPrompts(nextPrompts);
        setReferences(nextReferences);
        setLoaded(true);
    }, []);

    useEffect(() => {
        let active = true;
        const refresh = () => { if (active) void reload(); };
        refresh();
        const unsubscribe = subscribeImageLibrary(refresh);
        return () => {
            active = false;
            unsubscribe();
        };
    }, [reload]);

    return { prompts, references, loaded };
};
