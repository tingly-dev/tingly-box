import {
    Box,
    ButtonBase,
    Card,
    CardContent,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, Edit, ZoomIn } from '@/components/icons';
import { overlayActionSx, zoomScrimSx } from './ImageGenPlayground.chrome';
import { formatBytes } from './imageGenSession';
import type { ImportedImage } from './ImageGenPlayground.types';

interface ImportedImageCardProps {
    item: ImportedImage;
    onOpen: () => void;
    onUseAsReference: () => void;
    onRemove: () => void;
}

// An imported image sits in the results strip alongside the runs: same card
// chrome, same zoom-to-work-on-it gesture. Its subtitle says what it is —
// there is no prompt or model to report, only the file itself.
const ImportedImageCard: React.FC<ImportedImageCardProps> = ({ item, onOpen, onUseAsReference, onRemove }) => {
    const { t } = useTranslation();
    const dimensions = item.width && item.height ? `${item.width}×${item.height} px` : '';
    return (
        <Card
            data-testid="imagegen-imported-image"
            variant="outlined"
            sx={{
                flex: { xs: '0 0 min(82vw, 320px)', md: '0 0 clamp(280px, 46%, 360px)' },
                height: '100%',
                bgcolor: 'background.paper',
                scrollSnapAlign: 'start',
            }}
        >
            <CardContent sx={{ p: 1.5, height: '100%', '&:last-child': { pb: 1.5 } }}>
                <Stack spacing={1.25} sx={{ height: '100%' }}>
                    <Box sx={{ minWidth: 0 }}>
                        <Typography
                            variant="body2"
                            sx={{ fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        >
                            {item.name}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{ display: 'block', color: 'text.secondary', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        >
                            {[t('playground.importedBadge', { defaultValue: 'Imported' }), dimensions, formatBytes(item.bytes)]
                                .filter(Boolean)
                                .join(' · ')}
                        </Typography>
                    </Box>
                    <Box sx={{ position: 'relative', flex: 1, minHeight: 0, borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}>
                        <ButtonBase
                            onClick={onOpen}
                            aria-label={t('playground.openImported', { defaultValue: 'Open {{name}}', name: item.name })}
                            sx={{
                                width: '100%',
                                height: '100%',
                                display: 'block',
                                '&:hover .image-preview-overlay, &:focus-visible .image-preview-overlay': { opacity: 1 },
                            }}
                        >
                            <Box
                                component="img"
                                src={item.src}
                                alt={item.name}
                                sx={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block' }}
                            />
                            <Box className="image-preview-overlay" sx={zoomScrimSx}>
                                <ZoomIn sx={{ fontSize: 30 }} />
                            </Box>
                        </ButtonBase>
                        <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                            <IconButton
                                size="small"
                                onClick={onUseAsReference}
                                aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                sx={{ position: 'absolute', bottom: 8, right: 8, ...overlayActionSx() }}
                            >
                                <Edit fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        <IconButton
                            size="small"
                            onClick={onRemove}
                            aria-label={t('playground.removeImported', { defaultValue: 'Remove {{name}}', name: item.name })}
                            sx={{ position: 'absolute', top: 8, right: 8, ...overlayActionSx() }}
                        >
                            <Close fontSize="small" />
                        </IconButton>
                    </Box>
                </Stack>
            </CardContent>
        </Card>
    );
};

export default ImportedImageCard;
