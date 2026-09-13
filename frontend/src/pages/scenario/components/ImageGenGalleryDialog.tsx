import { useMemo, useState } from 'react';
import {
    Box,
    ButtonBase,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, Edit, ErrorOutline, Refresh, RestartAlt, Search, ZoomIn } from '@/components/icons';
import {
    formatBytes,
    resultSrc,
    type GenerationRun,
    type ImportedImage,
} from './ImageGenPlayground.types';

// One tile of the overview. A completed run contributes one tile per image it
// produced — the grid is about images, not about runs — while a run that is
// still going or that failed contributes the one tile that says so, because an
// overview that quietly drops those is not an overview of the session.
type GalleryTile =
    | { key: string; at: number; kind: 'output'; run: GenerationRun; imageIndex: number; src: string }
    | { key: string; at: number; kind: 'pending'; run: GenerationRun }
    | { key: string; at: number; kind: 'failed'; run: GenerationRun }
    | { key: string; at: number; kind: 'import'; item: ImportedImage };

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
            defaultValue: 'Generated from {{count}} reference images',
            count: sources.length,
        })}>
            <Stack
                direction="row"
                spacing={0.25}
                data-testid="imagegen-gallery-tile-sources"
                sx={{
                    position: 'absolute',
                    top: 6,
                    left: 6,
                    p: 0.25,
                    borderRadius: 1,
                    bgcolor: 'rgba(15, 23, 42, 0.62)',
                    backdropFilter: 'blur(4px)',
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
    onRemoveRun: (id: string) => void;
    onRemoveImport: (id: string) => void;
}

// Hover actions sit in the corner of a tile; the same chrome for all of them.
const tileActionSx = {
    width: 28,
    height: 28,
    color: 'common.white',
    bgcolor: 'rgba(15, 23, 42, 0.58)',
    backdropFilter: 'blur(4px)',
    '&:hover': { bgcolor: 'rgba(15, 23, 42, 0.82)' },
} as const;

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
    onRemoveRun,
    onRemoveImport,
}) => {
    const { t } = useTranslation();
    const [query, setQuery] = useState('');

    const tiles = useMemo<GalleryTile[]>(() => {
        const fromRuns = runs.flatMap<GalleryTile>((run) => {
            const at = run.createdAt ?? 0;
            if (run.status === 'pending') return [{ key: run.id, at, kind: 'pending' as const, run }];
            if (run.status === 'failed') return [{ key: run.id, at, kind: 'failed' as const, run }];
            return run.images
                .map((image, imageIndex) => ({ imageIndex, src: resultSrc(image) }))
                .filter(({ src }) => src)
                .map(({ imageIndex, src }) => ({
                    key: `${run.id}-${imageIndex}`,
                    at,
                    kind: 'output' as const,
                    run,
                    imageIndex,
                    src,
                }));
        });
        const fromImports = imported.map<GalleryTile>((item) => ({
            key: item.id,
            at: item.createdAt,
            kind: 'import',
            item,
        }));
        // Newest first: in an overview the thing just made is what the eye
        // should land on, and it is the one most likely to be acted on.
        return [...fromRuns, ...fromImports].sort((a, b) => b.at - a.at);
    }, [imported, runs]);

    const filtered = useMemo(() => {
        const needle = query.trim().toLowerCase();
        if (!needle) return tiles;
        return tiles.filter((tile) => (tile.kind === 'import'
            ? tile.item.name.toLowerCase().includes(needle)
            : [tile.run.prompt, tile.run.model].join(' ').toLowerCase().includes(needle)));
    }, [query, tiles]);

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth={false}
            fullWidth
            slotProps={{
                paper: {
                    sx: {
                        width: { xs: 'calc(100vw - 16px)', sm: 'calc(100vw - 48px)' },
                        height: { xs: 'calc(100dvh - 16px)', sm: 'calc(100dvh - 48px)' },
                        maxWidth: 'none',
                        maxHeight: 'none',
                        m: { xs: 1, sm: 3 },
                        borderRadius: 3,
                    },
                },
            }}
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
                        : t('playground.gallery.count', { defaultValue: '{{count}} images', count: tiles.length })}
                </Typography>
                {/* On a phone the row wraps: the title keeps the close button
                    company and the search takes the second line by itself,
                    rather than the close button being pushed onto a line of
                    its own. */}
                <TextField
                    size="small"
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
                            startAdornment: (
                                <InputAdornment position="start">
                                    <Search sx={{ fontSize: 18, color: 'text.disabled' }} />
                                </InputAdornment>
                            ),
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
                    <Stack spacing={1} sx={{ alignItems: 'center', justifyContent: 'center', height: '100%', color: 'text.secondary' }}>
                        <Typography variant="subtitle2">
                            {query.trim()
                                ? t('playground.gallery.noMatch', { defaultValue: 'Nothing matches “{{query}}”', query: query.trim() })
                                : t('playground.previewEmpty', { defaultValue: 'Generated and imported images appear here' })}
                        </Typography>
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
                                                sx={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block' }}
                                            />
                                            <Box
                                                className="tile-zoom"
                                                sx={{
                                                    position: 'absolute',
                                                    inset: 0,
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    color: 'common.white',
                                                    bgcolor: 'rgba(15, 23, 42, 0.34)',
                                                    opacity: 0,
                                                    transition: 'opacity 0.16s ease-out',
                                                }}
                                            >
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
                                        sx={{
                                            position: 'absolute',
                                            bottom: 8,
                                            right: 8,
                                            opacity: { xs: 1, md: 0 },
                                            transition: 'opacity 0.16s ease-out',
                                        }}
                                    >
                                        {tile.kind === 'failed' && (
                                            <Tooltip title={t('playground.retry', { defaultValue: 'Retry' })}>
                                                <IconButton
                                                    size="small"
                                                    onClick={() => onRetryRun(tile.run)}
                                                    aria-label={t('playground.retry', { defaultValue: 'Retry' })}
                                                    sx={tileActionSx}
                                                >
                                                    <Refresh fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        {tile.kind !== 'pending' && tile.kind !== 'failed' && (
                                            <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                                                <IconButton
                                                    size="small"
                                                    onClick={() => onUseAsReference(tile.kind === 'import' ? tile.item.src : tile.src)}
                                                    aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                                    data-testid="imagegen-gallery-use-as-reference"
                                                    sx={tileActionSx}
                                                >
                                                    <Edit fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        {tile.kind !== 'import' && (
                                            <Tooltip title={t('playground.reuseRequest', { defaultValue: 'Edit this request' })}>
                                                <IconButton
                                                    size="small"
                                                    onClick={() => onReuseRun(tile.run)}
                                                    aria-label={t('playground.reuseRequest', { defaultValue: 'Edit this request' })}
                                                    data-testid="imagegen-gallery-reuse-run"
                                                    sx={tileActionSx}
                                                >
                                                    <RestartAlt fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                    </Stack>
                                    {tile.kind !== 'pending' && (
                                        <IconButton
                                            className="tile-actions"
                                            size="small"
                                            onClick={() => (tile.kind === 'import'
                                                ? onRemoveImport(tile.item.id)
                                                : onRemoveRun(tile.run.id))}
                                            aria-label={tile.kind === 'import'
                                                ? t('playground.removeImported', { defaultValue: 'Remove {{name}}', name: tile.item.name })
                                                : t('playground.removeRun', { defaultValue: 'Remove this generation' })}
                                            sx={{
                                                position: 'absolute',
                                                top: 8,
                                                right: 8,
                                                opacity: { xs: 1, md: 0 },
                                                transition: 'opacity 0.16s ease-out',
                                                ...tileActionSx,
                                            }}
                                        >
                                            <Close fontSize="small" />
                                        </IconButton>
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
        </Dialog>
    );
};

export default ImageGenGalleryDialog;
