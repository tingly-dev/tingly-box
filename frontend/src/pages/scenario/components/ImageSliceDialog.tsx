import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Box,
    Button,
    Checkbox,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    FormControlLabel,
    IconButton,
    InputLabel,
    MenuItem,
    Select,
    Slider,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add, Close, Download, Gif, GridView, Pause, PlayArrow, Remove } from '@/components/icons';
import { createZipBlob } from '@/utils/zip';
import { downloadBlob, slugify } from '@/utils/download';
import { encodeGif } from '@/utils/gif';
import { DEFAULT_TOLERANCE, type BackgroundKind } from '@/utils/imageMatte';
import {
    analyzeSheetBackground,
    computeTileRects,
    DEFAULT_FRAME_DELAY,
    DEFAULT_GRID,
    FRAME_DELAYS,
    FULL_CROP,
    GUTTER_MAX,
    isFullCrop,
    loadImage,
    normalizeCrop,
    renderAnimationFrames,
    renderTile,
    renderTileDataUrl,
    tileFileName,
    type CropRect,
    type MatteSpec,
    type TileRect,
} from '@/utils/imageSlice';

// Rows and columns are a number you nudge while watching the overlay, so each
// is a stepper with a typable field rather than a dropdown that hides the
// image behind a menu on every change. The cap is generous enough for a
// spritesheet; 3x3 is the shape a sticker sheet almost always comes back as.
const AXIS_MAX = 12;
const clampAxis = (value: number): number => Math.min(AXIS_MAX, Math.max(1, Math.round(value) || 1));

interface AxisStepperProps {
    label: string;
    value: number;
    onChange: (value: number) => void;
    decreaseLabel: string;
    increaseLabel: string;
}

const AxisStepper: React.FC<AxisStepperProps> = ({ label, value, onChange, decreaseLabel, increaseLabel }) => (
    <Stack direction="row" sx={{ alignItems: 'flex-start', flex: 1, minWidth: 0, '& > .MuiIconButton-root': { mt: 0.25 } }}>
        <IconButton
            size="small"
            onClick={() => onChange(clampAxis(value - 1))}
            disabled={value <= 1}
            aria-label={decreaseLabel}
        >
            <Remove fontSize="small" />
        </IconButton>
        <TextField
            size="small"
            helperText={label}
            value={value}
            onChange={(event) => {
                const next = Number(event.target.value);
                if (Number.isFinite(next) && next >= 1) onChange(clampAxis(next));
            }}
            onKeyDown={(event) => {
                if (event.key === 'ArrowUp') { event.preventDefault(); onChange(clampAxis(value + 1)); }
                if (event.key === 'ArrowDown') { event.preventDefault(); onChange(clampAxis(value - 1)); }
            }}
            slotProps={{
                htmlInput: { 'aria-label': label, inputMode: 'numeric', min: 1, max: AXIS_MAX, style: { textAlign: 'center', padding: '6px 4px' } },
                formHelperText: { sx: { mx: 0, textAlign: 'center', mt: 0.25, lineHeight: 1.2 } },
            }}
            sx={{ width: 52, '& .MuiInputBase-root': { px: 0.5 } }}
        />
        <IconButton
            size="small"
            onClick={() => onChange(clampAxis(value + 1))}
            disabled={value >= AXIS_MAX}
            aria-label={increaseLabel}
        >
            <Add fontSize="small" />
        </IconButton>
    </Stack>
);

// The conventional "transparent here" checkerboard.
const CHECKERBOARD_IMAGE = [
    'linear-gradient(45deg, rgba(128,128,128,0.18) 25%, transparent 25%)',
    'linear-gradient(-45deg, rgba(128,128,128,0.18) 25%, transparent 25%)',
    'linear-gradient(45deg, transparent 75%, rgba(128,128,128,0.18) 75%)',
    'linear-gradient(-45deg, transparent 75%, rgba(128,128,128,0.18) 75%)',
].join(', ');
const EXPORT_SIZES = [512, 256];

// The animation preview is a thumbnail strip played in place: small enough
// that re-rendering every frame on each knob change stays instant.
const PREVIEW_BOX = { width: 128, height: 128 };
// A GIF of nine 1024px tiles is tens of megabytes; nothing about a sticker
// animation needs that, and the cap is invisible to the user in practice.
const GIF_MAX_SIZE = 480;

// Below this, a pointer gesture was a click on a tile rather than a drag that
// redraws the frame — the two share the same surface on purpose.
const DRAG_THRESHOLD = 0.01;
// How far one arrow key nudges a frame corner, as a fraction of the image.
const NUDGE = 0.01;
const NUDGE_COARSE = 0.05;

// Which part of the frame a pointer grabbed. There is only ever one frame —
// it starts as the whole image and is moved and resized from there, never
// redrawn — so a press inside it moves it and a press on an edge or corner
// resizes that side.
type DragMode = 'move' | 'n' | 's' | 'e' | 'w' | 'nw' | 'ne' | 'se' | 'sw';

// Thickness of the invisible strip along each frame edge that drags it.
const EDGE_GRAB = 12;
const percent = (value: number): string => `${value * 100}%`;
// Handles sit *inside* the frame rather than straddling its border: the
// preview clips to the image (the dimming outside the frame is that clip), so
// a handle that overhangs the image edge loses half its hit area exactly where
// the frame starts out — at the image's own corners.
const INSET_START = 'translate(0, 0)';

// The eight grab targets of the frame: four edges (invisible strips, sized to
// be hittable) and four corners (visible squares, which are also the keyboard
// entry point). Positions are computed from the frame so there is one source
// of truth for where an edge is.
interface FrameHandle {
    mode: Exclude<DragMode, 'new'>;
    corner: boolean;
    cursor: string;
    labelKey: string;
    label: string;
    position: (crop: { x: number; y: number; width: number; height: number }) => React.CSSProperties;
}

const FRAME_HANDLES: FrameHandle[] = [
    {
        mode: 'n', corner: false, cursor: 'ns-resize',
        labelKey: 'playground.slice.frameTop', label: 'Drag the top edge of the frame',
        position: (crop) => ({ left: percent(crop.x), top: percent(crop.y), width: percent(crop.width), height: EDGE_GRAB }),
    },
    {
        mode: 's', corner: false, cursor: 'ns-resize',
        labelKey: 'playground.slice.frameBottom', label: 'Drag the bottom edge of the frame',
        position: (crop) => ({ left: percent(crop.x), top: percent(crop.y + crop.height), width: percent(crop.width), height: EDGE_GRAB, transform: 'translateY(-100%)' }),
    },
    {
        mode: 'w', corner: false, cursor: 'ew-resize',
        labelKey: 'playground.slice.frameLeft', label: 'Drag the left edge of the frame',
        position: (crop) => ({ left: percent(crop.x), top: percent(crop.y), width: EDGE_GRAB, height: percent(crop.height) }),
    },
    {
        mode: 'e', corner: false, cursor: 'ew-resize',
        labelKey: 'playground.slice.frameRight', label: 'Drag the right edge of the frame',
        position: (crop) => ({ left: percent(crop.x + crop.width), top: percent(crop.y), width: EDGE_GRAB, height: percent(crop.height), transform: 'translateX(-100%)' }),
    },
    {
        mode: 'nw', corner: true, cursor: 'nwse-resize',
        labelKey: 'playground.slice.frameTopLeft', label: 'Top-left corner of the frame',
        position: (crop) => ({ left: percent(crop.x), top: percent(crop.y), transform: INSET_START }),
    },
    {
        mode: 'ne', corner: true, cursor: 'nesw-resize',
        labelKey: 'playground.slice.frameTopRight', label: 'Top-right corner of the frame',
        position: (crop) => ({ left: percent(crop.x + crop.width), top: percent(crop.y), transform: 'translate(-100%, 0)' }),
    },
    {
        mode: 'sw', corner: true, cursor: 'nesw-resize',
        labelKey: 'playground.slice.frameBottomLeft', label: 'Bottom-left corner of the frame',
        position: (crop) => ({ left: percent(crop.x), top: percent(crop.y + crop.height), transform: 'translate(0, -100%)' }),
    },
    {
        mode: 'se', corner: true, cursor: 'nwse-resize',
        labelKey: 'playground.slice.frameBottomRight', label: 'Bottom-right corner of the frame',
        position: (crop) => ({ left: percent(crop.x + crop.width), top: percent(crop.y + crop.height), transform: 'translate(-100%, -100%)' }),
    },
];

const clampFraction = (value: number): number => Math.min(1, Math.max(0, value));

// Applies one drag to the frame the gesture started from. Edges move
// independently, corners move two at once, and 'move' slides the whole frame
// without resizing it — so its size survives being repositioned.
const applyDrag = (mode: DragMode, origin: CropRect, startX: number, startY: number, x: number, y: number): CropRect => {
    if (mode === 'move') {
        return {
            x: Math.min(Math.max(origin.x + (x - startX), 0), 1 - origin.width),
            y: Math.min(Math.max(origin.y + (y - startY), 0), 1 - origin.height),
            width: origin.width,
            height: origin.height,
        };
    }
    let { x: left, y: top, width, height } = origin;
    const right = left + width;
    const bottom = top + height;
    if (mode.includes('w')) { left = x; width = right - x; }
    if (mode.includes('e')) { width = x - left; }
    if (mode.includes('n')) { top = y; height = bottom - y; }
    if (mode.includes('s')) { height = y - top; }
    return normalizeCrop({ x: left, y: top, width, height });
};

type LoadState =
    | { status: 'idle' | 'loading' | 'error' }
    | { status: 'ready'; image: HTMLImageElement };

interface ImageSliceDialogProps {
    open: boolean;
    src: string | null;
    prompt: string;
    onClose: () => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const ImageSliceDialog: React.FC<ImageSliceDialogProps> = ({
    open,
    src,
    prompt,
    onClose,
    showNotification,
}) => {
    const { t } = useTranslation();
    // One cell for the load, not three: "decoded", "still loading" and "failed"
    // are outcomes of the same fetch, and keeping them apart lets impossible
    // combinations exist that the render then has to guard against.
    const [load, setLoad] = useState<LoadState>({ status: 'idle' });
    const image = load.status === 'ready' ? load.image : null;
    const [rows, setRows] = useState(DEFAULT_GRID.rows);
    const [cols, setCols] = useState(DEFAULT_GRID.cols);
    const [crop, setCrop] = useState<CropRect>(DEFAULT_GRID.crop);
    const [gutter, setGutter] = useState(DEFAULT_GRID.gutter);
    const [exportSize, setExportSize] = useState<number | null>(null);
    const [excluded, setExcluded] = useState<Set<number>>(new Set());
    const [working, setWorking] = useState(false);
    // Background cleanup: what the sheet was found to have, whether the user
    // wants it gone, and how far into the fringe the key reaches.
    const [detected, setDetected] = useState<BackgroundKind>('none');
    const [detectedColors, setDetectedColors] = useState<[number, number, number][]>([]);
    const [cleanKind, setCleanKind] = useState<BackgroundKind>('none');
    const [tolerance, setTolerance] = useState(DEFAULT_TOLERANCE);
    const [frameDelay, setFrameDelay] = useState(DEFAULT_FRAME_DELAY);
    const [playing, setPlaying] = useState(true);
    const [frame, setFrame] = useState(0);

    // Start from a clean grid whenever a new image is opened — the dialog is a
    // per-image work surface, not a sticky global setting.
    useEffect(() => {
        if (!open) return;
        setRows(DEFAULT_GRID.rows);
        setCols(DEFAULT_GRID.cols);
        setCrop(DEFAULT_GRID.crop);
        setGutter(DEFAULT_GRID.gutter);
        setExportSize(null);
        setExcluded(new Set());
        setTolerance(DEFAULT_TOLERANCE);
        setFrameDelay(DEFAULT_FRAME_DELAY);
        setPlaying(true);
    }, [open, src]);

    // An exclusion names a tile of one particular grid; re-cutting the image
    // renumbers every tile, so carrying the old indices over would silently
    // drop unrelated pieces from the download.
    useEffect(() => {
        setExcluded(new Set());
    }, [rows, cols]);

    useEffect(() => {
        if (!open || !src) return;
        let released: (() => void) | null = null;
        let cancelled = false;
        setLoad({ status: 'loading' });
        loadImage(src)
            .then(({ image: loaded, release }) => {
                if (cancelled) {
                    release();
                    return;
                }
                released = release;
                setLoad({ status: 'ready', image: loaded });
            })
            .catch(() => {
                if (!cancelled) setLoad({ status: 'error' });
            });
        return () => {
            cancelled = true;
            // Drop the decoded image with the object URL it was decoded from,
            // or a reopen paints one frame against a revoked blob URL.
            setLoad({ status: 'idle' });
            released?.();
        };
    }, [open, src]);

    // What kind of fake background this sheet has is a property of the image,
    // so it is read once per image rather than being a question put to the
    // user. The checkbox below then only has to say yes or no.
    useEffect(() => {
        if (!image) {
            setDetected('none');
            setDetectedColors([]);
            setCleanKind('none');
            return;
        }
        try {
            const analysis = analyzeSheetBackground(image);
            setDetected(analysis.kind);
            setDetectedColors(analysis.colors);
            setCleanKind(analysis.kind);
        } catch {
            setDetected('none');
            setDetectedColors([]);
            setCleanKind('none');
        }
    }, [image]);

    const matte = useMemo<MatteSpec | null>(
        () => (cleanKind === 'none' ? null : { kind: cleanKind, tolerance, colors: detectedColors }),
        [cleanKind, detectedColors, tolerance],
    );

    const rects = useMemo(
        () => (image
            ? computeTileRects(image.naturalWidth, image.naturalHeight, { rows, cols, crop, gutter })
            : []),
        [image, rows, cols, crop, gutter],
    );
    const selectedRects = useMemo(() => rects.filter((rect) => !excluded.has(rect.index)), [rects, excluded]);

    // The frame is dragged on the image itself rather than dialled in on a
    // slider: what "the useful part of this sheet" means is something the user
    // can only point at, and a symmetric margin cannot express an off-centre
    // region at all.
    const surfaceRef = useRef<HTMLDivElement | null>(null);
    const dragRef = useRef<{ mode: DragMode; startX: number; startY: number; origin: CropRect; moved: boolean } | null>(null);
    // A drag that redrew the frame must not also toggle the tile it ended on.
    const suppressClickRef = useRef(false);

    const beginDrag = useCallback((mode: DragMode, event: React.PointerEvent) => {
        const box = surfaceRef.current?.getBoundingClientRect();
        if (!box || box.width === 0 || box.height === 0) return;
        event.preventDefault();
        const toFraction = (clientX: number, clientY: number) => ({
            x: clampFraction((clientX - box.left) / box.width),
            y: clampFraction((clientY - box.top) / box.height),
        });
        const start = toFraction(event.clientX, event.clientY);
        dragRef.current = { mode, startX: start.x, startY: start.y, origin: crop, moved: false };

        const handleMove = (moveEvent: PointerEvent) => {
            const drag = dragRef.current;
            if (!drag) return;
            const point = toFraction(moveEvent.clientX, moveEvent.clientY);
            if (!drag.moved
                && Math.abs(point.x - drag.startX) < DRAG_THRESHOLD
                && Math.abs(point.y - drag.startY) < DRAG_THRESHOLD) return;
            drag.moved = true;
            setCrop(applyDrag(drag.mode, drag.origin, drag.startX, drag.startY, point.x, point.y));
        };
        const handleUp = () => {
            window.removeEventListener('pointermove', handleMove);
            window.removeEventListener('pointerup', handleUp);
            suppressClickRef.current = dragRef.current?.moved ?? false;
            dragRef.current = null;
        };
        window.addEventListener('pointermove', handleMove);
        window.addEventListener('pointerup', handleUp);
    }, [crop]);

    // Keyboard equivalent of dragging a corner, so the frame is reachable
    // without a pointer (the slider it replaced was).
    const nudgeCorner = useCallback((mode: DragMode, event: React.KeyboardEvent) => {
        const step = event.shiftKey ? NUDGE_COARSE : NUDGE;
        const delta = { x: 0, y: 0 };
        if (event.key === 'ArrowLeft') delta.x = -step;
        else if (event.key === 'ArrowRight') delta.x = step;
        else if (event.key === 'ArrowUp') delta.y = -step;
        else if (event.key === 'ArrowDown') delta.y = step;
        else return;
        event.preventDefault();
        setCrop((current) => {
            const cornerX = mode.includes('w') ? current.x : current.x + current.width;
            const cornerY = mode.includes('n') ? current.y : current.y + current.height;
            return applyDrag(
                mode,
                current,
                cornerX,
                cornerY,
                clampFraction(cornerX + delta.x),
                clampFraction(cornerY + delta.y),
            );
        });
    }, []);

    // Frames of the animation, in reading order — the order the tiles were
    // cut in is the order a sheet is meant to be read, so there is no separate
    // sequencing step to get wrong.
    const previewFrames = useMemo(() => {
        if (!image || selectedRects.length === 0) return [];
        try {
            return selectedRects.map((rect) => ({
                index: rect.index,
                url: renderTileDataUrl(image, rect, { box: PREVIEW_BOX, matte }),
            }));
        } catch {
            return [];
        }
    }, [image, matte, selectedRects]);

    // The same rendered tiles, addressed by grid position: with cleanup on
    // they are laid back over the sheet, because a checkbox whose effect is
    // only visible in the exported file is a checkbox the user has to guess at.
    const previewByIndex = useMemo(
        () => new Map(previewFrames.map((entry) => [entry.index, entry.url])),
        [previewFrames],
    );

    useEffect(() => {
        setFrame(0);
    }, [previewFrames.length]);

    useEffect(() => {
        if (!playing || previewFrames.length < 2) return;
        const timer = window.setInterval(
            () => setFrame((current) => (current + 1) % previewFrames.length),
            frameDelay,
        );
        return () => window.clearInterval(timer);
    }, [frameDelay, playing, previewFrames.length]);

    const toggleTile = useCallback((index: number) => {
        if (suppressClickRef.current) {
            suppressClickRef.current = false;
            return;
        }
        setExcluded((current) => {
            const next = new Set(current);
            if (next.has(index)) next.delete(index);
            else next.add(index);
            return next;
        });
    }, []);

    const stem = useMemo(() => slugify(prompt), [prompt]);

    // One action that adapts: a single selected tile is worth a bare PNG, and
    // anything more is worth an archive. Tiles keep their grid number either
    // way, so excluding one never renumbers the rest.
    const handleDownload = useCallback(async () => {
        if (!image || selectedRects.length === 0) return;
        setWorking(true);
        try {
            const nameOf = (rect: TileRect) => tileFileName(stem, rect.index, rects.length);
            const options = { exportSize, matte };
            if (selectedRects.length === 1) {
                downloadBlob(await renderTile(image, selectedRects[0], options), nameOf(selectedRects[0]));
                return;
            }
            const entries = await Promise.all(selectedRects.map(async (rect) => ({
                name: nameOf(rect),
                data: new Uint8Array(await (await renderTile(image, rect, options)).arrayBuffer()),
            })));
            downloadBlob(createZipBlob(entries), `${stem}-${selectedRects.length}.zip`);
        } catch {
            showNotification(
                t('playground.slice.failed', { defaultValue: 'Could not slice this image' }),
                'error',
            );
        } finally {
            setWorking(false);
        }
    }, [exportSize, image, matte, rects.length, selectedRects, showNotification, stem, t]);

    // The same tiles, in the same order, handed over as one animation instead
    // of a folder the user would have to assemble somewhere else.
    const handleDownloadGif = useCallback(async () => {
        if (!image || selectedRects.length < 2) return;
        setWorking(true);
        try {
            const longest = Math.max(selectedRects[0].width, selectedRects[0].height);
            const size = Math.min(exportSize ?? longest, GIF_MAX_SIZE);
            const { width, height, frames } = renderAnimationFrames(image, selectedRects, {
                exportSize: size,
                matte,
            });
            downloadBlob(
                encodeGif({ width, height, frames, delayMs: frameDelay }),
                `${stem}-${selectedRects.length}.gif`,
            );
        } catch {
            showNotification(
                t('playground.slice.gifFailed', { defaultValue: 'Could not build the animation' }),
                'error',
            );
        } finally {
            setWorking(false);
        }
    }, [exportSize, frameDelay, image, matte, selectedRects, showNotification, stem, t]);

    const naturalWidth = image?.naturalWidth ?? 1;
    const naturalHeight = image?.naturalHeight ?? 1;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 1 }}>
                <GridView fontSize="small" />
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="h6" component="span" sx={{ display: 'block', fontSize: '1.05rem' }}>
                        {t('playground.slice.title', { defaultValue: 'Split into tiles' })}
                    </Typography>
                    <Typography
                        variant="caption"
                        sx={{ display: 'block', color: 'text.secondary', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                    >
                        {prompt}
                    </Typography>
                </Box>
                <IconButton
                    onClick={onClose}
                    aria-label={t('playground.slice.close', { defaultValue: 'Close slicer' })}
                >
                    <Close />
                </IconButton>
            </DialogTitle>
            <DialogContent dividers>
                <Box
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: { xs: '1fr', md: 'minmax(0, 1fr) 260px' },
                        gap: 3,
                        alignItems: 'start',
                    }}
                >
                    <Box
                        sx={{
                            position: 'relative',
                            minHeight: 240,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            borderRadius: 2,
                            bgcolor: 'action.hover',
                            p: 1,
                        }}
                    >
                        {load.status === 'loading' && <CircularProgress size={28} />}
                        {load.status === 'error' && (
                            <Typography variant="body2" sx={{ color: 'text.secondary', textAlign: 'center', p: 3 }}>
                                {t('playground.slice.loadFailed', {
                                    defaultValue: 'This image could not be read for slicing. Providers that return a remote URL may block browser access to their pixels.',
                                })}
                            </Typography>
                        )}
                        {image && (
                            <Box
                                ref={surfaceRef}
                                onPointerDown={(event) => {
                                    const box = surfaceRef.current?.getBoundingClientRect();
                                    if (!box) return;
                                    const x = (event.clientX - box.left) / box.width;
                                    const y = (event.clientY - box.top) / box.height;
                                    const inside = x >= crop.x && x <= crop.x + crop.width
                                        && y >= crop.y && y <= crop.y + crop.height;
                                    if (inside) beginDrag('move', event);
                                }}
                                sx={{
                                    position: 'relative',
                                    display: 'inline-block',
                                    maxWidth: '100%',
                                    overflow: 'hidden',
                                    touchAction: 'none',
                                    cursor: 'move',
                                    // Scoped to exactly the image's own box: a
                                    // checkerboard reads as "this part is
                                    // transparent", so any of it visible outside
                                    // the artwork is a lie about the artwork.
                                    // An opaque image covers it completely.
                                    backgroundImage: CHECKERBOARD_IMAGE,
                                    backgroundSize: '16px 16px',
                                    backgroundPosition: '0 0, 0 8px, 8px -8px, -8px 0',
                                }}
                            >
                                <Box
                                    component="img"
                                    src={image.src}
                                    alt={t('playground.slice.sheetAlt', { defaultValue: 'Image being sliced' })}
                                    // Dimmed rather than hidden while cleaning:
                                    // the margin and gap sliders still need the
                                    // whole sheet to aim at.
                                    sx={{ display: 'block', maxWidth: '100%', maxHeight: '60vh', opacity: matte ? 0.15 : 1 }}
                                />
                                {/* Everything outside the frame is dimmed by the
                                    frame's own huge spread shadow — one element
                                    instead of four filler rectangles to keep in
                                    sync. */}
                                <Box
                                    style={{
                                        left: `${crop.x * 100}%`,
                                        top: `${crop.y * 100}%`,
                                        width: `${crop.width * 100}%`,
                                        height: `${crop.height * 100}%`,
                                    }}
                                    sx={{
                                        position: 'absolute',
                                        boxShadow: '0 0 0 9999px rgba(15, 23, 42, 0.55)',
                                        border: '1px solid rgba(255, 255, 255, 0.9)',
                                        boxSizing: 'border-box',
                                        pointerEvents: 'none',
                                    }}
                                />
                                {FRAME_HANDLES.map((handle) => (
                                    <Box
                                        key={handle.mode}
                                        role="button"
                                        tabIndex={handle.corner ? 0 : -1}
                                        aria-label={t(handle.labelKey, { defaultValue: handle.label })}
                                        onPointerDown={(event) => {
                                            event.stopPropagation();
                                            beginDrag(handle.mode, event);
                                        }}
                                        onKeyDown={(event) => {
                                            if (handle.corner) nudgeCorner(handle.mode, event);
                                        }}
                                        style={handle.position(crop)}
                                        sx={{
                                            position: 'absolute',
                                            boxSizing: 'border-box',
                                            cursor: handle.cursor,
                                            // Above the tile overlays: they are
                                            // painted later and would otherwise
                                            // swallow every grab of the frame.
                                            zIndex: 2,
                                            ...(handle.corner
                                                ? {
                                                    width: 14,
                                                    height: 14,
                                                    bgcolor: 'common.white',
                                                    border: '2px solid',
                                                    borderColor: 'primary.main',
                                                    borderRadius: '3px',
                                                    '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.light' },
                                                }
                                                : {
                                                    bgcolor: 'transparent',
                                                    // A pill at the strip's midpoint says "this edge
                                                    // drags too"; the strip itself stays invisible
                                                    // because it is a hit area, not a shape.
                                                    '&::after': {
                                                        content: '""',
                                                        position: 'absolute',
                                                        left: '50%',
                                                        top: '50%',
                                                        transform: 'translate(-50%, -50%)',
                                                        width: handle.mode === 'n' || handle.mode === 's' ? 22 : 5,
                                                        height: handle.mode === 'n' || handle.mode === 's' ? 5 : 22,
                                                        borderRadius: 3,
                                                        bgcolor: 'common.white',
                                                        border: '1px solid',
                                                        borderColor: 'primary.main',
                                                        boxSizing: 'border-box',
                                                    },
                                                }),
                                        }}
                                    />
                                ))}
                                {rects.map((rect) => {
                                    const isExcluded = excluded.has(rect.index);
                                    return (
                                        <Box
                                            key={rect.index}
                                            role="checkbox"
                                            aria-checked={!isExcluded}
                                            tabIndex={0}
                                            onClick={() => toggleTile(rect.index)}
                                            onKeyDown={(event) => {
                                                if (event.key === 'Enter' || event.key === ' ') {
                                                    event.preventDefault();
                                                    toggleTile(rect.index);
                                                }
                                            }}
                                            aria-label={t('playground.slice.tile', {
                                                defaultValue: 'Tile {{number}}',
                                                number: rect.index + 1,
                                            })}
                                            style={{
                                                left: `${(rect.x / naturalWidth) * 100}%`,
                                                top: `${(rect.y / naturalHeight) * 100}%`,
                                                width: `${(rect.width / naturalWidth) * 100}%`,
                                                height: `${(rect.height / naturalHeight) * 100}%`,
                                            }}
                                            sx={{
                                                position: 'absolute',
                                                border: '2px solid',
                                                borderColor: isExcluded ? 'transparent' : 'primary.main',
                                                bgcolor: isExcluded ? 'rgba(15, 23, 42, 0.55)' : 'transparent',
                                                boxSizing: 'border-box',
                                                cursor: 'pointer',
                                                transition: 'background-color 0.12s ease-out',
                                                '&:hover': { bgcolor: isExcluded ? 'rgba(15, 23, 42, 0.42)' : 'rgba(25, 118, 210, 0.16)' },
                                            }}
                                        >
                                            {matte && !isExcluded && previewByIndex.has(rect.index) && (
                                                <Box
                                                    component="img"
                                                    src={previewByIndex.get(rect.index)}
                                                    alt=""
                                                    sx={{
                                                        display: 'block',
                                                        width: '100%',
                                                        height: '100%',
                                                        objectFit: 'fill',
                                                        pointerEvents: 'none',
                                                    }}
                                                />
                                            )}
                                        </Box>
                                    );
                                })}
                            </Box>
                        )}
                    </Box>

                    <Stack spacing={2}>
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {t('playground.slice.hint', {
                                defaultValue: 'Cuts an evenly divided grid — a sticker sheet, a contact sheet, a spritesheet. Drag the frame to move it and its corners to resize it, adjust the gap until the outlines sit on the artwork, then click a tile to leave it out.',
                            })}
                        </Typography>

                        <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                            <AxisStepper
                                label={t('playground.slice.rows', { defaultValue: 'Rows' })}
                                value={rows}
                                onChange={setRows}
                                decreaseLabel={t('playground.slice.fewerRows', { defaultValue: 'Fewer rows' })}
                                increaseLabel={t('playground.slice.moreRows', { defaultValue: 'More rows' })}
                            />
                            <Typography variant="body2" sx={{ color: 'text.disabled', mt: 1 }}>×</Typography>
                            <AxisStepper
                                label={t('playground.slice.cols', { defaultValue: 'Columns' })}
                                value={cols}
                                onChange={setCols}
                                decreaseLabel={t('playground.slice.fewerCols', { defaultValue: 'Fewer columns' })}
                                increaseLabel={t('playground.slice.moreCols', { defaultValue: 'More columns' })}
                            />
                        </Stack>

                        {/* The frame is dragged on the image; this side only
                            reports where it landed, in the pixels of the source
                            image, and offers the way back to the whole sheet. */}
                        <Box>
                            <Stack direction="row" spacing={1} sx={{ alignItems: 'baseline', justifyContent: 'space-between' }}>
                                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                    {t('playground.slice.frame', { defaultValue: 'Frame' })}
                                </Typography>
                                <Button
                                    size="small"
                                    disabled={isFullCrop(crop)}
                                    onClick={() => setCrop(FULL_CROP)}
                                    sx={{ minWidth: 0, px: 0.75, py: 0, fontSize: '0.72rem' }}
                                >
                                    {t('playground.slice.frameReset', { defaultValue: 'Whole image' })}
                                </Button>
                            </Stack>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.78rem' }}>
                                {t('playground.slice.frameValue', {
                                    defaultValue: '{{width}}×{{height}} px at {{x}},{{y}}',
                                    width: Math.round(crop.width * naturalWidth),
                                    height: Math.round(crop.height * naturalHeight),
                                    x: Math.round(crop.x * naturalWidth),
                                    y: Math.round(crop.y * naturalHeight),
                                })}
                            </Typography>
                        </Box>
                        <Box>
                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                {t('playground.slice.gutter', { defaultValue: 'Gap between tiles' })} · {Math.round(gutter * 100)}%
                            </Typography>
                            <Slider
                                size="small"
                                value={gutter}
                                min={0}
                                max={GUTTER_MAX}
                                step={0.01}
                                onChange={(_, value) => setGutter(value as number)}
                                aria-label={t('playground.slice.gutter', { defaultValue: 'Gap between tiles' })}
                            />
                        </Box>

                        <FormControl size="small" fullWidth>
                            <InputLabel id="slice-export-label">
                                {t('playground.slice.exportSize', { defaultValue: 'Output size' })}
                            </InputLabel>
                            <Select
                                labelId="slice-export-label"
                                label={t('playground.slice.exportSize', { defaultValue: 'Output size' })}
                                value={exportSize ?? 'original'}
                                onChange={(event) => {
                                    const value = event.target.value;
                                    setExportSize(value === 'original' ? null : Number(value));
                                }}
                            >
                                <MenuItem value="original">
                                    {image
                                        ? t('playground.slice.exportOriginalWithSize', {
                                            defaultValue: 'Original · {{width}}×{{height}} px',
                                            width: Math.round(rects[0]?.width ?? 0),
                                            height: Math.round(rects[0]?.height ?? 0),
                                        })
                                        : t('playground.slice.exportOriginal', { defaultValue: 'Original' })}
                                </MenuItem>
                                {EXPORT_SIZES.map((value) => (
                                    <MenuItem key={value} value={value}>{value} px</MenuItem>
                                ))}
                            </Select>
                        </FormControl>

                        <Box>
                            <FormControlLabel
                                control={(
                                    <Checkbox
                                        size="small"
                                        checked={cleanKind !== 'none'}
                                        onChange={(event) => setCleanKind(event.target.checked
                                            ? (detected === 'none' ? 'checker' : detected)
                                            : 'none')}
                                    />
                                )}
                                label={(
                                    <Typography variant="body2">
                                        {t('playground.slice.cleanBackground', { defaultValue: 'Clear the background' })}
                                    </Typography>
                                )}
                            />
                            {/* The detection is stated as a fact about this
                                image, so the checkbox stays a yes/no and the
                                user can tell whether it will find anything. */}
                            <Typography variant="caption" sx={{ display: 'block', color: 'text.secondary', mt: -0.5 }}>
                                {detected === 'checker' && t('playground.slice.detectedChecker', {
                                    defaultValue: 'Found a checkerboard — the picture a model paints instead of transparency.',
                                })}
                                {detected === 'green' && t('playground.slice.detectedGreen', {
                                    defaultValue: 'Found a green screen behind the artwork.',
                                })}
                                {detected === 'none' && t('playground.slice.detectedNone', {
                                    defaultValue: 'No checkerboard or green screen found; pick one to key it out anyway.',
                                })}
                            </Typography>
                            {cleanKind !== 'none' && (
                                <Stack spacing={1.5} sx={{ mt: 1.5 }}>
                                    <FormControl size="small" fullWidth>
                                        <InputLabel id="slice-clean-label">
                                            {t('playground.slice.cleanKind', { defaultValue: 'Background to clear' })}
                                        </InputLabel>
                                        <Select
                                            labelId="slice-clean-label"
                                            label={t('playground.slice.cleanKind', { defaultValue: 'Background to clear' })}
                                            value={cleanKind}
                                            onChange={(event) => setCleanKind(event.target.value as BackgroundKind)}
                                        >
                                            <MenuItem value="checker">
                                                {t('playground.slice.cleanChecker', { defaultValue: 'Checkerboard' })}
                                            </MenuItem>
                                            <MenuItem value="green">
                                                {t('playground.slice.cleanGreen', { defaultValue: 'Green screen' })}
                                            </MenuItem>
                                        </Select>
                                    </FormControl>
                                    <Box>
                                        <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                                {t('playground.slice.tolerance', { defaultValue: 'Tolerance' })} · {Math.round(tolerance * 100)}%
                                            </Typography>
                                            {/* The key colour itself, because a
                                                key is "this colour ± a margin"
                                                and the user cannot judge the
                                                margin without seeing the
                                                colour it is around. */}
                                            {cleanKind === 'green' && detectedColors[0] && (
                                                <>
                                                    <Box
                                                        sx={{
                                                            width: 12,
                                                            height: 12,
                                                            borderRadius: '2px',
                                                            border: '1px solid',
                                                            borderColor: 'divider',
                                                            bgcolor: `rgb(${detectedColors[0].join(',')})`,
                                                        }}
                                                    />
                                                    <Typography variant="caption" sx={{ color: 'text.disabled', fontFamily: 'monospace' }}>
                                                        {`#${detectedColors[0].map((value) => value.toString(16).padStart(2, '0')).join('')}`}
                                                    </Typography>
                                                </>
                                            )}
                                        </Stack>
                                        <Slider
                                            size="small"
                                            value={tolerance}
                                            min={0}
                                            max={1}
                                            step={0.02}
                                            onChange={(_, value) => setTolerance(value as number)}
                                            aria-label={t('playground.slice.tolerance', { defaultValue: 'Tolerance' })}
                                        />
                                    </Box>
                                </Stack>
                            )}
                        </Box>

                        <FormControlLabel
                            control={(
                                <Checkbox
                                    size="small"
                                    checked={rects.length > 0 && selectedRects.length === rects.length}
                                    indeterminate={selectedRects.length > 0 && selectedRects.length < rects.length}
                                    onChange={(event) => setExcluded(event.target.checked
                                        ? new Set()
                                        : new Set(rects.map((rect) => rect.index)))}
                                />
                            )}
                            label={(
                                <Typography variant="body2">
                                    {t('playground.slice.selectedCount', {
                                        defaultValue: '{{selected}} of {{total}} tiles',
                                        selected: selectedRects.length,
                                        total: rects.length,
                                    })}
                                </Typography>
                            )}
                        />

                        {/* The animation is the same cut, read in the same
                            order — so it lives on this surface as a preview
                            plus one more download, not behind a mode switch. */}
                        <Box sx={{ pt: 1, borderTop: 1, borderColor: 'divider' }}>
                            <Typography variant="subtitle2" sx={{ mb: 1 }}>
                                {t('playground.slice.animate', { defaultValue: 'Play the tiles in order' })}
                            </Typography>
                            <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center' }}>
                                <Box
                                    sx={{
                                        width: 88,
                                        height: 88,
                                        flexShrink: 0,
                                        borderRadius: 1,
                                        border: 1,
                                        borderColor: 'divider',
                                        backgroundImage: CHECKERBOARD_IMAGE,
                                        backgroundSize: '12px 12px',
                                        backgroundPosition: '0 0, 0 6px, 6px -6px, -6px 0',
                                        overflow: 'hidden',
                                    }}
                                >
                                    {previewFrames.length > 0 && (
                                        <Box
                                            component="img"
                                            src={previewFrames[Math.min(frame, previewFrames.length - 1)].url}
                                            alt={t('playground.slice.animationAlt', { defaultValue: 'Animation preview' })}
                                            sx={{ display: 'block', width: '100%', height: '100%', objectFit: 'contain' }}
                                        />
                                    )}
                                </Box>
                                <Stack spacing={1} sx={{ flex: 1, minWidth: 0 }}>
                                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                        <IconButton
                                            size="small"
                                            disabled={previewFrames.length < 2}
                                            onClick={() => setPlaying((current) => !current)}
                                            aria-label={playing
                                                ? t('playground.slice.pause', { defaultValue: 'Pause preview' })
                                                : t('playground.slice.play', { defaultValue: 'Play preview' })}
                                        >
                                            {playing ? <Pause fontSize="small" /> : <PlayArrow fontSize="small" />}
                                        </IconButton>
                                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                            {t('playground.slice.frameCount', {
                                                defaultValue: '{{count}} frames · {{fps}} fps',
                                                count: previewFrames.length,
                                                fps: Math.round(1000 / frameDelay),
                                            })}
                                        </Typography>
                                    </Stack>
                                    <FormControl size="small" fullWidth>
                                        <InputLabel id="slice-delay-label">
                                            {t('playground.slice.frameDelay', { defaultValue: 'Frame duration' })}
                                        </InputLabel>
                                        <Select
                                            labelId="slice-delay-label"
                                            label={t('playground.slice.frameDelay', { defaultValue: 'Frame duration' })}
                                            value={frameDelay}
                                            onChange={(event) => setFrameDelay(Number(event.target.value))}
                                        >
                                            {FRAME_DELAYS.map((value) => (
                                                <MenuItem key={value} value={value}>{value} ms</MenuItem>
                                            ))}
                                        </Select>
                                    </FormControl>
                                </Stack>
                            </Stack>
                        </Box>

                    </Stack>
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 2 }}>
                <Button onClick={onClose} color="inherit">
                    {t('playground.slice.cancel', { defaultValue: 'Cancel' })}
                </Button>
                <Button
                    variant="outlined"
                    startIcon={<Gif />}
                    disabled={!image || working || selectedRects.length < 2}
                    onClick={() => void handleDownloadGif()}
                >
                    {t('playground.slice.downloadGif', { defaultValue: 'Download GIF' })}
                </Button>
                <Button
                    variant="contained"
                    startIcon={working ? <CircularProgress size={16} color="inherit" /> : <Download />}
                    disabled={!image || working || selectedRects.length === 0}
                    onClick={() => void handleDownload()}
                >
                    {selectedRects.length === 1
                        ? t('playground.slice.downloadOne', { defaultValue: 'Download this tile' })
                        : t('playground.slice.downloadZip', {
                            defaultValue: 'Download {{count}} PNGs (ZIP)',
                            count: selectedRects.length,
                        })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ImageSliceDialog;
