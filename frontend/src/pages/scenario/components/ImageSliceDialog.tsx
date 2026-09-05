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
import { Close, Download, GridView } from '@/components/icons';
import { createZipBlob } from '@/utils/zip';
import { downloadBlob, slugify } from '@/utils/download';
import {
    computeTileRects,
    DEFAULT_GRID,
    GUTTER_MAX,
    loadImage,
    MARGIN_MAX,
    renderTile,
    tileFileName,
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

    const rects = useMemo(
        () => (image
            ? computeTileRects(image.naturalWidth, image.naturalHeight, { rows, cols, margin, gutter })
            : []),
        [image, rows, cols, margin, gutter],
    );
    const selectedRects = useMemo(() => rects.filter((rect) => !excluded.has(rect.index)), [rects, excluded]);

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
            if (selectedRects.length === 1) {
                downloadBlob(await renderTile(image, selectedRects[0], exportSize), nameOf(selectedRects[0]));
                return;
            }
            const entries = await Promise.all(selectedRects.map(async (rect) => ({
                name: nameOf(rect),
                data: new Uint8Array(await (await renderTile(image, rect, exportSize)).arrayBuffer()),
            })));
            downloadBlob(createZipBlob(entries), `${stem}-${selectedRects.length}.zip`);
        } catch {
            showNotification(
                t('image-playground.slice.failed', { defaultValue: 'Could not slice this image' }),
                'error',
            );
        } finally {
            setWorking(false);
        }
    }, [exportSize, image, rects.length, selectedRects, showNotification, stem, t]);

    const naturalWidth = image?.naturalWidth ?? 1;
    const naturalHeight = image?.naturalHeight ?? 1;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 1 }}>
                <GridView fontSize="small" />
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="h6" component="span" sx={{ display: 'block', fontSize: '1.05rem' }}>
                        {t('image-playground.slice.title', { defaultValue: 'Split into tiles' })}
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
                    aria-label={t('image-playground.slice.close', { defaultValue: 'Close slicer' })}
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
                                {t('image-playground.slice.loadFailed', {
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
                                    alt={t('image-playground.slice.sheetAlt', { defaultValue: 'Image being sliced' })}
                                    sx={{ display: 'block', maxWidth: '100%', maxHeight: '60vh' }}
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
                                            aria-label={t('image-playground.slice.tile', {
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
                                        />
                                    );
                                })}
                            </Box>
                        )}
                    </Box>

                    <Stack spacing={2}>
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {t('image-playground.slice.hint', {
                                defaultValue: 'Cuts an evenly divided grid — a sticker sheet, a contact sheet, a spritesheet. Adjust the margin and gap until the outlines sit on the artwork, then click a tile to leave it out.',
                            })}
                        </Typography>

                        <Stack direction="row" spacing={1.5}>
                            <FormControl size="small" fullWidth>
                                <InputLabel id="slice-rows-label">
                                    {t('image-playground.slice.rows', { defaultValue: 'Rows' })}
                                </InputLabel>
                                <Select
                                    labelId="slice-rows-label"
                                    label={t('image-playground.slice.rows', { defaultValue: 'Rows' })}
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
                                    {t('image-playground.slice.cols', { defaultValue: 'Columns' })}
                                </InputLabel>
                                <Select
                                    labelId="slice-cols-label"
                                    label={t('image-playground.slice.cols', { defaultValue: 'Columns' })}
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
                                {t('image-playground.slice.margin', { defaultValue: 'Outer margin' })} · {Math.round(margin * 100)}%
                            </Typography>
                            <Slider
                                size="small"
                                value={margin}
                                min={0}
                                max={MARGIN_MAX}
                                step={0.005}
                                onChange={(_, value) => setMargin(value as number)}
                                aria-label={t('image-playground.slice.margin', { defaultValue: 'Outer margin' })}
                            />
                        </Box>
                        <Box>
                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                {t('image-playground.slice.gutter', { defaultValue: 'Gap between tiles' })} · {Math.round(gutter * 100)}%
                            </Typography>
                            <Slider
                                size="small"
                                value={gutter}
                                min={0}
                                max={GUTTER_MAX}
                                step={0.01}
                                onChange={(_, value) => setGutter(value as number)}
                                aria-label={t('image-playground.slice.gutter', { defaultValue: 'Gap between tiles' })}
                            />
                        </Box>

                        <FormControl size="small" fullWidth>
                            <InputLabel id="slice-export-label">
                                {t('image-playground.slice.exportSize', { defaultValue: 'Output size' })}
                            </InputLabel>
                            <Select
                                labelId="slice-export-label"
                                label={t('image-playground.slice.exportSize', { defaultValue: 'Output size' })}
                                value={exportSize ?? 'original'}
                                onChange={(event) => {
                                    const value = event.target.value;
                                    setExportSize(value === 'original' ? null : Number(value));
                                }}
                            >
                                <MenuItem value="original">
                                    {image
                                        ? t('image-playground.slice.exportOriginalWithSize', {
                                            defaultValue: 'Original · {{width}}×{{height}} px',
                                            width: Math.round(rects[0]?.width ?? 0),
                                            height: Math.round(rects[0]?.height ?? 0),
                                        })
                                        : t('image-playground.slice.exportOriginal', { defaultValue: 'Original' })}
                                </MenuItem>
                                {EXPORT_SIZES.map((value) => (
                                    <MenuItem key={value} value={value}>{value} px</MenuItem>
                                ))}
                            </Select>
                        </FormControl>

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
                                    {t('image-playground.slice.selectedCount', {
                                        defaultValue: '{{selected}} of {{total}} tiles',
                                        selected: selectedRects.length,
                                        total: rects.length,
                                    })}
                                </Typography>
                            )}
                        />

                    </Stack>
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 2 }}>
                <Button onClick={onClose} color="inherit">
                    {t('image-playground.slice.cancel', { defaultValue: 'Cancel' })}
                </Button>
                <Button
                    variant="contained"
                    startIcon={working ? <CircularProgress size={16} color="inherit" /> : <Download />}
                    disabled={!image || working || selectedRects.length === 0}
                    onClick={() => void handleDownload()}
                >
                    {selectedRects.length === 1
                        ? t('image-playground.slice.downloadOne', { defaultValue: 'Download this tile' })
                        : t('image-playground.slice.downloadZip', {
                            defaultValue: 'Download {{count}} PNGs (ZIP)',
                            count: selectedRects.length,
                        })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ImageSliceDialog;
