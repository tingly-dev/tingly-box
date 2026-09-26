import { useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Check } from '@/components/icons';
import type { LibraryReference } from '@/utils/imageLibrary';
import ThumbImage from './ThumbImage';
import { THUMB_EDGE_TILE } from './imageThumbnails';

interface LibraryReferencePickerDialogProps {
    open: boolean;
    references: LibraryReference[];
    // How many more the request row can take — picking stops there instead
    // of accepting images that would then be dropped.
    room: number;
    onClose: () => void;
    onPick: (references: LibraryReference[]) => void;
}

// The reference row's fourth way in: images kept in the library. Several can
// be picked at once — a character sheet plus a style board is the usual
// pair — in the order they are clicked, which is the order they are sent in.
const LibraryReferencePickerDialog: React.FC<LibraryReferencePickerDialogProps> = ({
    open,
    references,
    room,
    onClose,
    onPick,
}) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    // Emptied whenever the dialog is left, so it opens with nothing picked.
    const [picked, setPicked] = useState<string[]>([]);
    const close = () => { setPicked([]); onClose(); };

    const toggle = (id: string) => {
        setPicked((current) => {
            if (current.includes(id)) return current.filter((value) => value !== id);
            if (current.length >= room) return current;
            return [...current, id];
        });
    };
    const confirm = () => {
        const byId = new Map(references.map((reference) => [reference.id, reference]));
        onPick(picked.map((id) => byId.get(id)).filter((reference): reference is LibraryReference => reference !== undefined));
        setPicked([]);
    };

    return (
        <Dialog open={open} onClose={close} maxWidth="md" fullWidth>
            <DialogTitle>
                {t('imageLibrary.pickReferencesTitle', { defaultValue: 'Add reference images from the library' })}
            </DialogTitle>
            <DialogContent dividers>
                {references.length === 0 ? (
                    <Stack spacing={1.5} sx={{ alignItems: 'center', py: 4, textAlign: 'center' }}>
                        <Typography variant="body2" color="text.secondary">
                            {t('imageLibrary.noReferencesPicker', {
                                defaultValue: 'No images kept yet. Save one from the image viewer, or add your own on the library page.',
                            })}
                        </Typography>
                        <Button variant="outlined" size="small" onClick={() => { close(); navigate('/image/library?tab=references'); }}>
                            {t('imageLibrary.openLibrary', { defaultValue: 'Open the library' })}
                        </Button>
                    </Stack>
                ) : (
                    <Box
                        sx={{
                            display: 'grid',
                            gridTemplateColumns: 'repeat(auto-fill, minmax(128px, 1fr))',
                            gap: 1.5,
                        }}
                    >
                        {references.map((reference) => {
                            const order = picked.indexOf(reference.id);
                            const selected = order >= 0;
                            const full = !selected && picked.length >= room;
                            return (
                                <ButtonBase
                                    key={reference.id}
                                    onClick={() => toggle(reference.id)}
                                    disabled={full}
                                    aria-pressed={selected}
                                    aria-label={reference.name}
                                    sx={{
                                        display: 'block',
                                        textAlign: 'left',
                                        borderRadius: 1.5,
                                        overflow: 'hidden',
                                        border: '2px solid',
                                        borderColor: selected ? 'primary.main' : 'divider',
                                        opacity: full ? 0.45 : 1,
                                    }}
                                >
                                    <Box sx={{ position: 'relative', aspectRatio: '1 / 1', bgcolor: 'action.hover' }}>
                                        <ThumbImage src={reference.src} alt={reference.name} edge={THUMB_EDGE_TILE} />
                                        {selected && (
                                            <Box
                                                sx={{
                                                    position: 'absolute',
                                                    top: 6,
                                                    right: 6,
                                                    width: 22,
                                                    height: 22,
                                                    borderRadius: '50%',
                                                    bgcolor: 'primary.main',
                                                    color: 'primary.contrastText',
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    fontSize: 12,
                                                    fontWeight: 600,
                                                }}
                                            >
                                                {picked.length > 1 ? order + 1 : <Check sx={{ fontSize: 14 }} />}
                                            </Box>
                                        )}
                                    </Box>
                                    <Typography variant="caption" noWrap sx={{ display: 'block', px: 1, py: 0.5 }}>
                                        {reference.name}
                                    </Typography>
                                </ButtonBase>
                            );
                        })}
                    </Box>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                {references.length > 0 && (
                    <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>
                        {room === 0
                            ? t('imageLibrary.pickNoRoom', { defaultValue: 'The request already has the maximum number of reference images' })
                            : t('imageLibrary.pickRoom', { defaultValue: '{{picked}} picked · room for {{room}}', picked: picked.length, room })}
                    </Typography>
                )}
                <Button onClick={close}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                <Button variant="contained" disabled={picked.length === 0} onClick={confirm}>
                    {t('imageLibrary.pickConfirm', {
                        defaultValue_one: 'Add {{count}} image',
                        defaultValue_other: 'Add {{count}} images',
                        count: picked.length,
                    })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default LibraryReferencePickerDialog;
