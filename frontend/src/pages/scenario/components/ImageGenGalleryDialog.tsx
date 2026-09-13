import { useMemo, useState, type ReactNode } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, DeleteSweep, Edit, ErrorOutline, Photo, Refresh, RestartAlt, ZoomIn } from '@/components/icons';
import ConfirmDialog from '@/components/ConfirmDialog';
import EmptyState from '@/components/EmptyState';
import SearchField from '@/components/SearchField';
import type { GenerationRun, ImportedImage } from './ImageGenPlayground.types';
import { buildGalleryTiles, filterGalleryTiles, formatBytes } from './imageGenSession';
import {
    fullBleedDialogPaperSx,
    hoverRevealSx,
    overlayActionSx,
    overlayPlateSx,
    zoomScrimSx,
} from './ImageGenPlayground.chrome';

// Up to three of a run's reference images, in the corner of its tile. In a
// grid of finished pictures there is otherwise nothing to say a tile came from
// an edit, let alone from what — and that is the question the overview gets
// asked most after "where is it". Informational only: the frames are
// click-through, and the lightbox behind the tile is where they are browsable.
const TileSourceBadge: React.FC<{ sources: string[] }> = ({ sources }) => {
    const { t } = useTranslation();
    if (sources.length === 0) return null;
    const shown = sources.slice(0, 3);
    return (
        <Tooltip title={t('playground.gallery.fromReferences', {
            defaultValue_one: 'Generated from {{count}} reference image',
            defaultValue_other: 'Generated from {{count}} reference images',
            count: sources.length,
        })}>
            <Stack
                direction="row"
                spacing={0.25}
                data-testid="imagegen-gallery-tile-sources"
                sx={{
                    ...overlayPlateSx,
                    position: 'absolute',
                    top: 6,
                    left: 6,
                    p: 0.25,
                    borderRadius: 1,
                    pointerEvents: 'none',
                    alignItems: 'center',
                }}
            >
                {shown.map((src, i) => (
                    <Box
                        key={i}
                        component="img"
                        src={src}
                        alt=""
                        loading="lazy"
                        decoding="async"
                        sx={{ width: 22, height: 22, borderRadius: 0.5, objectFit: 'cover', display: 'block' }}
                    />
                ))}
                {sources.length > shown.length && (
                    <Typography variant="caption" sx={{ px: 0.25, color: 'common.white', fontSize: 10 }}>
                        +{sources.length - shown.length}
                    </Typography>
                )}
            </Stack>
        </Tooltip>
    );
};

interface ImageGenGalleryDialogProps {
    open: boolean;
    runs: GenerationRun[];
    imported: ImportedImage[];
    onClose: () => void;
    onOpenOutput: (run: GenerationRun, imageIndex: number, src: string) => void;
    onOpenImport: (item: ImportedImage) => void;
    onUseAsReference: (src: string) => void;
    onReuseRun: (run: GenerationRun) => void;
    onRetryRun: (run: GenerationRun) => void;
    onCancelRun: (id: string) => void;
    onRemoveRun: (id: string) => void;
    onRemoveImport: (id: string) => void;
    // Empties the session. Only reachable from here: this is the one surface
    // that shows how much there is to empty.
    onClearAll: () => void;
}

// Every action on a tile is the same button with a different icon: it fades in
// with the pointer, is always there on touch, and says the same thing twice —
// once to the pointer, once to a screen reader.
const TileAction: React.FC<{
    label: string;
    icon: ReactNode;
    onClick: () => void;
    testId?: string;
    corner?: 'bottom' | 'top';
}> = ({ label, icon, onClick, testId, corner = 'bottom' }) => (
    <Tooltip title={label}>
        <IconButton
            size="small"
            onClick={onClick}
            aria-label={label}
            data-testid={testId}
            sx={corner === 'top'
                ? { ...overlayActionSx(28), position: 'absolute', top: 8, right: 8, ...hoverRevealSx }
                : overlayActionSx(28)}
        >
            {icon}
        </IconButton>
    </Tooltip>
);

/**
 * The session's images all at once.
 *
 * The results panel is a horizontal strip: right for "watch the run that is
 * happening", useless for "find the one I made twenty images ago" — that turns
 * into dragging a scrollbar and squinting. This dialog answers the second
 * question instead: every image in the session on one wrapping grid, newest
 * first (the strip is oldest-first, because there the newest is the one it
 * scrolls to), searchable by prompt or file name, each tile carrying the same
 * actions its card has — open it, send it back in as a reference, load the
 * request that made it.
 */
const ImageGenGalleryDialog: React.FC<ImageGenGalleryDialogProps> = ({
    open,
    runs,
    imported,
    onClose,
    onOpenOutput,
    onOpenImport,
    onUseAsReference,
    onReuseRun,
    onRetryRun,
    onCancelRun,
    onRemoveRun,
    onRemoveImport,
    onClearAll,
}) => {
    const { t } = useTranslation();
    const [query, setQuery] = useState('');
    const [confirmClear, setConfirmClear] = useState(false);
    const clearLabel = t('playground.gallery.clearAll', { defaultValue: 'Clear session' });

    const tiles = useMemo(() => buildGalleryTiles(runs, imported), [imported, runs]);
    const filtered = useMemo(() => filterGalleryTiles(tiles, query), [query, tiles]);

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth={false}
            fullWidth
            slotProps={{ paper: { sx: fullBleedDialogPaperSx } }}
        >
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1.5, py: 1.5, pr: 1.5, flexWrap: 'wrap' }}>
                <Typography variant="h6" component="span" sx={{ fontSize: '1.05rem' }}>
                    {t('playground.gallery.title', { defaultValue: 'All session images' })}
                </Typography>
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {query.trim()
                        ? t('playground.gallery.matchCount', {
                            defaultValue: '{{shown}} of {{total}}',
                            shown: filtered.length,
                            total: tiles.length,
                        })
                        : t('playground.gallery.count', {
                            defaultValue_one: '{{count}} image',
                            defaultValue_other: '{{count}} images',
                            count: tiles.length,
                        })}
                </Typography>
                {/* On a phone the row wraps: the title keeps the close button
                    company and the search takes the second line by itself,
                    rather than the close button being pushed onto a line of
                    its own. */}
                <SearchField
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder={t('playground.gallery.search', { defaultValue: 'Search prompts and file names' })}
                    data-testid="imagegen-gallery-search"
                    sx={{
                        width: { xs: '100%', sm: 280 },
                        order: { xs: 3, sm: 1 },
                        ml: { sm: 'auto' },
                    }}
                    slotProps={{
                        input: {
                            endAdornment: query ? (
                                <InputAdornment position="end">
                                    <IconButton
                                        size="small"
                                        edge="end"
                                        onClick={() => setQuery('')}
                                        aria-label={t('playground.gallery.clearSearch', { defaultValue: 'Clear the search' })}
                                    >
                                        <Close sx={{ fontSize: 16 }} />
                                    </IconButton>
                                </InputAdornment>
                            ) : undefined,
                        },
                    }}
                />
                {tiles.length > 0 && (
                    <Button
                        size="small"
                        color="inherit"
                        startIcon={<DeleteSweep fontSize="small" />}
                        onClick={() => setConfirmClear(true)}
                        data-testid="imagegen-gallery-clear"
                        sx={{ order: { xs: 4, sm: 1 }, color: 'text.secondary', '&:hover': { color: 'error.main' } }}
                    >
                        {clearLabel}
                    </Button>
                )}
                <IconButton
                    onClick={onClose}
                    aria-label={t('playground.gallery.close', { defaultValue: 'Close the overview' })}
                    sx={{ order: 2, ml: { xs: 'auto', sm: 0 } }}
                >
                    <Close />
                </IconButton>
            </DialogTitle>
            <DialogContent dividers sx={{ bgcolor: 'action.hover' }}>
                {filtered.length === 0 ? (
                    <Stack sx={{ height: '100%', justifyContent: 'center' }}>
                        <EmptyState
                            icon={<Photo />}
                            title={query.trim()
                                ? t('playground.gallery.noMatch', { defaultValue: 'Nothing matches “{{query}}”', query: query.trim() })
                                : t('playground.previewEmpty', { defaultValue: 'Generated and imported images appear here' })}
                            description={query.trim()
                                ? t('playground.gallery.noMatchHint', { defaultValue: 'Search runs over prompts, models and file names.' })
                                : undefined}
                        />
                    </Stack>
                ) : (
                    <Box
                        data-testid="imagegen-gallery-grid"
                        sx={{
                            display: 'grid',
                            gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 190px), 1fr))',
                            gap: 1.5,
                            alignContent: 'start',
                        }}
                    >
                        {filtered.map((tile) => (
                            <Box
                                key={tile.key}
                                data-testid="imagegen-gallery-tile"
                                data-gallery-kind={tile.kind}
                                sx={{ minWidth: 0 }}
                            >
                                <Box
                                    sx={{
                                        position: 'relative',
                                        aspectRatio: '1 / 1',
                                        borderRadius: 1.5,
                                        overflow: 'hidden',
                                        border: '1px solid',
                                        borderColor: tile.kind === 'failed' ? 'error.main' : 'divider',
                                        bgcolor: 'background.paper',
                                        '&:hover .tile-actions, &:focus-within .tile-actions': { opacity: 1 },
                                    }}
                                >
                                    {tile.kind === 'pending' ? (
                                        <Stack sx={{ height: '100%', alignItems: 'center', justifyContent: 'center' }} spacing={1}>
                                            <CircularProgress size={22} />
                                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                                {t('playground.generatingNew', { defaultValue: 'Generating new images…' })}
                                            </Typography>
                                        </Stack>
                                    ) : tile.kind === 'failed' ? (
                                        <Stack sx={{ height: '100%', alignItems: 'center', justifyContent: 'center', p: 1.5, textAlign: 'center' }} spacing={0.75}>
                                            <ErrorOutline color="error" fontSize="small" />
                                            <Typography variant="caption" sx={{ fontWeight: 500 }}>
                                                {t('playground.runFailed', { defaultValue: 'Generation failed' })}
                                            </Typography>
                                            <Typography
                                                variant="caption"
                                                sx={{
                                                    color: 'error.main',
                                                    display: '-webkit-box',
                                                    WebkitLineClamp: 3,
                                                    WebkitBoxOrient: 'vertical',
                                                    overflow: 'hidden',
                                                    wordBreak: 'break-word',
                                                }}
                                            >
                                                {tile.run.error}
                                            </Typography>
                                        </Stack>
                                    ) : (
                                        <ButtonBase
                                            onClick={() => (tile.kind === 'import'
                                                ? onOpenImport(tile.item)
                                                : onOpenOutput(tile.run, tile.imageIndex, tile.src))}
                                            aria-label={tile.kind === 'import'
                                                ? t('playground.openImported', { defaultValue: 'Open {{name}}', name: tile.item.name })
                                                : t('playground.openResult', {
                                                    defaultValue: 'Open generated image {{number}}',
                                                    number: tile.imageIndex + 1,
                                                })}
                                            sx={{
                                                width: '100%',
                                                height: '100%',
                                                display: 'block',
                                                '&:hover .tile-zoom, &:focus-visible .tile-zoom': { opacity: 1 },
                                            }}
                                        >
                                            <Box
                                                component="img"
                                                src={tile.kind === 'import' ? tile.item.src : tile.src}
                                                alt={tile.kind === 'import' ? tile.item.name : tile.run.prompt}
                                                loading="lazy"
                                                decoding="async"
                                                sx={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block' }}
                                            />
                                            <Box className="tile-zoom" sx={zoomScrimSx}>
                                                <ZoomIn sx={{ fontSize: 28 }} />
                                            </Box>
                                        </ButtonBase>
                                    )}

                                    {tile.kind !== 'import' && (
                                        <TileSourceBadge sources={tile.run.sourceImages ?? []} />
                                    )}

                                    {/* The tile's actions are the card's actions: nothing
                                        here sends the user back to the strip to do a thing
                                        the overview could have done. */}
                                    <Stack
                                        className="tile-actions"
                                        direction="row"
                                        spacing={0.5}
                                        sx={{ position: 'absolute', bottom: 8, right: 8, ...hoverRevealSx }}
                                    >
                                        {tile.kind === 'pending' && (
                                            <TileAction
                                                label={t('playground.cancelRun', { defaultValue: 'Cancel' })}
                                                icon={<Close fontSize="small" />}
                                                onClick={() => onCancelRun(tile.run.id)}
                                                testId="imagegen-gallery-cancel-run"
                                            />
                                        )}
                                        {tile.kind === 'failed' && (
                                            <TileAction
                                                label={t('playground.retry', { defaultValue: 'Retry' })}
                                                icon={<Refresh fontSize="small" />}
                                                onClick={() => onRetryRun(tile.run)}
                                            />
                                        )}
                                        {(tile.kind === 'output' || tile.kind === 'import') && (
                                            <TileAction
                                                label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                                icon={<Edit fontSize="small" />}
                                                onClick={() => onUseAsReference(tile.kind === 'import' ? tile.item.src : tile.src)}
                                                testId="imagegen-gallery-use-as-reference"
                                            />
                                        )}
                                        {tile.kind !== 'import' && (
                                            <TileAction
                                                label={t('playground.reuse.action', { defaultValue: 'Edit this request' })}
                                                icon={<RestartAlt fontSize="small" />}
                                                onClick={() => onReuseRun(tile.run)}
                                                testId="imagegen-gallery-reuse-run"
                                            />
                                        )}
                                    </Stack>
                                    {tile.kind !== 'pending' && (
                                        <TileAction
                                            corner="top"
                                            label={tile.kind === 'import'
                                                ? t('playground.removeImported', { defaultValue: 'Remove {{name}}', name: tile.item.name })
                                                : t('playground.removeRun', { defaultValue: 'Remove this generation' })}
                                            icon={<Close fontSize="small" />}
                                            onClick={() => (tile.kind === 'import'
                                                ? onRemoveImport(tile.item.id)
                                                : onRemoveRun(tile.run.id))}
                                        />
                                    )}
                                </Box>
                                {/* Two lines under every tile, always the same two: what it
                                    is, then what made it. A grid of pictures with no captions
                                    is a puzzle. */}
                                <Typography
                                    variant="caption"
                                    sx={{
                                        display: '-webkit-box',
                                        WebkitLineClamp: 2,
                                        WebkitBoxOrient: 'vertical',
                                        overflow: 'hidden',
                                        mt: 0.75,
                                        wordBreak: 'break-word',
                                    }}
                                >
                                    {tile.kind === 'import' ? tile.item.name : tile.run.prompt}
                                </Typography>
                                <Typography
                                    variant="caption"
                                    sx={{
                                        display: 'block',
                                        color: 'text.disabled',
                                        overflow: 'hidden',
                                        textOverflow: 'ellipsis',
                                        whiteSpace: 'nowrap',
                                    }}
                                >
                                    {tile.kind === 'import'
                                        ? [
                                            t('playground.importedBadge', { defaultValue: 'Imported' }),
                                            tile.item.width && tile.item.height ? `${tile.item.width}×${tile.item.height} px` : '',
                                            formatBytes(tile.item.bytes),
                                        ].filter(Boolean).join(' · ')
                                        : `${tile.run.model} · ${tile.run.size} · ${tile.run.quality}`}
                                </Typography>
                            </Box>
                        ))}
                    </Box>
                )}
            </DialogContent>
            <ConfirmDialog
                open={confirmClear}
                title={t('playground.gallery.clearAllTitle', { defaultValue: 'Clear this session?' })}
                description={t('playground.gallery.clearAllBody', {
                    defaultValue_one: 'Removes the image from the playground. Images already written to the output folder stay on disk.',
                    defaultValue_other: 'Removes all {{count}} images from the playground. Images already written to the output folder stay on disk.',
                    count: tiles.length,
                })}
                confirmLabel={clearLabel}
                cancelLabel={t('common.cancel', { defaultValue: 'Cancel' })}
                confirmColor="error"
                onClose={() => setConfirmClear(false)}
                onConfirm={() => { setConfirmClear(false); onClearAll(); }}
            />
        </Dialog>
    );
};

export default ImageGenGalleryDialog;
