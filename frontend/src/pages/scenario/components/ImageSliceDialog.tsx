import { useCallback, useEffect, useMemo, useState } from 'react';
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
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, Download, Gif, GridView, Pause, PlayArrow } from '@/components/icons';
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
    GUTTER_MAX,
    loadImage,
    MARGIN_MAX,
    renderAnimationFrames,
    renderTile,
    renderTileDataUrl,
    tileFileName,
    type MatteSpec,
    type TileRect,
} from '@/utils/imageSlice';

// Slicing is only ever useful up to a handful of rows/columns; a bounded
// select keeps the control honest (concrete values, no free-text validation).
// 3x3 is the shape a sticker sheet almost always comes back as, so it is the
// grid the dialog opens on.
const AXIS_CHOICES = [1, 2, 3, 4, 5, 6];

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
    const [margin, setMargin] = useState(DEFAULT_GRID.margin);
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
        setMargin(DEFAULT_GRID.margin);
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
            ? computeTileRects(image.naturalWidth, image.naturalHeight, { rows, cols, margin, gutter })
            : []),
        [image, rows, cols, margin, gutter],
    );
    const selectedRects = useMemo(() => rects.filter((rect) => !excluded.has(rect.index)), [rects, excluded]);

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
                                sx={{
                                    position: 'relative',
                                    display: 'inline-block',
                                    maxWidth: '100%',
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
                                defaultValue: 'Cuts an evenly divided grid — a sticker sheet, a contact sheet, a spritesheet. Adjust the margin and gap until the outlines sit on the artwork, then click a tile to leave it out.',
                            })}
                        </Typography>

                        <Stack direction="row" spacing={1.5}>
                            <FormControl size="small" fullWidth>
                                <InputLabel id="slice-rows-label">
                                    {t('playground.slice.rows', { defaultValue: 'Rows' })}
                                </InputLabel>
                                <Select
                                    labelId="slice-rows-label"
                                    label={t('playground.slice.rows', { defaultValue: 'Rows' })}
                                    value={rows}
                                    onChange={(event) => setRows(Number(event.target.value))}
                                >
                                    {AXIS_CHOICES.map((value) => (
                                        <MenuItem key={value} value={value}>{value}</MenuItem>
                                    ))}
                                </Select>
                            </FormControl>
                            <FormControl size="small" fullWidth>
                                <InputLabel id="slice-cols-label">
                                    {t('playground.slice.cols', { defaultValue: 'Columns' })}
                                </InputLabel>
                                <Select
                                    labelId="slice-cols-label"
                                    label={t('playground.slice.cols', { defaultValue: 'Columns' })}
                                    value={cols}
                                    onChange={(event) => setCols(Number(event.target.value))}
                                >
                                    {AXIS_CHOICES.map((value) => (
                                        <MenuItem key={value} value={value}>{value}</MenuItem>
                                    ))}
                                </Select>
                            </FormControl>
                        </Stack>

                        <Box>
                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                {t('playground.slice.margin', { defaultValue: 'Outer margin' })} · {Math.round(margin * 100)}%
                            </Typography>
                            <Slider
                                size="small"
                                value={margin}
                                min={0}
                                max={MARGIN_MAX}
                                step={0.005}
                                onChange={(_, value) => setMargin(value as number)}
                                aria-label={t('playground.slice.margin', { defaultValue: 'Outer margin' })}
                            />
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
                                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                            {t('playground.slice.tolerance', { defaultValue: 'Edge tolerance' })} · {Math.round(tolerance * 100)}%
                                        </Typography>
                                        <Slider
                                            size="small"
                                            value={tolerance}
                                            min={0}
                                            max={1}
                                            step={0.02}
                                            onChange={(_, value) => setTolerance(value as number)}
                                            aria-label={t('playground.slice.tolerance', { defaultValue: 'Edge tolerance' })}
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
