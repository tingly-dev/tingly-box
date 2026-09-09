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
import { Accessibility, Add, Close, Create, Delete, DeleteSweep, Eraser, Flip, Undo } from '@/components/icons';
import {
    applyStrokeStyle,
    BRUSH_SIZES,
    fitWithin,
    parseImageSize,
    renderStrokes,
    SKETCH_BACKGROUND,
    SKETCH_COLORS,
    StrokeHistory,
    toCanvasPoint,
    type BrushSizeKey,
    type CanvasDimensions,
    type CanvasPoint,
    type Stroke,
} from '@/utils/sketchCanvas';
import {
    applyPreset,
    createFigure,
    drawFigure,
    drawFigureHandles,
    figureBounds,
    flipFigure,
    hitTestJoint,
    nextFigureAt,
    placeNewFigure,
    isScaleHandleHit,
    moveJoint,
    scaleFigure,
    translateFigure,
    type JointKey,
    type PoseFigure,
    type PosePresetKey,
} from '@/utils/poseFigure';

type Tool = 'pen' | 'eraser' | 'pose';

export interface SketchLayers {
    strokes: Stroke[];
    figures: PoseFigure[];
    // Pixels the sketch was opened on top of and cannot re-derive: a sketch
    // flattened by an older build. Normally null.
    backdrop: string | null;
}

const PRESET_KEYS: readonly PosePresetKey[] = ['standing', 'walking', 'sitting', 'armsUp'];

// One undo stack for the whole surface: a snapshot of whichever layer the
// action touched, so Ctrl+Z always means "the last thing I did", whichever
// tool did it. Both layers are lists now, which is why an undo frame costs
// bytes instead of the four megabytes an ImageData of a 1024² canvas did.
interface SketchSnapshot {
    strokes: Stroke[] | null;
    figures: PoseFigure[] | null;
    backdrop: HTMLImageElement | null;
    hadBackdrop: boolean;
}

// `before` is the figure list as it was when the drag started; it only
// reaches the undo stack once the pointer actually moves, so selecting a
// figure does not leave an undo step that does nothing.
type PoseDragBase = { pointerId: number; figureId: string; before: PoseFigure[]; committed: boolean };
type PoseDrag =
    | (PoseDragBase & { mode: 'joint'; joint: JointKey })
    | (PoseDragBase & { mode: 'move'; last: CanvasPoint })
    | (PoseDragBase & { mode: 'scale'; origin: CanvasPoint; startDistance: number; start: PoseFigure });

// What a finished sketch hands back. `file`/`previewUrl` are the composited
// pixels the model gets; `layers` is what it was made of, so re-opening it
// restores editable strokes and a posable figure instead of a picture of them.
export interface SketchResult {
    file: File;
    previewUrl: string;
    layers: SketchLayers;
}

interface SketchCanvasDialogProps {
    open: boolean;
    // The Playground's output size ("1024x1024"); the canvas takes its shape.
    size: string;
    // Re-entry: the stroke layer (data URL) and the figures of a previous
    // sketch. `null` + an empty list opens a blank canvas. A sketch made
    // before layers were kept has only flattened pixels: it comes back as
    // strokes with no figures, which is still drawable, just not posable.
    initialImage: string | null;
    initialStrokes: Stroke[];
    initialFigures: PoseFigure[];
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
    initialStrokes,
    initialFigures,
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
    const [overlayEl, setOverlayEl] = useState<HTMLCanvasElement | null>(null);
    const [stageEl, setStageEl] = useState<HTMLDivElement | null>(null);
    const historyRef = useRef(new StrokeHistory<SketchSnapshot>());
    // The stroke in progress: drawn to the canvas segment by segment as it
    // happens (a full redraw per pointermove would be visible), and committed
    // to the list on pointer-up.
    const drawingRef = useRef<{ pointerId: number; last: CanvasPoint; stroke: Stroke } | null>(null);
    const poseDragRef = useRef<PoseDrag | null>(null);
    const [tool, setTool] = useState<Tool>('pen');
    const [color, setColor] = useState<string>(SKETCH_COLORS[0]);
    const [brush, setBrush] = useState<BrushSizeKey>('medium');
    // Neither layer is ever pixels while the dialog is open: strokes are the
    // points they were drawn from, figures are joints. "Used" is not "locked"
    // (principle 10) — both stay editable, and the flattened PNG exists only
    // as the thing handed to the model.
    const [strokes, setStrokes] = useState<Stroke[]>([]);
    const [backdrop, setBackdrop] = useState<HTMLImageElement | null>(null);
    const [figures, setFigures] = useState<PoseFigure[]>([]);
    const [selectedId, setSelectedId] = useState<string | null>(null);
    // Two facts the toolbar and the primary action key off: whether there is
    // anything to undo, and whether there is anything on the canvas at all.
    const [canUndo, setCanUndo] = useState(false);
    const [stageBox, setStageBox] = useState<CanvasDimensions>({ width: 0, height: 0 });

    // The size is read once per open: changing Size mid-sketch would have to
    // resample the drawing, and the dialog is short-lived enough that
    // reopening is the honest answer.
    const dims = useMemo(() => parseImageSize(size), [size]);
    const cssSize = useMemo(() => fitWithin(dims, stageBox), [dims, stageBox]);
    const dirty = strokes.length > 0 || figures.length > 0 || backdrop !== null;
    const selectedFigure = useMemo(
        () => figures.find((figure) => figure.id === selectedId) ?? null,
        [figures, selectedId],
    );

    // Hit targets and handles are specified in screen pixels and converted to
    // canvas units, so grabbing a joint feels the same on a 512 and a 1792
    // canvas.
    const displayScale = cssSize.width > 0 ? cssSize.width / dims.width : 1;
    const toCanvasPx = useCallback((screenPx: number) => screenPx / (displayScale || 1), [displayScale]);

    const getContext = useCallback(() => canvasEl?.getContext('2d') ?? null, [canvasEl]);

    const paintBackground = useCallback((ctx: CanvasRenderingContext2D) => {
        ctx.save();
        ctx.globalCompositeOperation = 'source-over';
        ctx.fillStyle = SKETCH_BACKGROUND;
        ctx.fillRect(0, 0, dims.width, dims.height);
        ctx.restore();
    }, [dims]);

    // The stroke layer is rebuilt from its list, never patched: backdrop
    // first, then every stroke in order. Undo is therefore "drop the last
    // stroke and redraw", not "put back a saved frame".
    const redraw = useCallback((nextStrokes: readonly Stroke[], image: HTMLImageElement | null) => {
        const ctx = getContext();
        if (!ctx) return;
        paintBackground(ctx);
        if (image) ctx.drawImage(image, 0, 0, dims.width, dims.height);
        renderStrokes(ctx, nextStrokes, dims);
    }, [dims, getContext, paintBackground]);

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
        // Undo is seeded from the strokes themselves: each frame is the list
        // one stroke shorter. That is what lets Ctrl+Z keep peeling marks that
        // were drawn before the sketch was saved, instead of stopping dead at
        // whatever state it re-opened in. (Figures cannot be rebuilt this way
        // — they come back at their saved pose, and undo starts from there.)
        for (let i = 0; i < initialStrokes.length; i += 1) {
            historyRef.current.push({
                strokes: initialStrokes.slice(0, i),
                figures: null,
                backdrop: null,
                hadBackdrop: initialImage !== null,
            });
        }
        drawingRef.current = null;
        poseDragRef.current = null;
        setCanUndo(initialStrokes.length > 0);
        setStrokes(initialStrokes);
        setBackdrop(null);
        setFigures(initialFigures);
        // Re-opening a sketch that has one figure lands on the figure tool
        // with that figure selected: the handles are the answer to "is this
        // still posable?", so they should be on screen before the first click.
        setSelectedId(initialFigures.length === 1 ? initialFigures[0].id : null);
        setTool(initialFigures.length > 0 ? 'pose' : 'pen');
        renderStrokes(ctx, initialStrokes, dims);
        if (initialImage) {
            loadDataUrl(initialImage)
                .then((image) => {
                    if (cancelled) return;
                    setBackdrop(image);
                    redraw(initialStrokes, image);
                })
                .catch(() => undefined);
        }
        return () => { cancelled = true; };
    }, [open, canvasEl, dims, initialImage, initialStrokes, initialFigures, getContext, paintBackground, redraw]);

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

    // The figures live on their own layer above the strokes. Handles are drawn
    // here and only here: the exported PNG is composited separately, so the
    // model never receives the blue joint dots.
    useEffect(() => {
        if (!open) return;
        const overlay = overlayEl;
        const ctx = overlay?.getContext('2d');
        if (!overlay || !ctx) return;
        overlay.width = dims.width;
        overlay.height = dims.height;
        ctx.clearRect(0, 0, dims.width, dims.height);
        for (const figure of figures) {
            drawFigure(ctx, figure, { selected: tool === 'pose' && figure.id === selectedId });
        }
        if (tool === 'pose' && selectedFigure) {
            drawFigureHandles(ctx, selectedFigure, toCanvasPx(7));
        }
    }, [open, overlayEl, dims, figures, selectedId, selectedFigure, tool, toCanvasPx]);

    const pushSnapshot = useCallback((snapshot: Partial<SketchSnapshot>) => {
        historyRef.current.push({
            strokes: snapshot.strokes ?? null,
            figures: snapshot.figures ?? null,
            backdrop: snapshot.backdrop ?? null,
            hadBackdrop: backdrop !== null,
        });
        setCanUndo(true);
    }, [backdrop]);

    const snapshotStrokes = useCallback(() => {
        pushSnapshot({ strokes });
    }, [pushSnapshot, strokes]);

    const snapshotFigures = useCallback(() => {
        pushSnapshot({ figures });
    }, [figures, pushSnapshot]);

    const handleUndo = useCallback(() => {
        const frame = historyRef.current.pop();
        if (!frame) return;
        if (frame.figures) {
            setFigures(frame.figures);
            setSelectedId((current) => (frame.figures?.some((figure) => figure.id === current) ? current : null));
        }
        if (frame.strokes || frame.hadBackdrop !== (backdrop !== null)) {
            const nextStrokes = frame.strokes ?? strokes;
            const nextBackdrop = frame.hadBackdrop ? (frame.backdrop ?? backdrop) : null;
            setStrokes(nextStrokes);
            setBackdrop(nextBackdrop);
            redraw(nextStrokes, nextBackdrop);
        }
        setCanUndo(historyRef.current.canUndo);
    }, [backdrop, redraw, strokes]);

    const handleClear = useCallback(() => {
        // Clearing takes the backdrop with it, so an emptied canvas is empty
        // rather than back to the picture it was opened on.
        pushSnapshot({ strokes, figures, backdrop });
        setStrokes([]);
        setBackdrop(null);
        setFigures([]);
        setSelectedId(null);
        redraw([], null);
    }, [backdrop, figures, pushSnapshot, redraw, strokes]);

    const updateFigure = useCallback((id: string, update: (figure: PoseFigure) => PoseFigure) => {
        setFigures((current) => current.map((figure) => (figure.id === id ? update(figure) : figure)));
    }, []);

    // Picking the tool with an empty canvas drops a figure straight onto the
    // surface: no pose picker standing between the user and the work
    // (principle 2). Poses are swapped afterwards, in place.
    const handleAddFigure = useCallback(() => {
        snapshotFigures();
        // Placed clear of the figures already down, and in the next tone, so
        // a second figure is visibly a second figure.
        const figure = createFigure('standing', dims, placeNewFigure(figures, dims), figures.length);
        setFigures((current) => [...current, figure]);
        setSelectedId(figure.id);
        setTool('pose');
    }, [dims, figures, snapshotFigures]);

    const handleRemoveFigure = useCallback(() => {
        if (!selectedFigure) return;
        snapshotFigures();
        setFigures((current) => current.filter((figure) => figure.id !== selectedFigure.id));
        setSelectedId(null);
    }, [selectedFigure, snapshotFigures]);

    const handleFlipFigure = useCallback(() => {
        if (!selectedFigure) return;
        snapshotFigures();
        updateFigure(selectedFigure.id, flipFigure);
    }, [selectedFigure, snapshotFigures, updateFigure]);

    const handlePreset = useCallback((preset: PosePresetKey) => {
        if (!selectedFigure) return;
        snapshotFigures();
        updateFigure(selectedFigure.id, (figure) => applyPreset(figure, preset, dims));
    }, [dims, selectedFigure, snapshotFigures, updateFigure]);

    const handleToolChange = useCallback((next: Tool) => {
        setTool(next);
        if (next !== 'pose') return;
        if (figures.length === 0) {
            handleAddFigure();
        } else if (!selectedId) {
            setSelectedId(figures[figures.length - 1].id);
        }
    }, [figures, handleAddFigure, selectedId]);

    useEffect(() => {
        if (!open) return;
        const onKeyDown = (event: KeyboardEvent) => {
            if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z' && !event.shiftKey) {
                event.preventDefault();
                handleUndo();
                return;
            }
            if ((event.key === 'Delete' || event.key === 'Backspace') && tool === 'pose' && selectedFigure) {
                const target = event.target as HTMLElement | null;
                // Never steal the key from a field the user is typing in.
                if (target && /^(INPUT|TEXTAREA)$/.test(target.tagName)) return;
                event.preventDefault();
                handleRemoveFigure();
            }
        };
        window.addEventListener('keydown', onKeyDown);
        return () => window.removeEventListener('keydown', onKeyDown);
    }, [open, handleUndo, handleRemoveFigure, selectedFigure, tool]);

    const pointFromEvent = useCallback((event: { clientX: number; clientY: number }) => {
        if (!canvasEl) return { x: 0, y: 0 };
        return toCanvasPoint(event.clientX, event.clientY, canvasEl.getBoundingClientRect(), dims);
    }, [canvasEl, dims]);

    const handlePointerDown = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        // Primary button / touch / pen only — a right-click is not a stroke.
        if (event.button !== 0 || drawingRef.current) return;
        const ctx = getContext();
        if (!ctx) return;
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        const point = pointFromEvent(event);
        const stroke: Stroke = {
            tool: tool === 'eraser' ? 'eraser' : 'pen',
            color,
            brush,
            points: [point],
        };
        applyStrokeStyle(ctx, stroke, dims);
        // A tap with no movement still leaves a dot.
        ctx.beginPath();
        ctx.moveTo(point.x, point.y);
        ctx.lineTo(point.x + 0.01, point.y + 0.01);
        ctx.stroke();
        drawingRef.current = { pointerId: event.pointerId, last: point, stroke };
    }, [brush, color, dims, getContext, pointFromEvent, tool]);

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
        applyStrokeStyle(ctx, drawing.stroke, dims);
        ctx.beginPath();
        ctx.moveTo(drawing.last.x, drawing.last.y);
        for (const point of points) ctx.lineTo(point.x, point.y);
        ctx.stroke();
        drawing.stroke.points.push(...points);
        drawing.last = points[points.length - 1];
    }, [dims, getContext, pointFromEvent]);

    const handlePointerEnd = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        const drawing = drawingRef.current;
        if (!drawing || drawing.pointerId !== event.pointerId) return;
        drawingRef.current = null;
        if (event.currentTarget.hasPointerCapture(event.pointerId)) {
            event.currentTarget.releasePointerCapture(event.pointerId);
        }
        // The stroke joins the list only now: one gesture is one undo step,
        // and the pixels already on screen match what a replay would produce.
        snapshotStrokes();
        setStrokes((current) => [...current, drawing.stroke]);
    }, [snapshotStrokes]);

    // Pose interactions read top-down (the last figure drawn is the one on
    // top): a joint under the pointer wins, then the selected figure's scale
    // grip, then the body itself, and an empty spot deselects.
    const handlePosePointerDown = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        if (event.button !== 0 || poseDragRef.current) return;
        event.preventDefault();
        const point = pointFromEvent(event);
        const jointRadius = toCanvasPx(14);

        if (selectedFigure && isScaleHandleHit(selectedFigure, point, toCanvasPx(14))) {
            const bounds = figureBounds(selectedFigure);
            const origin = { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
            const startDistance = Math.hypot(point.x - origin.x, point.y - origin.y);
            if (startDistance > 0) {
                event.currentTarget.setPointerCapture(event.pointerId);
                poseDragRef.current = {
                    pointerId: event.pointerId,
                    mode: 'scale',
                    figureId: selectedFigure.id,
                    before: figures,
                    committed: false,
                    origin,
                    startDistance,
                    start: selectedFigure,
                };
                return;
            }
        }

        for (let index = figures.length - 1; index >= 0; index -= 1) {
            const figure = figures[index];
            const joint = hitTestJoint(figure, point, jointRadius);
            if (joint) {
                event.currentTarget.setPointerCapture(event.pointerId);
                setSelectedId(figure.id);
                poseDragRef.current = {
                    pointerId: event.pointerId,
                    mode: 'joint',
                    figureId: figure.id,
                    before: figures,
                    committed: false,
                    joint,
                };
                return;
            }
        }

        // Clicking a pile walks down it rather than always grabbing the top
        // figure, which would bury everything under it.
        const body = nextFigureAt(figures, point, selectedId, toCanvasPx(4));
        if (body) {
            event.currentTarget.setPointerCapture(event.pointerId);
            setSelectedId(body.id);
            poseDragRef.current = {
                pointerId: event.pointerId,
                mode: 'move',
                figureId: body.id,
                before: figures,
                committed: false,
                last: point,
            };
            return;
        }

        setSelectedId(null);
    }, [figures, pointFromEvent, selectedFigure, selectedId, toCanvasPx]);

    const handlePosePointerMove = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        const drag = poseDragRef.current;
        if (!drag || drag.pointerId !== event.pointerId) return;
        event.preventDefault();
        if (!drag.committed) {
            drag.committed = true;
            pushSnapshot({ figures: drag.before });
        }
        const point = pointFromEvent(event);
        if (drag.mode === 'joint') {
            updateFigure(drag.figureId, (figure) => moveJoint(figure, drag.joint, point));
            return;
        }
        if (drag.mode === 'move') {
            const dx = point.x - drag.last.x;
            const dy = point.y - drag.last.y;
            drag.last = point;
            updateFigure(drag.figureId, (figure) => translateFigure(figure, dx, dy));
            return;
        }
        const distance = Math.hypot(point.x - drag.origin.x, point.y - drag.origin.y);
        const factor = distance / drag.startDistance;
        updateFigure(drag.figureId, () => scaleFigure(drag.start, factor, drag.origin));
    }, [pointFromEvent, pushSnapshot, updateFigure]);

    const handlePosePointerEnd = useCallback((event: React.PointerEvent<HTMLCanvasElement>) => {
        const drag = poseDragRef.current;
        if (!drag || drag.pointerId !== event.pointerId) return;
        poseDragRef.current = null;
        if (event.currentTarget.hasPointerCapture(event.pointerId)) {
            event.currentTarget.releasePointerCapture(event.pointerId);
        }
    }, []);

    const handleSubmit = useCallback(() => {
        const canvas = canvasEl;
        if (!canvas) return;
        // Composite onto a throwaway canvas so the live surface keeps its
        // layers: strokes stay strokes, figures stay posable.
        const output = document.createElement('canvas');
        output.width = dims.width;
        output.height = dims.height;
        const ctx = output.getContext('2d');
        if (!ctx) {
            showNotification(t('playground.sketch.failed', { defaultValue: 'Could not export the sketch' }), 'error');
            return;
        }
        ctx.drawImage(canvas, 0, 0);
        for (const figure of figures) drawFigure(ctx, figure);
        output.toBlob((blob) => {
            if (!blob) {
                showNotification(t('playground.sketch.failed', { defaultValue: 'Could not export the sketch' }), 'error');
                return;
            }
            onSubmit({
                file: new File([blob], `sketch-${Date.now()}.png`, { type: 'image/png' }),
                previewUrl: output.toDataURL('image/png'),
                // The layers travel with the result, as data rather than
                // pixels: this is what makes a saved sketch re-editable.
                layers: { strokes, figures, backdrop: initialImage },
            });
        }, 'image/png');
    }, [canvasEl, dims, figures, initialImage, onSubmit, showNotification, strokes, t]);

    const toolLabel = (key: Tool) => {
        if (key === 'pen') return t('playground.sketch.pen', { defaultValue: 'Pen' });
        if (key === 'eraser') return t('playground.sketch.eraser', { defaultValue: 'Eraser' });
        return t('playground.sketch.pose.tool', { defaultValue: 'Figure' });
    };
    const brushLabel = (key: BrushSizeKey) => t(`playground.sketch.brush.${key}`, {
        defaultValue: key === 'thin' ? 'Thin' : key === 'thick' ? 'Thick' : 'Medium',
    });
    const presetLabel = (key: PosePresetKey) => t(`playground.sketch.pose.preset.${key}`, {
        defaultValue: key === 'standing' ? 'Standing'
            : key === 'walking' ? 'Walking'
                : key === 'sitting' ? 'Sitting' : 'Arms up',
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
                            onChange={(_, next: Tool | null) => { if (next) handleToolChange(next); }}
                            aria-label={t('playground.sketch.tool', { defaultValue: 'Tool' })}
                        >
                            <ToggleButton value="pen" aria-label={toolLabel('pen')}>
                                <Tooltip title={toolLabel('pen')}><Create fontSize="small" /></Tooltip>
                            </ToggleButton>
                            <ToggleButton value="eraser" aria-label={toolLabel('eraser')}>
                                <Tooltip title={toolLabel('eraser')}><Eraser fontSize="small" /></Tooltip>
                            </ToggleButton>
                            <ToggleButton value="pose" aria-label={toolLabel('pose')}>
                                <Tooltip title={toolLabel('pose')}><Accessibility fontSize="small" /></Tooltip>
                            </ToggleButton>
                        </ToggleButtonGroup>

                        {tool === 'pose' ? (
                            <>
                                <Tooltip title={t('playground.sketch.pose.add', { defaultValue: 'Add figure' })}>
                                    <IconButton
                                        size="small"
                                        onClick={handleAddFigure}
                                        aria-label={t('playground.sketch.pose.add', { defaultValue: 'Add figure' })}
                                    >
                                        <Add fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                                {selectedFigure ? (
                                    <>
                                        <Stack
                                            direction="row"
                                            spacing={0.5}
                                            useFlexGap
                                            sx={{ flexWrap: 'wrap', alignItems: 'center' }}
                                            aria-label={t('playground.sketch.pose.presets', { defaultValue: 'Pose' })}
                                        >
                                            {PRESET_KEYS.map((preset) => (
                                                <Button
                                                    key={preset}
                                                    size="small"
                                                    variant="outlined"
                                                    color="inherit"
                                                    onClick={() => handlePreset(preset)}
                                                    sx={{ textTransform: 'none', py: 0.1, px: 0.9, minWidth: 0, color: 'text.secondary' }}
                                                >
                                                    {presetLabel(preset)}
                                                </Button>
                                            ))}
                                        </Stack>
                                        <Tooltip title={t('playground.sketch.pose.flip', { defaultValue: 'Mirror figure' })}>
                                            <IconButton
                                                size="small"
                                                onClick={handleFlipFigure}
                                                aria-label={t('playground.sketch.pose.flip', { defaultValue: 'Mirror figure' })}
                                            >
                                                <Flip fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title={t('playground.sketch.pose.remove', { defaultValue: 'Remove figure' })}>
                                            <IconButton
                                                size="small"
                                                onClick={handleRemoveFigure}
                                                aria-label={t('playground.sketch.pose.remove', { defaultValue: 'Remove figure' })}
                                            >
                                                <Delete fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    </>
                                ) : null}
                            </>
                        ) : (
                            <>
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
                            </>
                        )}

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
                            sx={{ position: 'relative', lineHeight: 0 }}
                            style={{ width: cssSize.width, height: cssSize.height }}
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
                            {/* Figure layer. Transparent to pointers unless the
                                figure tool is active, so drawing over a pose
                                needs no mode dance. */}
                            <Box
                                component="canvas"
                                ref={setOverlayEl}
                                aria-hidden
                                onPointerDown={handlePosePointerDown}
                                onPointerMove={handlePosePointerMove}
                                onPointerUp={handlePosePointerEnd}
                                onPointerCancel={handlePosePointerEnd}
                                onContextMenu={(event: React.MouseEvent) => event.preventDefault()}
                                style={{ width: cssSize.width, height: cssSize.height }}
                                sx={{
                                    position: 'absolute',
                                    top: 0,
                                    left: 0,
                                    display: 'block',
                                    borderRadius: 1,
                                    touchAction: 'none',
                                    userSelect: 'none',
                                    pointerEvents: tool === 'pose' ? 'auto' : 'none',
                                    cursor: tool === 'pose' ? 'move' : 'crosshair',
                                }}
                            />
                        </Box>
                    </Box>

                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {figures.length > 0
                            ? t('playground.sketch.pose.hint', {
                                defaultValue: 'Drag the joints to pose the figure, the body to move it, the corner to resize. The grey mannequin is a pose reference — the prompt says who it is.',
                            })
                            : t('playground.sketch.hint', {
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
