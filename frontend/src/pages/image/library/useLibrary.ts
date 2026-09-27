import { useEffect, useState } from 'react';
import type { LibraryImage, PromptPiece } from './model';
import { listImages, listPieces, subscribeLibrary } from './store';

// The library as React state: read once, re-read after any write from any
// view. `loaded` tells "nothing kept yet" from "not read yet", so an empty
// state never flashes before the lists arrive.
export const useLibrary = () => {
    const [state, setState] = useState<{ pieces: PromptPiece[]; images: LibraryImage[]; loaded: boolean }>({
        pieces: [],
        images: [],
        loaded: false,
    });

    useEffect(() => {
        let active = true;
        const refresh = async () => {
            const [pieces, images] = await Promise.all([listPieces(), listImages()]);
            if (active) setState({ pieces, images, loaded: true });
        };
        void refresh();
        const unsubscribe = subscribeLibrary(() => { void refresh(); });
        return () => {
            active = false;
            unsubscribe();
        };
    }, []);

    return state;
};
