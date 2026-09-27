import { useEffect, useState } from 'react';
import type { AssetImage, PromptPiece } from './model';
import { listImages, listPieces, subscribeAssets } from './store';

// Image assets as React state: read once, re-read after any write from any
// view. `loaded` tells "nothing kept yet" from "not read yet", so an empty
// state never flashes before the lists arrive.
export const useImageAssets = () => {
    const [state, setState] = useState<{ pieces: PromptPiece[]; images: AssetImage[]; loaded: boolean }>({
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
        const unsubscribe = subscribeAssets(() => { void refresh(); });
        return () => {
            active = false;
            unsubscribe();
        };
    }, []);

    return state;
};
