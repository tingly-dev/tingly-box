import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
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
import { Close, Create, DeleteSweep, Eraser, Undo } from '@/components/icons';
import {
    BRUSH_SIZES,
    brushWidthFor,
    ERASER_WIDTH_MULTIPLIER,
    fitWithin,
    parseImageSize,
    SKETCH_BACKGROUND,
    SKETCH_COLORS,
    StrokeHistory,
    toCanvasPoint,
    type BrushSizeKey,
    type CanvasDimensions,
    type CanvasPoint,
} from '@/utils/sketchCanvas';

type Tool = 'pen' | 'eraser';

export interface SketchResult {
    file: File;
    previewUrl: string;
}

interface SketchCanvasDialogProps {
    open: boolean;
    // The Playground's output size ("1024x1024"); the canvas takes its shape.
    size: string;
    // Re-entry: a previous sketch (data URL) to keep drawing on. `null` opens
    // a blank canvas.
    initialImage: string | null;
    onClose: () => void;
    onSubmit: (result: SketchResult) => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const loadDataUrl = (src: string): Promise<HTMLImageElement> => new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error('sketch image failed to load'));
    image.src = src;
});

const SketchCanvasDialog: React.FC<SketchCanvasDialogProps> = ({
    open,
    size,
    initialImage,
    onClose,
    onSubmit,
    showNotification,
}) => {
    const { t } = useTranslation();
    // Element state rather than refs: the Dialog mounts its children through
    // a Portal one tick after `open` flips, so a plain ref is still null when
    // an `[open]` effect runs. Callback refs re-run the effects once the
    // nodes actually exist.
    const [canvasEl, setCanvasEl] = useState<HTMLCanvasElement | null>(null);
    const [stageEl, setStageEl] = useState<HTMLDivElement | null>(null);
    const historyRef = useRef(new StrokeHistory<ImageData>());
    const drawingRef = useRef<{ pointerId: number; last: CanvasPoint } | null>(null);
    const [tool, setTool] = useState<Tool>('pen');
    const [color, setColor] = useState<string>(SKETCH_COLORS[0]);
    const [brush, setBrush] = useState<BrushSizeKey>('medium');
    // Two facts the toolbar and the primary action key off: whether there is
    // anything to undo, and whether there is anything on the canvas at all.
    const [canUndo, setCanUndo] = useState(false);
    const [dirty, setDirty] = useState(false);
    const [stageBox, setStageBox] = useState<CanvasDimensions>({ width: 0, height: 0 });

    // The size is read once per open: changing Size mid-sketch would have to
    // resample the drawing, and the dialog is short-lived enough that
    // reopening is the honest answer.
    const dims = useMemo(() => parseImageSize(size), [size]);
    const cssSize = useMemo(() => fitWithin(dims, stageBox), [dims, stageBox]);

    const getContext = useCallback(() => canvasEl?.getContext('2d') ?? null, [canvasEl]);

    const paintBackground = useCallback((ctx: CanvasRenderingContext2D) => {
        ctx.save();
        ctx.globalCompositeOperation = 'source-over';
        ctx.fillStyle = SKETCH_BACKGROUND;
        ctx.fillRect(0, 0, dims.width, dims.height);
        ctx.restore();
    }, [dims]);

    // Fresh surface on every open: white background (or the sketch being
    // re-edited), empty history, pen selected.
    useEffect(() => {
        if (!open) return;
        const canvas = canvasEl;
        const ctx = getContext();
        if (!canvas || !ctx) return;
        let cancelled = false;
        canvas.width = dims.width;
        canvas.height = dims.height;
        paintBackground(ctx);
        historyRef.current.clear();
        drawingRef.current = null;
        setCanUndo(false);
        setDirty(initialImage !== null);
        setTool('pen');
        if (initialImage) {
            loadDataUrl(initialImage)
                .then((image) => {
                    if (cancelled) return;
                    ctx.drawImage(image, 0, 0, dims.width, dims.height);
                })
                .catch(() => {
                    if (!cancelled) setDirty(false);
                });
        }
        return () => { cancelled = true; };
    }, [open, canvasEl, dims, initialImage, getContext, paintBackground]);

    // Track the stage's box so the canvas can be sized to fit it — a fixed
    // pixel size would either overflow phones or leave desktops with a stamp.
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

    const snapshot = useCallback(() => {
        const ctx = getContext();
        if (!ctx) return;
        historyRef.current.push(ctx.getImageData(0, 0, dims.width, dims.height));
        setCanUndo(true);
    }, [dims, getContext]);

    const handleUndo = useCallback(() => {
        const ctx = getContext();
        const frame = historyRef.current.pop();
        if (!ctx || !frame) return;
        ctx.putImageData(frame, 0, 0);
        setCanUndo(historyRef.current.canUndo);
        // The first frame is always the blank/initial surface, so an empty
        // history means nothing of the user's is left on the canvas.
        setDirty(historyRef.current.canUndo || initialImage !== null);
    }, [getContext, initialImage]);

    const handleClear = useCallback(() => {
        const ctx = getContext();
        if (!ctx) return;
        snapshot();
        paintBackground(ctx);
        setDirty(false);
    }, [getContext, paintBackground, snapshot]);

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

    const pointFromEvent = useCallback((event: { clientX: number; clientY: number }) => {
        if (!canvasEl) return { x: 0, y: 0 };
        return toCanvasPoint(event.clientX, event.clientY, canvasEl.getBoundingClientRect(), dims);
    }, [canvasEl, dims]);

    const applyStrokeStyle = useCallback((ctx: CanvasRenderingContext2D) => {
        const width = brushWidthFor(brush, dims);
        ctx.lineCap = 'round';
        ctx.lineJoin = 'round';
        ctx.globalCompositeOperation = 'source-over';
        if (tool === 'eraser') {
            ctx.strokeStyle = SKETCH_BACKGROUND;
            ctx.lineWidth = width * ERASER_WIDTH_MULTIPLIER;
        } else {
            ctx.strokeStyle = color;
            ctx.lineWidth = width;
        }
    }, [brush, color, dims, tool]);

    const handlePointerDown = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        // Primary button / touch / pen only — a right-click is not a stroke.
        if (event.button !== 0 || drawingRef.current) return;
        const ctx = getContext();
        if (!ctx) return;
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        snapshot();
        const point = pointFromEvent(event);
        applyStrokeStyle(ctx);
        // A tap with no movement still leaves a dot.
        ctx.beginPath();
        ctx.moveTo(point.x, point.y);
        ctx.lineTo(point.x + 0.01, point.y + 0.01);
        ctx.stroke();
        drawingRef.current = { pointerId: event.pointerId, last: point };
        setDirty(true);
    }, [applyStrokeStyle, getContext, pointFromEvent, snapshot]);

    const handlePointerMove = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        const drawing = drawingRef.current;
        if (!drawing || drawing.pointerId !== event.pointerId) return;
        const ctx = getContext();
        if (!ctx) return;
        event.preventDefault();
        // Coalesced events carry the intermediate samples the browser batched
        // between frames — the difference between a curve and a polyline on a
        // fast stroke.
        const samples = typeof event.nativeEvent.getCoalescedEvents === 'function'
            ? event.nativeEvent.getCoalescedEvents()
            : [];
        const points = (samples.length > 0 ? samples : [event.nativeEvent]).map(pointFromEvent);
        ctx.beginPath();
        ctx.moveTo(drawing.last.x, drawing.last.y);
        for (const point of points) ctx.lineTo(point.x, point.y);
        ctx.stroke();
        drawing.last = points[points.length - 1];
    }, [getContext, pointFromEvent]);

    const handlePointerEnd = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        const drawing = drawingRef.current;
        if (!drawing || drawing.pointerId !== event.pointerId) return;
        drawingRef.current = null;
        if (event.currentTarget.hasPointerCapture(event.pointerId)) {
            event.currentTarget.releasePointerCapture(event.pointerId);
        }
    }, []);

    const handleSubmit = useCallback(() => {
        const canvas = canvasEl;
        if (!canvas) return;
        canvas.toBlob((blob) => {
            if (!blob) {
                showNotification(t('playground.sketch.failed', { defaultValue: 'Could not export the sketch' }), 'error');
                return;
            }
            onSubmit({
                file: new File([blob], `sketch-${Date.now()}.png`, { type: 'image/png' }),
                previewUrl: canvas.toDataURL('image/png'),
            });
        }, 'image/png');
    }, [canvasEl, onSubmit, showNotification, t]);

    const toolLabel = (key: Tool) => (key === 'pen'
        ? t('playground.sketch.pen', { defaultValue: 'Pen' })
        : t('playground.sketch.eraser', { defaultValue: 'Eraser' }));
    const brushLabel = (key: BrushSizeKey) => t(`playground.sketch.brush.${key}`, {
        defaultValue: key === 'thin' ? 'Thin' : key === 'thick' ? 'Thick' : 'Medium',
    });

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 1 }}>
                <Create fontSize="small" />
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="h6" component="span" sx={{ display: 'block', fontSize: '1.05rem' }}>
                        {t('playground.sketch.title', { defaultValue: 'Sketch' })}
                    </Typography>
                    <Typography variant="caption" sx={{ display: 'block', color: 'text.secondary' }}>
                        {t('playground.sketch.canvasSize', {
                            defaultValue: '{{width}} × {{height}} px · matches Size',
                            width: dims.width,
                            height: dims.height,
                        })}
                    </Typography>
                </Box>
                <IconButton onClick={onClose} aria-label={t('playground.sketch.close', { defaultValue: 'Close sketch' })}>
                    <Close />
                </IconButton>
            </DialogTitle>
            <DialogContent dividers sx={{ p: { xs: 1.5, sm: 2 } }}>
                <Stack spacing={1.5}>
                    <Stack
                        direction="row"
                        spacing={1.5}
                        useFlexGap
                        sx={{ flexWrap: 'wrap', alignItems: 'center' }}
                    >
                        <ToggleButtonGroup
                            value={tool}
                            exclusive
                            size="small"
                            onChange={(_, next: Tool | null) => { if (next) setTool(next); }}
                            aria-label={t('playground.sketch.tool', { defaultValue: 'Tool' })}
                        >
                            <ToggleButton value="pen" aria-label={toolLabel('pen')}>
                                <Tooltip title={toolLabel('pen')}><Create fontSize="small" /></Tooltip>
                            </ToggleButton>
                            <ToggleButton value="eraser" aria-label={toolLabel('eraser')}>
                                <Tooltip title={toolLabel('eraser')}><Eraser fontSize="small" /></Tooltip>
                            </ToggleButton>
                        </ToggleButtonGroup>

                        <Stack
                            direction="row"
                            spacing={0.75}
                            role="radiogroup"
                            aria-label={t('playground.sketch.color', { defaultValue: 'Colour' })}
                            sx={{ alignItems: 'center' }}
                        >
                            {SKETCH_COLORS.map((swatch) => {
                                const selected = tool === 'pen' && swatch === color;
                                return (
                                    <ButtonBase
                                        key={swatch}
                                        role="radio"
                                        aria-checked={selected}
                                        aria-label={swatch}
                                        onClick={() => { setColor(swatch); setTool('pen'); }}
                                        sx={{
                                            width: 24,
                                            height: 24,
                                            borderRadius: '50%',
                                            bgcolor: swatch,
                                            border: '2px solid',
                                            borderColor: selected ? 'primary.main' : 'background.paper',
                                            boxShadow: selected ? '0 0 0 2px rgba(25, 118, 210, 0.35)' : '0 0 0 1px rgba(0,0,0,0.18)',
                                            transition: 'box-shadow 0.12s ease-out',
                                        }}
                                    />
                                );
                            })}
                        </Stack>

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

                        <Box sx={{ flex: 1 }} />

                        <Tooltip title={t('playground.sketch.undo', { defaultValue: 'Undo (Ctrl+Z)' })}>
                            <span>
                                <IconButton
                                    size="small"
                                    onClick={handleUndo}
                                    disabled={!canUndo}
                                    aria-label={t('playground.sketch.undo', { defaultValue: 'Undo (Ctrl+Z)' })}
                                >
                                    <Undo fontSize="small" />
                                </IconButton>
                            </span>
                        </Tooltip>
                        <Tooltip title={t('playground.sketch.clear', { defaultValue: 'Clear canvas' })}>
                            <span>
                                <IconButton
                                    size="small"
                                    onClick={handleClear}
                                    disabled={!dirty}
                                    aria-label={t('playground.sketch.clear', { defaultValue: 'Clear canvas' })}
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
                            component="canvas"
                            ref={setCanvasEl}
                            role="img"
                            aria-label={t('playground.sketch.canvasAlt', { defaultValue: 'Sketch canvas' })}
                            onPointerDown={handlePointerDown}
                            onPointerMove={handlePointerMove}
                            onPointerUp={handlePointerEnd}
                            onPointerCancel={handlePointerEnd}
                            onContextMenu={(event: React.MouseEvent) => event.preventDefault()}
                            style={{ width: cssSize.width, height: cssSize.height }}
                            sx={{
                                display: 'block',
                                bgcolor: SKETCH_BACKGROUND,
                                boxShadow: 1,
                                borderRadius: 1,
                                // Draw, don't scroll: the browser must not steal the gesture.
                                touchAction: 'none',
                                cursor: 'crosshair',
                                userSelect: 'none',
                            }}
                        />
                    </Box>

                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t('playground.sketch.hint', {
                            defaultValue: 'A rough sketch is enough — the prompt says what it should become. It joins the reference images and goes to the model as-is.',
                        })}
                    </Typography>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 2, py: 1.5 }}>
                <Button onClick={onClose}>
                    {t('playground.sketch.cancel', { defaultValue: 'Cancel' })}
                </Button>
                <Button variant="contained" onClick={handleSubmit} disabled={!dirty} startIcon={<Create />}>
                    {initialImage
                        ? t('playground.sketch.update', { defaultValue: 'Update sketch' })
                        : t('playground.sketch.use', { defaultValue: 'Use sketch' })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default SketchCanvasDialog;
