import { useRef, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    IconButton,
    Paper,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import ConfirmDialog from '@/components/ConfirmDialog';
import { Delete, Download, Edit, FileUpload } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import { downloadImage } from '@/utils/download';
import ThumbImage from '../components/ThumbImage';
import { THUMB_EDGE_TILE } from '../components/imageThumbnails';
import { formatBytes } from '../components/imageGenSession';
import { fileToDataUrl } from '../components/imageFiles';
import { handoffState } from './handoff';
import { EmptyState, NoMatches, SearchField } from './AssetsChrome';
import { matchesQuery, type AssetImage } from './model';
import { addImages, deleteImage, renameImage } from './store';

// Kept reference images — a character sheet, a style board, a product shot —
// that outlive the session they were used in. They come in the way images
// come into the playground (browse, drop, paste) or from its image viewer,
// and go back out as references.
const ImagesPanel: React.FC<{ images: AssetImage[]; loaded: boolean }> = ({ images, loaded }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { notify } = useNotify();
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [query, setQuery] = useState('');
    const [viewing, setViewing] = useState<AssetImage | null>(null);
    const [renaming, setRenaming] = useState<{ image: AssetImage; name: string } | null>(null);
    const [removing, setRemoving] = useState<AssetImage | null>(null);
    const visible = images.filter((image) => matchesQuery([image.name], query));

    const handleAddFiles = async (files: FileList | File[]) => {
        const picked = Array.from(files).filter((file) => file.type.startsWith('image/'));
        if (picked.length === 0) return;
        let saved = null;
        try {
            saved = await addImages(await Promise.all(picked.map(async (file) => ({ name: file.name, src: await fileToDataUrl(file) }))));
        } catch {
            // Reported below, same as a failed write.
        }
        if (saved) notify('success', t('imageAssets.imagesAdded', { defaultValue: 'Added {{count}} images', count: saved.filter((item) => !item.existing).length }));
        else notify('error', t('imageAssets.saveFailed', { defaultValue: 'Could not save to Assets' }));
    };
    const use = (image: AssetImage) => navigate('/image/playground', { state: handoffState({ imageIds: [image.id] }) });
    const download = (image: AssetImage) => {
        downloadImage(image.src, image.name.replace(/\.[a-z0-9]+$/i, '')).catch(() => {
            notify('error', t('playground.downloadFailed', { defaultValue: 'Could not download this image' }));
        });
    };
    const commitRename = () => {
        if (renaming?.name.trim()) void renameImage(renaming.image, renaming.name);
        setRenaming(null);
    };

    const useLabel = t('imageAssets.useAsReference', { defaultValue: 'Use as reference' });
    const renameLabel = t('imageAssets.rename', { defaultValue: 'Rename' });
    const deleteLabel = t('common.delete', { defaultValue: 'Delete' });

    return (
        <Stack
            spacing={2}
            onDragOver={(event) => event.preventDefault()}
            onDrop={(event) => {
                event.preventDefault();
                if (event.dataTransfer.files?.length) void handleAddFiles(event.dataTransfer.files);
            }}
            onPaste={(event) => {
                const files = Array.from(event.clipboardData?.files ?? []);
                if (!files.some((file) => file.type.startsWith('image/'))) return;
                event.preventDefault();
                void handleAddFiles(files);
            }}
        >
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} sx={{ alignItems: { sm: 'center' } }}>
                <SearchField
                    value={query}
                    onChange={setQuery}
                    placeholder={t('imageAssets.searchImages', { defaultValue: 'Search by name' })}
                />
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>
                    {t('imageAssets.dropHint', { defaultValue: 'Drop or paste images anywhere here' })}
                </Typography>
                <Button variant="contained" startIcon={<FileUpload />} onClick={() => fileInputRef.current?.click()}>
                    {t('imageAssets.addImages', { defaultValue: 'Add images' })}
                </Button>
            </Stack>

            {loaded && images.length === 0 && (
                <EmptyState
                    title={t('imageAssets.emptyImagesTitle', { defaultValue: 'Keep the images you generate from again and again' })}
                    body={t('imageAssets.emptyImagesBody', {
                        defaultValue: 'Add a character sheet, a style board or a product shot here, or save any image from the Playground’s image viewer. They show up under Assets in the Playground’s reference row.',
                    })}
                />
            )}
            {loaded && images.length > 0 && visible.length === 0 && <NoMatches />}

            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 1.5 }}>
                {visible.map((image) => (
                    <Paper key={image.id} variant="outlined" sx={{ overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
                        <ButtonBase
                            onClick={() => setViewing(image)}
                            aria-label={image.name}
                            sx={{ display: 'block', aspectRatio: '1 / 1', bgcolor: 'action.hover' }}
                        >
                            <ThumbImage src={image.src} alt={image.name} edge={THUMB_EDGE_TILE} />
                        </ButtonBase>
                        <Box sx={{ px: 1, pt: 0.75 }}>
                            <Typography variant="body2" noWrap title={image.name}>{image.name}</Typography>
                            <Typography variant="caption" color="text.secondary">
                                {[image.width && image.height ? `${image.width}×${image.height}` : '', formatBytes(image.bytes)]
                                    .filter(Boolean)
                                    .join(' · ')}
                            </Typography>
                        </Box>
                        <Stack direction="row" spacing={0.25} sx={{ alignItems: 'center', px: 0.5, pb: 0.5 }}>
                            <Button size="small" onClick={() => use(image)}>{useLabel}</Button>
                            <Box sx={{ flex: 1 }} />
                            <Tooltip title={renameLabel}>
                                <IconButton size="small" onClick={() => setRenaming({ image, name: image.name })} aria-label={renameLabel}>
                                    <Edit sx={{ fontSize: 16 }} />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title={deleteLabel}>
                                <IconButton size="small" onClick={() => setRemoving(image)} aria-label={deleteLabel}>
                                    <Delete sx={{ fontSize: 16 }} />
                                </IconButton>
                            </Tooltip>
                        </Stack>
                    </Paper>
                ))}
            </Box>

            <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                multiple
                hidden
                onChange={(event) => {
                    if (event.target.files?.length) void handleAddFiles(event.target.files);
                    event.target.value = '';
                }}
            />

            <Dialog open={viewing !== null} onClose={() => setViewing(null)} maxWidth="lg">
                <DialogContent sx={{ p: 0, bgcolor: 'common.black', display: 'flex', justifyContent: 'center' }}>
                    {viewing && (
                        <Box
                            component="img"
                            src={viewing.src}
                            alt={viewing.name}
                            sx={{ maxWidth: '100%', maxHeight: '75vh', objectFit: 'contain', display: 'block' }}
                        />
                    )}
                </DialogContent>
                <DialogActions sx={{ px: 2, py: 1 }}>
                    <Typography variant="body2" noWrap sx={{ flex: 1, minWidth: 0 }}>{viewing?.name}</Typography>
                    <Button startIcon={<Download />} onClick={() => { if (viewing) download(viewing); }}>
                        {t('playground.download', { defaultValue: 'Download' })}
                    </Button>
                    <Button variant="contained" onClick={() => { if (viewing) use(viewing); }}>{useLabel}</Button>
                </DialogActions>
            </Dialog>

            <Dialog open={renaming !== null} onClose={() => setRenaming(null)} maxWidth="xs" fullWidth>
                <DialogContent>
                    <TextField
                        autoFocus
                        fullWidth
                        size="small"
                        label={t('imageAssets.nameLabel', { defaultValue: 'Name' })}
                        value={renaming?.name ?? ''}
                        onChange={(event) => setRenaming((current) => (current ? { ...current, name: event.target.value } : current))}
                        onKeyDown={(event) => {
                            if (event.key !== 'Enter') return;
                            // Otherwise the same keystroke goes on to "click" the
                            // Rename button focus returns to, reopening the dialog.
                            event.preventDefault();
                            commitRename();
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setRenaming(null)}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                    <Button variant="contained" disabled={!renaming?.name.trim()} onClick={commitRename}>
                        {t('common.save', { defaultValue: 'Save' })}
                    </Button>
                </DialogActions>
            </Dialog>

            <ConfirmDialog
                open={removing !== null}
                title={t('imageAssets.deleteImageTitle', { defaultValue: 'Delete {{name}}?', name: removing?.name ?? '' })}
                description={t('imageAssets.deleteImageBody', { defaultValue: 'Removes it from Assets.' })}
                confirmLabel={deleteLabel}
                cancelLabel={t('common.cancel', { defaultValue: 'Cancel' })}
                confirmColor="error"
                onClose={() => setRemoving(null)}
                onConfirm={() => {
                    if (removing) void deleteImage(removing.id);
                    setRemoving(null);
                }}
            />
        </Stack>
    );
};

export default ImagesPanel;
