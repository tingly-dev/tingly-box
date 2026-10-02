import { useEffect, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ImageAsset } from './assetTypes';
import { useAssets } from './assetStore';

interface Props {
    open: boolean;
    // How many more references the row can take.
    remaining: number;
    onClose: () => void;
    onPick: (assets: ImageAsset[]) => void;
}

// Picking references from the library, without leaving the page.
const AssetPickerDialog: React.FC<Props> = ({ open, remaining, onClose, onPick }) => {
    const { t } = useTranslation();
    const assets = useAssets();
    const [picked, setPicked] = useState<string[]>([]);
    useEffect(() => { if (open) setPicked([]); }, [open]);

    const toggle = (id: string) => setPicked((current) => {
        if (current.includes(id)) return current.filter((item) => item !== id);
        return current.length < remaining ? [...current, id] : current;
    });

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ fontSize: '1.05rem' }}>
                {t('imageLibrary.pickTitle', { defaultValue: 'Add references from the library' })}
            </DialogTitle>
            <DialogContent dividers>
                <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(120px, 1fr))', gap: 1.5 }}>
                    {assets.map((asset) => {
                        const order = picked.indexOf(asset.id);
                        const selected = order !== -1;
                        return (
                            <ButtonBase
                                key={asset.id}
                                disabled={!selected && picked.length >= remaining}
                                onClick={() => toggle(asset.id)}
                                aria-pressed={selected}
                                sx={{
                                    position: 'relative',
                                    aspectRatio: '1 / 1',
                                    borderRadius: 1.5,
                                    overflow: 'hidden',
                                    bgcolor: 'action.hover',
                                    outline: selected ? '3px solid' : 'none',
                                    outlineColor: 'primary.main',
                                    outlineOffset: -3,
                                    '&.Mui-disabled': { opacity: 0.5 },
                                }}
                            >
                                <Box component="img" src={asset.src} alt={asset.name} sx={{ width: '100%', height: '100%', objectFit: 'contain' }} />
                                {selected && (
                                    <Box sx={{ position: 'absolute', top: 6, right: 6, width: 22, height: 22, borderRadius: '50%', bgcolor: 'primary.main', color: '#fff', fontSize: 12, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                        {order + 1}
                                    </Box>
                                )}
                            </ButtonBase>
                        );
                    })}
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                <Typography sx={{ mr: 'auto', fontSize: 13, color: 'text.secondary' }}>
                    {t('imageLibrary.pickRemaining', { defaultValue: '{{count}} more can be added', count: remaining - picked.length })}
                </Typography>
                <Button onClick={onClose}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                <Button
                    variant="contained"
                    disabled={picked.length === 0}
                    onClick={() => onPick(picked.map((id) => assets.find((asset) => asset.id === id)).filter((asset): asset is ImageAsset => Boolean(asset)))}
                >
                    {t('imageLibrary.pickConfirm', { defaultValue: 'Add {{count}}', count: picked.length })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default AssetPickerDialog;
