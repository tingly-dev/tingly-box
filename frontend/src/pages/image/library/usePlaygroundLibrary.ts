import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { fetchBlob } from '@tingly/vision';
import { readImageSize } from '../components/imageFiles';
import { readHandoff } from './handoff';
import { appendPiece, type LibraryImage } from './model';
import { addImages, listImages, savePieces } from './store';
import { useLibrary } from './useLibrary';

type Notify = (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;

interface UsePlaygroundLibraryParams {
    setPrompt: (update: (current: string) => string) => void;
    // Puts images into the request's reference row.
    addReferences: (images: Array<{ src: string; name: string }>) => Promise<void>;
    notify: Notify;
}

// Everything the playground does with the library, in one place: saving the
// prompt or an image into it, and receiving what the library page hands over.
// The playground saves and loads without leaving its page; the library page
// is where things are browsed and tidied. See .design/image-library.md.
export const usePlaygroundLibrary = ({ setPrompt, addReferences, notify }: UsePlaygroundLibraryParams) => {
    const { t } = useTranslation();
    const { pieces, images } = useLibrary();
    const [pickerOpen, setPickerOpen] = useState(false);
    const saveFailed = t('imageLibrary.saveFailed', { defaultValue: 'Could not save to the library' });

    const savePrompt = useCallback(async (text: string) => {
        const saved = await savePieces([{ kind: 'prompt', text }]);
        notify(saved ? t('imageLibrary.promptSaved', { defaultValue: 'Prompt saved to the library' }) : saveFailed, saved ? 'success' : 'error');
    }, [notify, saveFailed, t]);

    // `stem` names the file; the extension comes from the image itself.
    const saveImage = useCallback(async (src: string, stem: string) => {
        if (images.some((image) => image.src === src)) {
            notify(t('imageLibrary.imageAlreadySaved', { defaultValue: 'This image is already in the library' }), 'info');
            return;
        }
        try {
            const blob = await fetchBlob(src);
            const extension = (blob.type.split('/')[1] ?? 'png').replace('jpeg', 'jpg');
            const saved = await addImages([{
                name: `${stem || 'image'}.${extension}`,
                src,
                bytes: blob.size,
                ...(await readImageSize(src) ?? {}),
            }]);
            notify(saved ? t('imageLibrary.imageSaved', { defaultValue: 'Image saved to the library' }) : saveFailed, saved ? 'success' : 'error');
        } catch {
            notify(saveFailed, 'error');
        }
    }, [images, notify, saveFailed, t]);

    // Arriving from the library page with something to use. The router state
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
            notify(t('imageLibrary.promptLoaded', { defaultValue: 'Prompt loaded from the library' }), 'success');
        }
        if (piece !== undefined) setPrompt((current) => appendPiece(current, piece));
        if (imageIds?.length) {
            void listImages().then((all) => addReferences(imageIds
                .map((id) => all.find((image) => image.id === id))
                .filter((image): image is LibraryImage => image !== undefined)));
        }
    }, [location.key, location.pathname, location.state, navigate, notify, setPrompt, t, addReferences]);

    return { pieces, images, savePrompt, saveImage, pickerOpen, setPickerOpen };
};
