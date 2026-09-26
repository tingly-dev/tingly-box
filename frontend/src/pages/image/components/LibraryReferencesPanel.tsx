import { useMemo, useRef, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    IconButton,
    InputAdornment,
    Paper,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import ConfirmDialog from '@/components/ConfirmDialog';
import { Delete, Download, Edit, FileUpload, Search } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import { downloadImage } from '@/utils/download';
import {
    deleteLibraryReference,
    matchesQuery,
    renameLibraryReference,
    saveLibraryReferences,
    type LibraryReference,
} from '@/utils/imageLibrary';
import ThumbImage from './ThumbImage';
import { THUMB_EDGE_TILE } from './imageThumbnails';
import { formatBytes } from './imageGenSession';
import { readImageSize } from './useImageGenRefs';
import { playgroundHandoffState } from './libraryHandoff';

const fileToDataUrl = (file: File): Promise<string> => new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
});

interface LibraryReferencesPanelProps {
    references: LibraryReference[];
    loaded: boolean;
}

// Kept reference images — a character sheet, a style board, a product shot —
// that outlive the session they were used in. Images come in the same ways
// they do on the playground (browse, drop, paste) or from the playground's
// image viewer, and go back out as references.
const LibraryReferencesPanel: React.FC<LibraryReferencesPanelProps> = ({ references, loaded }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { notify } = useNotify();
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [query, setQuery] = useState('');
    const [viewing, setViewing] = useState<LibraryReference | null>(null);
    const [renaming, setRenaming] = useState<{ reference: LibraryReference; name: string } | null>(null);
    const [removing, setRemoving] = useState<LibraryReference | null>(null);

    const visible = useMemo(
        () => references.filter((reference) => matchesQuery([reference.name], query)),
        [query, references],
    );

    const handleAddFiles = async (files: FileList | File[]) => {
        const images = Array.from(files).filter((file) => file.type.startsWith('image/'));
        if (images.length === 0) return;
        try {
            const inputs = await Promise.all(images.map(async (file) => {
                const src = await fileToDataUrl(file);
                return { name: file.name, src, bytes: file.size, ...(await readImageSize(src) ?? {}) };
            }));
            if (await saveLibraryReferences(inputs)) {
                notify('success', t('imageLibrary.imagesAdded', { defaultValue: 'Added {{count}} images', count: inputs.length }));
            } else {
                notify('error', t('imageLibrary.saveFailed', { defaultValue: 'Could not save to the library' }));
            }
        } catch {
            notify('error', t('imageLibrary.saveFailed', { defaultValue: 'Could not save to the library' }));
        }
    };

    const handleUse = (reference: LibraryReference) => {
        navigate('/image/playground', { state: playgroundHandoffState({ referenceIds: [reference.id] }) });
    };
    const handleDownload = async (reference: LibraryReference) => {
        try {
            await downloadImage(reference.src, reference.name.replace(/\.[a-z0-9]+$/i, ''));
        } catch {
            notify('error', t('playground.downloadFailed', { defaultValue: 'Could not download this image' }));
        }
    };

    const useLabel = t('imageLibrary.useAsReference', { defaultValue: 'Use as reference' });

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
                if (files.some((file) => file.type.startsWith('image/'))) {
                    event.preventDefault();
                    void handleAddFiles(files);
                }
            }}
        >
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} sx={{ alignItems: { sm: 'center' } }}>
                <TextField
                    size="small"
                    placeholder={t('imageLibrary.searchImages', { defaultValue: 'Search by name' })}
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    sx={{ flex: 1, maxWidth: { sm: 360 } }}
                    slotProps={{
                        input: {
                            startAdornment: (
                                <InputAdornment position="start"><Search fontSize="small" /></InputAdornment>
                            ),
                        },
                    }}
                />
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>
                    {t('imageLibrary.dropHint', { defaultValue: 'Drop or paste images anywhere here' })}
                </Typography>
                <Button variant="contained" startIcon={<FileUpload />} onClick={() => fileInputRef.current?.click()}>
                    {t('imageLibrary.addImages', { defaultValue: 'Add images' })}
                </Button>
            </Stack>

            {loaded && references.length === 0 && (
                <Paper variant="outlined" sx={{ p: 3, textAlign: 'center', borderStyle: 'dashed' }}>
                    <Typography variant="subtitle1" sx={{ mb: 1 }}>
                        {t('imageLibrary.emptyImagesTitle', { defaultValue: 'Keep the images you generate from again and again' })}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 560, mx: 'auto' }}>
                        {t('imageLibrary.emptyImagesBody', {
                            defaultValue: 'Add a character sheet, a style board or a product shot here, or save any image from the Playground’s image viewer. They show up under Library in the Playground’s reference row.',
                        })}
                    </Typography>
                </Paper>
            )}
            {loaded && references.length > 0 && visible.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ py: 3, textAlign: 'center' }}>
                    {t('imageLibrary.noMatches', { defaultValue: 'Nothing matches' })}
                </Typography>
            )}

            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 1.5 }}>
                {visible.map((reference) => (
                    <Paper key={reference.id} variant="outlined" sx={{ overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
                        <ButtonBase
                            onClick={() => setViewing(reference)}
                            aria-label={reference.name}
                            sx={{ display: 'block', aspectRatio: '1 / 1', bgcolor: 'action.hover' }}
                        >
                            <ThumbImage src={reference.src} alt={reference.name} edge={THUMB_EDGE_TILE} />
                        </ButtonBase>
                        <Box sx={{ px: 1, pt: 0.75 }}>
                            <Typography variant="body2" noWrap title={reference.name}>{reference.name}</Typography>
                            <Typography variant="caption" color="text.secondary">
                                {[
                                    reference.width && reference.height ? `${reference.width}×${reference.height}` : '',
                                    formatBytes(reference.bytes),
                                ].filter(Boolean).join(' · ')}
                            </Typography>
                        </Box>
                        <Stack direction="row" spacing={0.25} sx={{ alignItems: 'center', px: 0.5, pb: 0.5 }}>
                            <Button size="small" onClick={() => handleUse(reference)}>{useLabel}</Button>
                            <Box sx={{ flex: 1 }} />
                            <Tooltip title={t('imageLibrary.rename', { defaultValue: 'Rename' })}>
                                <IconButton
                                    size="small"
                                    onClick={() => setRenaming({ reference, name: reference.name })}
                                    aria-label={t('imageLibrary.rename', { defaultValue: 'Rename' })}
                                >
                                    <Edit sx={{ fontSize: 16 }} />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title={t('common.delete', { defaultValue: 'Delete' })}>
                                <IconButton
                                    size="small"
                                    onClick={() => setRemoving(reference)}
                                    aria-label={t('common.delete', { defaultValue: 'Delete' })}
                                >
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
                    <Button startIcon={<Download />} onClick={() => { if (viewing) void handleDownload(viewing); }}>
                        {t('playground.download', { defaultValue: 'Download' })}
                    </Button>
                    <Button variant="contained" onClick={() => { if (viewing) handleUse(viewing); }}>{useLabel}</Button>
                </DialogActions>
            </Dialog>

            <Dialog open={renaming !== null} onClose={() => setRenaming(null)} maxWidth="xs" fullWidth>
                <DialogContent>
                    <TextField
                        autoFocus
                        fullWidth
                        size="small"
                        label={t('imageLibrary.nameLabel', { defaultValue: 'Name' })}
                        value={renaming?.name ?? ''}
                        onChange={(event) => setRenaming((current) => (current ? { ...current, name: event.target.value } : current))}
                        onKeyDown={(event) => {
                            if (event.key === 'Enter' && renaming) {
                                void renameLibraryReference(renaming.reference, renaming.name);
                                setRenaming(null);
                            }
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setRenaming(null)}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                    <Button
                        variant="contained"
                        disabled={!renaming?.name.trim()}
                        onClick={() => {
                            if (renaming) void renameLibraryReference(renaming.reference, renaming.name);
                            setRenaming(null);
                        }}
                    >
                        {t('common.save', { defaultValue: 'Save' })}
                    </Button>
                </DialogActions>
            </Dialog>

            <ConfirmDialog
                open={removing !== null}
                title={t('imageLibrary.deleteImageTitle', { defaultValue: 'Delete {{name}}?', name: removing?.name ?? '' })}
                description={t('imageLibrary.deleteImageBody', { defaultValue: 'Removes it from the library.' })}
                confirmLabel={t('common.delete', { defaultValue: 'Delete' })}
                cancelLabel={t('common.cancel', { defaultValue: 'Cancel' })}
                confirmColor="error"
                onClose={() => setRemoving(null)}
                onConfirm={() => {
                    const target = removing;
                    setRemoving(null);
                    if (target) void deleteLibraryReference(target.id);
                }}
            />
        </Stack>
    );
};

export default LibraryReferencesPanel;
