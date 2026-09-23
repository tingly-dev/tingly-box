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
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Brush, Close, ContentCopy, Create, Download, Edit, GridView, RestartAlt } from '@/components/icons';
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
    // On by default — a mask was painted to be seen — and remembered while
    // the lightbox stays mounted, so walking the filmstrip does not reset it.
    const [showMask, setShowMask] = useState(true);
    const imageAlt = selectedImage
        ? selectedImage.label ?? (selectedImage.kind === 'output' ? t('playground.resultAlt', { defaultValue: 'Generated image {{number}}', number: selectedImage.index + 1 }) : t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: selectedImage.index + 1 }))
        : '';
    const maskLabel = showMask
        ? t('playground.mask.hideOverlay', { defaultValue: 'Hide mask' })
        : t('playground.mask.showOverlay', { defaultValue: 'Show mask' });
    // One plate per kind of frame. The plate carries its group's word once;
    // repeating it on every frame would be noise.
    const renderFilm = (
        kind: LightboxFrame['kind'],
        corner: { top?: number; bottom?: number; left?: number; right?: number },
        tooltipPlacement: 'left' | 'right',
    ) => {
        const frames = lightboxFilm.filter((frame) => frame.kind === kind);
        if (frames.length === 0) return null;
        const sharesHeight = lightboxFilm.some((frame) => frame.kind !== kind);
        const label = kind === 'source'
            ? t('playground.originalBadge', { defaultValue: 'Original' })
            : kind === 'reference'
                ? t('playground.referenceBadge', { defaultValue: 'Reference' })
                : t('playground.generatedBadge', { defaultValue: 'Generated' });
        return (
            <Stack
                data-testid={`imagegen-lightbox-film-${kind}`}
                spacing={0.75}
                sx={{
                    ...overlayPlateSx,
                    position: 'absolute',
                    ...corner,
                    // With both plates up they split the height, each
                    // scrolling on its own rather than running into the other.
                    maxHeight: sharesHeight ? 'calc(50% - 18px)' : 'calc(100% - 24px)',
                    overflowY: 'auto',
                    p: 0.75,
                    scrollbarWidth: 'thin',
                }}
            >
                <Typography
                    variant="caption"
                    sx={{ display: 'block', color: 'grey.400', fontSize: 10, lineHeight: 1.4 }}
                >
                    {label}
                </Typography>
                {frames.map((frame) => {
                    const active = selectedImage?.kind === frame.kind && selectedImage.index === frame.index;
                    return (
                        <Tooltip key={`${frame.kind}-${frame.index}`} title={label} placement={tooltipPlacement}>
                            <ButtonBase
                                onClick={() => onShowFrame(frame)}
                                aria-label={frame.kind === 'source'
                                    ? t('playground.viewSourceImage', {
                                        defaultValue: 'View original image {{number}}',
                                        number: frame.index + 1,
                                    })
                                    : frame.kind === 'reference'
                                        ? t('playground.referenceThumbAlt', {
                                            defaultValue: 'Reference image {{number}}',
                                            number: frame.index + 1,
                                        })
                                        : t('playground.openResult', {
                                        defaultValue: 'Open generated image {{number}}',
                                        number: frame.index + 1,
                                    })}
                                aria-current={active}
                                sx={{
                                    display: 'block',
                                    flexShrink: 0,
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
                    );
                })}
            </Stack>
        );
    };

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
                    {selectedImage?.maskSrc && (
                        <Tooltip title={maskLabel}>
                            <IconButton
                                onClick={() => setShowMask((value) => !value)}
                                aria-label={maskLabel}
                                aria-pressed={showMask}
                                data-testid="imagegen-lightbox-mask-toggle"
                                sx={{
                                    ...overlayIconSx,
                                    // Pressed reads as pressed: the button takes
                                    // the tint the overlay is drawn in.
                                    ...(showMask ? { color: 'warning.light', bgcolor: 'rgba(255, 255, 255, 0.2)' } : {}),
                                }}
                            >
                                <Brush fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    )}
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
                {/* Over the artwork, the run's two halves at opposite corners:
                    what went in top-left, what came out bottom-right — the
                    order a request reads in. The one on screen is ringed, and
                    clicking a frame swaps the lightbox to it, so comparing an
                    output against the image it was made from is one click each
                    way. */}
                {renderFilm('source', { top: 12, left: 12 }, 'right')}
                {/* Opened from the panel's reference row: the row itself, in
                    the same corner the inputs of a run sit in. */}
                {renderFilm('reference', { top: 12, left: 12 }, 'right')}
                {renderFilm('output', { bottom: 12, right: 12 }, 'left')}
                {selectedImage && (selectedImage.maskSrc ? (
                    // The mask has the image's exact pixel size (the editor
                    // paints at the reference's own resolution), so two
                    // layers filling the same box with `scale-down` line up
                    // pixel for pixel without measuring anything — and, like
                    // the unmasked image, a small one is never blown up.
                    <Box sx={{ position: 'relative', width: '100%', height: '100%' }}>
                        <Box
                            component="img"
                            src={selectedImage.src}
                            alt={imageAlt}
                            sx={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'scale-down', display: 'block' }}
                        />
                        {showMask && (
                            <Box
                                component="img"
                                src={selectedImage.maskSrc}
                                alt=""
                                aria-hidden
                                data-testid="imagegen-lightbox-mask"
                                sx={{
                                    position: 'absolute',
                                    inset: 0,
                                    width: '100%',
                                    height: '100%',
                                    objectFit: 'scale-down',
                                    display: 'block',
                                    opacity: 0.55,
                                    pointerEvents: 'none',
                                }}
                            />
                        )}
                    </Box>
                ) : (
                    <Box
                        component="img"
                        src={selectedImage.src}
                        alt={imageAlt}
                        sx={{
                            display: 'block',
                            maxWidth: '100%',
                            maxHeight: '100%',
                            objectFit: 'contain',
                        }}
                    />
                ))}
            </DialogContent>
        </Dialog>
    );
};

export default ImageGenLightbox;
