import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Brush, Close, Contrast, Create, DeleteSweep, Eraser, Undo } from '@/components/icons';
import {
    BRUSH_SIZES,
    StrokeHistory,
    fitWithin,
    toCanvasPoint,
    type BrushSizeKey,
    type CanvasDimensions,
    type CanvasPoint,
    type Stroke,
} from '@/utils/sketchCanvas';
import {
    MASK_PAINT_COLOR,
    MASK_PREVIEW_ALPHA,
    hasMaskContent,
    maskPreviewDataURL,
    maskToFile,
    renderMaskPreview,
    type MaskLayers,
} from '@/utils/maskCanvas';

// What a finished mask hands back. `file` is the alpha PNG that goes on the
// wire next to its image; `previewUrl` tints the region that will change, for
// the thumbnail; `layers` is the strokes it was made of, so re-opening gives
// back a mask that can still be painted rather than a picture of one.
export interface MaskResult {
    file: File;
    previewUrl: string;
    layers: MaskLayers;
}

interface MaskEditorDialogProps {
    open: boolean;
    // The reference image being masked: its pixels are the backdrop and its
    // size is the canvas size — a mask that does not match its image exactly
    // is rejected upstream.
    imageUrl: string | null;
    imageName?: string;
    // Re-entry: the mask already on this image, or null for a fresh one. One
    // object, and a stable reference: the canvas resets when it changes.
    initial: MaskLayers | null;
    onClose: () => void;
    onSubmit: (result: MaskResult) => void;
    // Present only when this image already has a mask: a painted region can be
    // taken back, and clearing the canvas cannot say that (an empty mask is
    // not a mask, so Apply stays disabled).
    onRemove?: () => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

type Tool = 'pen' | 'eraser';

interface MaskState {
    strokes: Stroke[];
    inverted: boolean;
}

const MaskEditorDialog: React.FC<MaskEditorDialogProps> = ({
    open,
    imageUrl,
    imageName,
    initial,
    onClose,
    onSubmit,
    onRemove,
    showNotification,
}) => {
    const { t } = useTranslation();
    // Callback refs rather than plain ones: MUI's Dialog mounts through a
    // Portal a tick after `open` flips, so an `[open]` effect would otherwise
    // run against a null canvas (the same trap the sketch canvas hit).
    const [canvasEl, setCanvasEl] = useState<HTMLCanvasElement | null>(null);
    const [stageEl, setStageEl] = useState<HTMLDivElement | null>(null);
    const historyRef = useRef(new StrokeHistory<MaskState>());
    const drawingRef = useRef<{ pointerId: number; stroke: Stroke } | null>(null);
    const frameRef = useRef<number | null>(null);
    const [tool, setTool] = useState<Tool>('pen');
    const [brush, setBrush] = useState<BrushSizeKey>('thick');
    const [strokes, setStrokes] = useState<Stroke[]>([]);
    const [inverted, setInverted] = useState(false);
    const [canUndo, setCanUndo] = useState(false);
    const [stageBox, setStageBox] = useState<CanvasDimensions>({ width: 0, height: 0 });
    const [dims, setDims] = useState<CanvasDimensions | null>(null);
    const [submitting, setSubmitting] = useState(false);

    // The canvas takes the reference image's own pixel size, not the
    // Playground's output Size: the API compares the mask against the image it
    // edits, and one pixel off is a 400. Read from the decoded image rather
    // than trusted from a caller-supplied number.
    useEffect(() => {
        if (!open || !imageUrl) return;
        let cancelled = false;
        const image = new Image();
        image.onload = () => {
            if (!cancelled) setDims({ width: image.naturalWidth, height: image.naturalHeight });
        };
        image.onerror = () => {
            if (cancelled) return;
            showNotification(
                t('playground.mask.loadFailed', { defaultValue: 'Could not read that image, so it cannot be masked.' }),
                'error',
            );
            onClose();
        };
        image.src = imageUrl;
        return () => { cancelled = true; };
    }, [open, imageUrl, onClose, showNotification, t]);

    const cssSize = useMemo(() => (dims ? fitWithin(dims, stageBox) : { width: 0, height: 0 }), [dims, stageBox]);

    // Fresh surface per open: the mask already on this image, or nothing.
    useEffect(() => {
        if (!open) return;
        const opening = initial?.strokes ?? [];
        historyRef.current.clear();
        // Undo frames are the stroke list one stroke shorter, so Ctrl+Z keeps
        // peeling marks painted before the mask was applied. Only the frames
        // that survive the cap are built.
        for (let i = Math.max(0, opening.length - historyRef.current.capacity); i < opening.length; i += 1) {
            historyRef.current.push({ strokes: opening.slice(0, i), inverted: initial?.inverted ?? false });
        }
        drawingRef.current = null;
        setStrokes(opening);
        setInverted(initial?.inverted ?? false);
        setCanUndo(opening.length > 0);
        setTool('pen');
    }, [open, initial]);

    useLayoutEffect(() => {
        if (!open) return;
        const stage = stageEl;
        if (!stage || typeof ResizeObserver === 'undefined') return;
        const measure = () => {
            const rect = stage.getBoundingClientRect();
            setStageBox({ width: rect.width, height: rect.height });
        };
        measure();
        const observer = new ResizeObserver(measure);
        observer.observe(stage);
        return () => observer.disconnect();
    }, [open, stageEl]);

    // One render path, used by the committed state and by the stroke in
    // progress alike. The sketch canvas draws live strokes segment by segment
    // because a full replay per pointermove is visible with hundreds of thin
    // marks; a mask is a handful of fat ones, and inverting means the picture
    // on screen is the *complement* of the strokes — which cannot be built
    // incrementally at all. So it replays, and the two modes stay one path.
    const paint = useCallback((live: Stroke[]) => {
        const ctx = canvasEl?.getContext('2d');
        if (!ctx || !dims) return;
        renderMaskPreview(ctx, { strokes: live, inverted }, dims);
    }, [canvasEl, dims, inverted]);

    useEffect(() => {
        if (!open || !canvasEl || !dims) return;
        canvasEl.width = dims.width;
        canvasEl.height = dims.height;
        paint(strokes);
    }, [open, canvasEl, dims, strokes, inverted, paint]);

    useEffect(() => () => {
        if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
    }, []);

    const snapshot = useCallback(() => {
        historyRef.current.push({ strokes, inverted });
        setCanUndo(true);
    }, [inverted, strokes]);

    const handleUndo = useCallback(() => {
        const frame = historyRef.current.pop();
        if (!frame) return;
        setStrokes(frame.strokes);
        setInverted(frame.inverted);
        setCanUndo(historyRef.current.canUndo);
    }, []);

    const handleClear = useCallback(() => {
        snapshot();
        setStrokes([]);
        setInverted(false);
    }, [snapshot]);

    const handleInvert = useCallback(() => {
        snapshot();
        setInverted((current) => !current);
    }, [snapshot]);

    useEffect(() => {
        if (!open) return;
        const onKeyDown = (event: KeyboardEvent) => {
            if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z' && !event.shiftKey) {
                event.preventDefault();
                handleUndo();
            }
        };
        window.addEventListener('keydown', onKeyDown);
        return () => window.removeEventListener('keydown', onKeyDown);
    }, [open, handleUndo]);

    const pointFromEvent = useCallback((
        event: { clientX: number; clientY: number },
        rect: DOMRect,
    ): CanvasPoint => toCanvasPoint(event.clientX, event.clientY, rect, dims ?? { width: 1, height: 1 }), [dims]);

    const handlePointerDown = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        if (event.button !== 0 || drawingRef.current || !dims) return;
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        const point = pointFromEvent(event, event.currentTarget.getBoundingClientRect());
        const stroke: Stroke = {
            tool: tool === 'eraser' ? 'eraser' : 'pen',
            // A mask has no colours — only covered and not covered. The field
            // rides along because the stroke shape is shared with the sketch.
            color: MASK_PAINT_COLOR,
            brush,
            points: [point],
        };
        drawingRef.current = { pointerId: event.pointerId, stroke };
        paint([...strokes, stroke]);
    }, [brush, dims, paint, pointFromEvent, strokes, tool]);

    const handlePointerMove = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        const drawing = drawingRef.current;
        if (!drawing || drawing.pointerId !== event.pointerId) return;
        event.preventDefault();
        // Coalesced samples are what makes a fast stroke a curve instead of a
        // polyline; one layout read covers all of them.
        const rect = event.currentTarget.getBoundingClientRect();
        const samples = typeof event.nativeEvent.getCoalescedEvents === 'function'
            ? event.nativeEvent.getCoalescedEvents()
            : [];
        const points = (samples.length > 0 ? samples : [event.nativeEvent])
            .map((sample) => pointFromEvent(sample, rect));
        drawing.stroke.points.push(...points);
        // Repaint at most once per frame: pointer events can outpace the
        // display, and every one of them repaints the whole mask.
        if (frameRef.current !== null) return;
        frameRef.current = requestAnimationFrame(() => {
            frameRef.current = null;
            const current = drawingRef.current;
            if (current) paint([...strokes, current.stroke]);
        });
    }, [paint, pointFromEvent, strokes]);

    const handlePointerEnd = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        const drawing = drawingRef.current;
        if (!drawing || drawing.pointerId !== event.pointerId) return;
        drawingRef.current = null;
        if (event.currentTarget.hasPointerCapture(event.pointerId)) {
            event.currentTarget.releasePointerCapture(event.pointerId);
        }
        // One gesture, one undo step — the stroke joins the list only now.
        snapshot();
        setStrokes((current) => [...current, drawing.stroke]);
    }, [snapshot]);

    const selects = hasMaskContent(strokes);

    const handleSubmit = useCallback(async () => {
        if (!dims || !selects) return;
        setSubmitting(true);
        try {
            const layers: MaskLayers = { size: dims, strokes, inverted };
            const file = await maskToFile(layers, dims);
            if (!file) {
                showNotification(
                    t('playground.mask.exportFailed', { defaultValue: 'Could not build the mask image.' }),
                    'error',
                );
                return;
            }
            onSubmit({ file, previewUrl: maskPreviewDataURL(layers, dims), layers });
        } finally {
            setSubmitting(false);
        }
    }, [dims, inverted, onSubmit, selects, showNotification, strokes, t]);

    const toolLabel = (key: Tool) => (key === 'pen'
        ? t('playground.mask.pen', { defaultValue: 'Paint' })
        : t('playground.mask.eraser', { defaultValue: 'Erase' }));
    const brushLabel = (key: BrushSizeKey) => t(`playground.sketch.brush.${key}`, {
        defaultValue: key === 'thin' ? 'Thin' : key === 'thick' ? 'Thick' : 'Medium',
    });

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 1 }}>
                <Brush fontSize="small" />
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="h6" component="span" sx={{ display: 'block', fontSize: '1.05rem' }}>
                        {t('playground.mask.title', { defaultValue: 'Mask' })}
                    </Typography>
                    <Typography variant="caption" sx={{ display: 'block', color: 'text.secondary' }}>
                        {dims
                            ? t('playground.mask.canvasSize', {
                                defaultValue: '{{width}} × {{height}} px · matches the reference image',
                                width: dims.width,
                                height: dims.height,
                            })
                            : (imageName ?? '')}
                    </Typography>
                </Box>
                <IconButton onClick={onClose} aria-label={t('playground.mask.close', { defaultValue: 'Close mask editor' })}>
                    <Close />
                </IconButton>
            </DialogTitle>
            <DialogContent dividers sx={{ p: { xs: 1.5, sm: 2 } }}>
                <Stack spacing={1.5}>
                    <Stack direction="row" spacing={1.5} useFlexGap sx={{ flexWrap: 'wrap', alignItems: 'center' }}>
                        <ToggleButtonGroup
                            value={tool}
                            exclusive
                            size="small"
                            onChange={(_, next: Tool | null) => { if (next) setTool(next); }}
                            aria-label={t('playground.mask.tool', { defaultValue: 'Tool' })}
                        >
                            <ToggleButton value="pen" aria-label={toolLabel('pen')}>
                                <Tooltip title={toolLabel('pen')}><Create fontSize="small" /></Tooltip>
                            </ToggleButton>
                            <ToggleButton value="eraser" aria-label={toolLabel('eraser')}>
                                <Tooltip title={toolLabel('eraser')}><Eraser fontSize="small" /></Tooltip>
                            </ToggleButton>
                        </ToggleButtonGroup>

                        <ToggleButtonGroup
                            value={brush}
                            exclusive
                            size="small"
                            onChange={(_, next: BrushSizeKey | null) => { if (next) setBrush(next); }}
                            aria-label={t('playground.sketch.brushSize', { defaultValue: 'Brush size' })}
                        >
                            {BRUSH_SIZES.map((option) => (
                                <ToggleButton key={option.key} value={option.key} aria-label={brushLabel(option.key)}>
                                    <Tooltip title={brushLabel(option.key)}>
                                        <Box
                                            sx={{
                                                width: 6 + option.width * 0.6,
                                                height: 6 + option.width * 0.6,
                                                borderRadius: '50%',
                                                bgcolor: 'currentColor',
                                            }}
                                        />
                                    </Tooltip>
                                </ToggleButton>
                            ))}
                        </ToggleButtonGroup>

                        {/* First-class, not hidden: "change the background" is
                            painting the subject and flipping, otherwise the
                            user traces a whole outline the long way round. */}
                        <Button
                            size="small"
                            variant={inverted ? 'contained' : 'outlined'}
                            color="inherit"
                            onClick={handleInvert}
                            startIcon={<Contrast fontSize="small" />}
                            sx={{ textTransform: 'none', py: 0.1, px: 1 }}
                        >
                            {t('playground.mask.invert', { defaultValue: 'Invert' })}
                        </Button>

                        <Box sx={{ flex: 1 }} />

                        <Tooltip title={t('playground.sketch.undo', { defaultValue: 'Undo (Ctrl+Z)' })}>
                            <span>
                                <IconButton size="small" onClick={handleUndo} disabled={!canUndo}>
                                    <Undo fontSize="small" />
                                </IconButton>
                            </span>
                        </Tooltip>
                        <Tooltip title={t('playground.mask.clear', { defaultValue: 'Clear mask' })}>
                            <span>
                                <IconButton
                                    size="small"
                                    onClick={handleClear}
                                    disabled={strokes.length === 0 && !inverted}
                                >
                                    <DeleteSweep fontSize="small" />
                                </IconButton>
                            </span>
                        </Tooltip>
                    </Stack>

                    <Box
                        ref={setStageEl}
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            height: { xs: '55vh', sm: '60vh' },
                            minHeight: 240,
                            borderRadius: 2,
                            bgcolor: 'action.hover',
                            overflow: 'hidden',
                        }}
                    >
                        <Box
                            sx={{ position: 'relative', lineHeight: 0 }}
                            style={{ width: cssSize.width, height: cssSize.height }}
                        >
                            {imageUrl ? (
                                <Box
                                    component="img"
                                    src={imageUrl}
                                    alt={imageName ?? t('playground.mask.imageAlt', { defaultValue: 'Image being masked' })}
                                    style={{ width: cssSize.width, height: cssSize.height }}
                                    sx={{ display: 'block', borderRadius: 1, boxShadow: 1, userSelect: 'none' }}
                                />
                            ) : null}
                            {/* The tint is element opacity rather than a
                                translucent paint colour: overlapping strokes
                                would otherwise stack into a darker patch that
                                reads as a second selection. */}
                            <Box
                                component="canvas"
                                ref={setCanvasEl}
                                role="img"
                                aria-label={t('playground.mask.canvasAlt', { defaultValue: 'Mask canvas' })}
                                onPointerDown={handlePointerDown}
                                onPointerMove={handlePointerMove}
                                onPointerUp={handlePointerEnd}
                                onPointerCancel={handlePointerEnd}
                                onContextMenu={(event: React.MouseEvent) => event.preventDefault()}
                                style={{ width: cssSize.width, height: cssSize.height, opacity: MASK_PREVIEW_ALPHA }}
                                sx={{
                                    position: 'absolute',
                                    top: 0,
                                    left: 0,
                                    display: 'block',
                                    borderRadius: 1,
                                    touchAction: 'none',
                                    cursor: 'crosshair',
                                    userSelect: 'none',
                                }}
                            />
                        </Box>
                    </Box>

                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t('playground.mask.hint', {
                            defaultValue: 'Painted areas are what the model may change; everything else stays. The prompt says what should appear there.',
                        })}
                    </Typography>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 2, py: 1.5 }}>
                {onRemove ? (
                    <Button color="inherit" onClick={onRemove} sx={{ mr: 'auto', color: 'text.secondary' }}>
                        {t('playground.mask.remove', { defaultValue: 'Remove mask' })}
                    </Button>
                ) : null}
                <Button onClick={onClose}>
                    {t('playground.mask.cancel', { defaultValue: 'Cancel' })}
                </Button>
                <Button
                    variant="contained"
                    onClick={() => { void handleSubmit(); }}
                    disabled={!selects || submitting}
                    startIcon={<Brush />}
                >
                    {initial
                        ? t('playground.mask.update', { defaultValue: 'Update mask' })
                        : t('playground.mask.use', { defaultValue: 'Use mask' })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default MaskEditorDialog;
