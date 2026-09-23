import {
    Box,
    ButtonBase,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, ContentCopy, Create, Download, Edit, GridView, RestartAlt } from '@/components/icons';
import { fullBleedDialogPaperSx, overlayPlateSx } from './ImageGenPlayground.chrome';
import type { GenerationRun, SelectedImage } from './ImageGenPlayground.types';
import type { LightboxFrame } from './useImageGenLightbox';
import type { ReferenceImage } from './ImageGenReferenceImages';

// Shared by the lightbox's overlay buttons — restyling the bar should be one edit.
const overlayIconSx = {
    color: 'common.white',
    bgcolor: 'rgba(255, 255, 255, 0.08)',
    '&:hover': { bgcolor: 'rgba(255, 255, 255, 0.16)' },
} as const;

interface ImageGenLightboxProps {
    selectedImage: SelectedImage | null;
    onClose: () => void;
    onKeyDown: (event: React.KeyboardEvent) => void;
    // The run the open image belongs to, if any — it enables the "put this
    // request back in the panel" action and the filmstrip.
    lightboxRun?: GenerationRun;
    lightboxFilm: LightboxFrame[];
    onShowFrame: (frame: LightboxFrame) => void;
    promptCopied: boolean;
    onCopyPrompt: (text: string) => void;
    reuseLabel: string;
    onReuseRun: (run: GenerationRun) => void;
    onSlice: (image: SelectedImage) => void;
    onDownload: (image: SelectedImage) => void;
    // The request's reference images, so a `reference` image can tell whether
    // it is a re-openable sketch.
    referenceImages: ReferenceImage[];
    onEditSketch: (index: number | null) => void;
    onUseAsReference: (src: string) => void;
}

// The full-bleed image viewer shared by every image on the panel — outputs,
// a run's originals, references, imports. One timeline of actions (copy the
// prompt, reuse the request, slice, download) over whichever picture is open.
const ImageGenLightbox: React.FC<ImageGenLightboxProps> = ({
    selectedImage,
    onClose,
    onKeyDown,
    lightboxRun,
    lightboxFilm,
    onShowFrame,
    promptCopied,
    onCopyPrompt,
    reuseLabel,
    onReuseRun,
    onSlice,
    onDownload,
    referenceImages,
    onEditSketch,
    onUseAsReference,
}) => {
    const { t } = useTranslation();
    return (
        <Dialog
            open={selectedImage !== null}
            onClose={onClose}
            onKeyDown={onKeyDown}
            maxWidth={false}
            fullWidth
            slotProps={{
                paper: {
                    sx: {
                        ...fullBleedDialogPaperSx,
                        bgcolor: 'grey.900',
                        color: 'common.white',
                        overflow: 'hidden',
                    },
                }
            }}
        >
            <DialogTitle
                sx={{
                    py: 1.5,
                    px: 2,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 2,
                    bgcolor: 'rgba(15, 23, 42, 0.96)',
                    color: 'common.white',
                }}
            >
                <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 0.75 }}>
                        {selectedImage?.kind === 'source' && (
                            <Typography
                                component="span"
                                variant="caption"
                                sx={{
                                    flexShrink: 0,
                                    px: 0.75,
                                    borderRadius: 1,
                                    bgcolor: 'rgba(255, 255, 255, 0.14)',
                                    color: 'grey.200',
                                    fontWeight: 600,
                                }}
                            >
                                {t('playground.originalBadge', { defaultValue: 'Original' })}
                            </Typography>
                        )}
                        <Typography
                            component="span"
                            variant="subtitle1"
                            sx={{ fontWeight: 600, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        >
                            {selectedImage?.label ?? selectedImage?.prompt}
                        </Typography>
                    </Box>
                    <Typography
                        component="span"
                        variant="caption"
                        sx={{
                            display: 'block',
                            color: 'grey.400',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                        }}
                    >
                        {selectedImage?.label
                            ? selectedImage.caption
                            : `${selectedImage?.model} · ${selectedImage?.size} · ${selectedImage?.quality}`}
                    </Typography>
                </Box>
                <Stack direction="row" spacing={0.75} sx={{ flexShrink: 0 }}>
                    {!selectedImage?.label && (
                        <Tooltip
                            title={promptCopied
                                ? t('playground.promptCopied', { defaultValue: 'Copied' })
                                : t('playground.copyPrompt', { defaultValue: 'Copy prompt' })}
                            open={promptCopied || undefined}
                            disableHoverListener={promptCopied}
                        >
                            <IconButton
                                onClick={() => { if (selectedImage) onCopyPrompt(selectedImage.prompt); }}
                                aria-label={t('playground.copyPrompt', { defaultValue: 'Copy prompt' })}
                                sx={overlayIconSx}
                            >
                                <ContentCopy fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    )}
                    {/* Zoomed in on a result is exactly where "now change one
                        word and run it again" happens — the request that made
                        this image goes back into the panel from here too. */}
                    {lightboxRun && (
                        <Tooltip title={reuseLabel}>
                            <IconButton
                                onClick={() => onReuseRun(lightboxRun)}
                                aria-label={reuseLabel}
                                sx={overlayIconSx}
                            >
                                <RestartAlt fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    )}
                    <Tooltip title={t('playground.slice.action', { defaultValue: 'Split into tiles' })}>
                        <IconButton
                            onClick={() => { if (selectedImage) onSlice(selectedImage); }}
                            aria-label={t('playground.slice.action', { defaultValue: 'Split into tiles' })}
                            sx={overlayIconSx}
                        >
                            <GridView fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title={t('playground.download', { defaultValue: 'Download' })}>
                        <IconButton
                            onClick={() => { if (selectedImage) onDownload(selectedImage); }}
                            aria-label={t('playground.download', { defaultValue: 'Download' })}
                            sx={overlayIconSx}
                        >
                            <Download fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    {/* An image that is already a reference has nowhere to
                        be sent — the one edit it still affords is redrawing
                        it, and only if it came from the sketch canvas. */}
                    {selectedImage?.kind === 'reference' ? (
                        referenceImages[selectedImage.index]?.source === 'sketch' && (
                            <Tooltip title={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}>
                                <IconButton
                                    onClick={() => {
                                        onEditSketch(selectedImage.index);
                                        onClose();
                                    }}
                                    aria-label={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}
                                    sx={overlayIconSx}
                                >
                                    <Create fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        )
                    ) : (
                        <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                            <IconButton
                                onClick={() => {
                                    if (!selectedImage) return;
                                    onUseAsReference(selectedImage.src);
                                    onClose();
                                }}
                                aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                sx={overlayIconSx}
                            >
                                <Edit fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    )}
                    <IconButton
                        onClick={onClose}
                        aria-label={t('playground.closePreview', { defaultValue: 'Close image preview' })}
                        sx={overlayIconSx}
                    >
                        <Close />
                    </IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent
                sx={{
                    p: 2,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    bgcolor: 'common.black',
                    overflow: 'hidden',
                    position: 'relative',
                }}
            >
                {/* Top-left, over the artwork: the run's originals first, then
                    what it produced, the one on screen ringed. Clicking a frame
                    swaps the lightbox to it, so comparing an output against the
                    image it was made from is one click each way. */}
                {lightboxFilm.length > 0 && (
                    <Stack
                        data-testid="imagegen-lightbox-film"
                        spacing={0.75}
                        sx={{
                            ...overlayPlateSx,
                            position: 'absolute',
                            top: 12,
                            left: 12,
                            maxHeight: 'calc(100% - 24px)',
                            overflowY: 'auto',
                            p: 0.75,
                            scrollbarWidth: 'thin',
                        }}
                    >
                        {lightboxFilm.map((frame, position) => {
                            const active = selectedImage?.kind === frame.kind && selectedImage.index === frame.index;
                            const label = frame.kind === 'source'
                                ? t('playground.originalBadge', { defaultValue: 'Original' })
                                : t('playground.generatedBadge', { defaultValue: 'Generated' });
                            // The first frame of each group carries the group's word;
                            // repeating it down the column would be noise.
                            const showLabel = position === 0 || lightboxFilm[position - 1].kind !== frame.kind;
                            return (
                                <Box key={`${frame.kind}-${frame.index}`}>
                                    {showLabel && (
                                        <Typography
                                            variant="caption"
                                            sx={{ display: 'block', mb: 0.25, color: 'grey.400', fontSize: 10, lineHeight: 1.4 }}
                                        >
                                            {label}
                                        </Typography>
                                    )}
                                    <Tooltip title={label} placement="right">
                                        <ButtonBase
                                            onClick={() => onShowFrame(frame)}
                                            aria-label={frame.kind === 'source'
                                                ? t('playground.viewSourceImage', {
                                                    defaultValue: 'View original image {{number}}',
                                                    number: frame.index + 1,
                                                })
                                                : t('playground.openResult', {
                                                    defaultValue: 'Open generated image {{number}}',
                                                    number: frame.index + 1,
                                                })}
                                            aria-current={active}
                                            sx={{
                                                display: 'block',
                                                // Smaller on a phone, where the strip shares the
                                                // width with the artwork it sits over.
                                                width: { xs: 36, sm: 48 },
                                                height: { xs: 36, sm: 48 },
                                                borderRadius: 1,
                                                overflow: 'hidden',
                                                outline: active ? '2px solid' : '1px solid',
                                                outlineColor: active ? 'primary.main' : 'rgba(255, 255, 255, 0.24)',
                                                outlineOffset: -1,
                                                opacity: active ? 1 : 0.72,
                                                transition: 'opacity 0.16s ease-out',
                                                '&:hover, &:focus-visible': { opacity: 1 },
                                            }}
                                        >
                                            <Box
                                                component="img"
                                                src={frame.src}
                                                alt=""
                                                sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                                            />
                                        </ButtonBase>
                                    </Tooltip>
                                </Box>
                            );
                        })}
                    </Stack>
                )}
                {selectedImage && (
                    <Box
                        component="img"
                        src={selectedImage.src}
                        alt={selectedImage.label
                            ?? (selectedImage.kind === 'output'
                                ? t('playground.resultAlt', { defaultValue: 'Generated image {{number}}', number: selectedImage.index + 1 })
                                : t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: selectedImage.index + 1 }))}
                        sx={{
                            display: 'block',
                            maxWidth: '100%',
                            maxHeight: '100%',
                            objectFit: 'contain',
                        }}
                    />
                )}
            </DialogContent>
        </Dialog>
    );
};

export default ImageGenLightbox;
