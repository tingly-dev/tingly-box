import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { fetchBlob } from '@tingly/vision';
import { fileToDataUrl } from '../components/imageFiles';
import { readHandoff } from './handoff';
import { appendPiece, type AssetImage } from './model';
import { addImages, listImages, savePieces } from './store';
import { useImageAssets } from './useImageAssets';

type Notify = (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;

interface UsePlaygroundAssetsParams {
    setPrompt: (update: (current: string) => string) => void;
    // Puts images into the request's reference row.
    addReferences: (images: Array<{ src: string; name: string }>) => Promise<void>;
    notify: Notify;
}

// Everything the playground does with its assets, in one place: saving the
// prompt or an image into it, and receiving what the Assets page hands over.
// The playground saves and loads without leaving its page; the Assets page
// is where things are browsed and tidied. See .design/image-assets.md.
export const usePlaygroundAssets = ({ setPrompt, addReferences, notify }: UsePlaygroundAssetsParams) => {
    const { t } = useTranslation();
    const { pieces, images } = useImageAssets();
    const [pickerOpen, setPickerOpen] = useState(false);
    const saveFailed = t('imageAssets.saveFailed', { defaultValue: 'Could not save to Assets' });

    const savePrompt = useCallback(async (text: string) => {
        const saved = await savePieces([{ kind: 'prompt', text }]);
        notify(saved ? t('imageAssets.promptSaved', { defaultValue: 'Prompt saved to Assets' }) : saveFailed, saved ? 'success' : 'error');
    }, [notify, saveFailed, t]);

    // `stem` names the file; the extension comes from the image itself. The
    // same image twice is kept once — the server says when it already was.
    const saveImage = useCallback(async (src: string, stem: string) => {
        try {
            const blob = await fetchBlob(src);
            const extension = (blob.type.split('/')[1] ?? 'png').replace('jpeg', 'jpg');
            const saved = await addImages([{
                name: `${stem || 'image'}.${extension}`,
                src: src.startsWith('data:') ? src : await fileToDataUrl(blob),
            }]);
            if (!saved) notify(saveFailed, 'error');
            else if (saved[0]?.existing) notify(t('imageAssets.imageAlreadySaved', { defaultValue: 'This image is already in Assets' }), 'info');
            else notify(t('imageAssets.imageSaved', { defaultValue: 'Image saved to Assets' }), 'success');
        } catch {
            notify(saveFailed, 'error');
        }
    }, [notify, saveFailed, t]);

    // Arriving from the Assets page with something to use. The router state
    // is consumed once and cleared, so a reload or Back does not apply it
    // again.
    const location = useLocation();
    const navigate = useNavigate();
    const handledRef = useRef<string | null>(null);
    useEffect(() => {
        const handoff = readHandoff(location.state);
        if (!handoff || handledRef.current === location.key) return;
        handledRef.current = location.key;
        navigate(location.pathname, { replace: true, state: null });
        const { prompt, piece, imageIds } = handoff;
        if (prompt !== undefined) {
            setPrompt(() => prompt);
            notify(t('imageAssets.promptLoaded', { defaultValue: 'Prompt loaded from Assets' }), 'success');
        }
        if (piece !== undefined) setPrompt((current) => appendPiece(current, piece));
        if (imageIds?.length) {
            void listImages().then((all) => addReferences(imageIds
                .map((id) => all.find((image) => image.id === id))
                .filter((image): image is AssetImage => image !== undefined)));
        }
    }, [location.key, location.pathname, location.state, navigate, notify, setPrompt, t, addReferences]);

    return { pieces, images, savePrompt, saveImage, pickerOpen, setPickerOpen };
};
