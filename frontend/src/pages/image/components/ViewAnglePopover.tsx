import { useCallback } from 'react';
import { Box, ButtonBase, Popover, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    CAMERA_AZIMUTHS,
    CAMERA_ELEVATIONS,
    cameraCellOf,
    cameraHeightOf,
    cameraTurn,
    figureLens,
    figureTurn,
    fitFigureIntoTile,
    LENSES,
    lensOf,
    setFigureLens,
    setFigureTurn,
    type FigureTurn,
    type PoseFigure,
    drawFigure,
} from '@tingly/mannequin';

// Where the camera stands. Deliberately not more entries in the pose grid: the
// same body seen from the side is the same body, and folding a camera move
// into the pose list would mean picking "sitting" could also spin the model
// round (principle 4 — orthogonal axes stay separate).
//
// And within the camera itself there are two more orthogonal axes, so it is a
// grid, not a list: rows are how high the camera stands (overhead → worm's
// eye), columns are where round the figure it stands. "Low angle from the
// side" is one row and one column, not a preset someone had to think of.
//
// Each tile is *this* figure from that position, not a generic icon of a
// camera: the answer to "what would the side view look like" is the side view.
const THUMB = { width: 40, height: 52, scale: 2 };

const ViewThumbnail: React.FC<{ figure: PoseFigure; turn: FigureTurn; lens: number }> = ({ figure, turn, lens }) => {
    const { yaw, pitch } = turn;
    const paint = useCallback((canvas: HTMLCanvasElement | null) => {
        if (!canvas) return;
        const width = THUMB.width * THUMB.scale;
        const height = THUMB.height * THUMB.scale;
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        if (!ctx) return;
        ctx.clearRect(0, 0, width, height);
        const box = { width, height };
        drawFigure(ctx, fitFigureIntoTile(setFigureLens(setFigureTurn(figure, { yaw, pitch }), lens), box, width * 0.08));
    }, [figure, yaw, pitch, lens]);

    return (
        <Box
            component="canvas"
            ref={paint}
            aria-hidden
            style={{ width: THUMB.width, height: THUMB.height }}
            sx={{ display: 'block' }}
        />
    );
};

interface ViewAnglePopoverProps {
    anchorEl: HTMLElement | null;
    figure: PoseFigure | null;
    onClose: () => void;
    onPick: (turn: FigureTurn) => void;
    onPickLens: (distance: number) => void;
}

const HEADER_WIDTH = 64;
// The lens row spans exactly the grid's width: 8 columns of (thumb + padding +
// border) with 7 gaps, shared out between 5 lenses with 4 gaps.
const GRID_WIDTH = 8 * (THUMB.width + 10) + 7 * 4;
const LENS_TILE_WIDTH = Math.floor((GRID_WIDTH - 4 * 4) / 5);

// The landmarks of the orbit get a word under their number; the diagonals are
// just their number — "front-left three-quarter" is longer than the tile.
const AZIMUTH_NAMES: Record<number, 'front' | 'side' | 'back'> = { 0: 'front', 90: 'side', [-90]: 'side', 180: 'back' };

const ViewAnglePopover: React.FC<ViewAnglePopoverProps> = ({ anchorEl, figure, onClose, onPick, onPickLens }) => {
    const { t } = useTranslation();
    const current = figure ? cameraCellOf(figure) : null;
    const turn = figure ? figureTurn(figure) : { yaw: 0, pitch: 0 };
    const lens = figure ? figureLens(figure) : 0;
    const currentLens = figure ? lensOf(figure) : null;

    return (
        <Popover
            open={anchorEl !== null && figure !== null}
            anchorEl={anchorEl}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            slotProps={{ paper: { sx: { p: 1.5, maxWidth: 'calc(100vw - 32px)', overflowX: 'auto' } } }}
        >
            {figure ? (
                <Stack spacing={0.5}>
                    {/* Column headers are the azimuth itself — 0° is facing the
                        camera, ±90° is a true profile, 180° is the back. */}
                    <Stack direction="row" spacing={0.5}>
                        <Box sx={{ width: HEADER_WIDTH, flexShrink: 0 }} />
                        {CAMERA_AZIMUTHS.map((azimuth) => (
                            <Typography
                                key={azimuth}
                                variant="caption"
                                sx={{ width: THUMB.width + 10, flexShrink: 0, textAlign: 'center', fontSize: 10, color: 'text.secondary' }}
                            >
                                <Box component="span" sx={{ display: 'block' }}>{azimuth}°</Box>
                                <Box component="span" sx={{ display: 'block', minHeight: 14 }}>
                                    {AZIMUTH_NAMES[azimuth]
                                        ? t(`playground.sketch.pose.camera.azimuth.${AZIMUTH_NAMES[azimuth]}`, { defaultValue: AZIMUTH_NAMES[azimuth] })
                                        : null}
                                </Box>
                            </Typography>
                        ))}
                    </Stack>
                    {CAMERA_ELEVATIONS.map((elevation) => {
                        const rowLabel = t(`playground.sketch.pose.camera.elevation.${elevation.key}`, { defaultValue: elevation.key });
                        return (
                            <Stack key={elevation.key} direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                                <Box sx={{ width: HEADER_WIDTH, flexShrink: 0, pr: 0.5 }}>
                                    <Typography variant="caption" component="div" sx={{ fontSize: 11, lineHeight: 1.2 }}>
                                        {rowLabel}
                                    </Typography>
                                    <Typography variant="caption" component="div" sx={{ fontSize: 10, color: 'text.secondary', lineHeight: 1.2 }}>
                                        {elevation.height > 0 ? '+' : ''}{elevation.height}°
                                    </Typography>
                                </Box>
                                {CAMERA_AZIMUTHS.map((azimuth) => {
                                    const selected = current?.elevation === elevation.key && current.azimuth === azimuth;
                                    const label = t('playground.sketch.pose.camera.cell', {
                                        defaultValue: '{{elevation}} · turned {{yaw}}°',
                                        elevation: rowLabel,
                                        yaw: azimuth,
                                    });
                                    return (
                                        <ButtonBase
                                            key={azimuth}
                                            onClick={() => onPick(cameraTurn(elevation, azimuth))}
                                            aria-label={label}
                                            title={label}
                                            aria-pressed={selected}
                                            sx={{
                                                p: '4px',
                                                flexShrink: 0,
                                                borderRadius: 1,
                                                border: '1px solid',
                                                borderColor: selected ? 'primary.main' : 'divider',
                                                bgcolor: selected ? 'action.selected' : 'background.paper',
                                                '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                            }}
                                        >
                                            <ViewThumbnail figure={figure} turn={cameraTurn(elevation, azimuth)} lens={lens} />
                                        </ButtonBase>
                                    );
                                })}
                            </Stack>
                        );
                    })}
                    {/* The lens is a third, independent axis — how far the camera
                        stands, not where — so it gets its own row rather than a
                        third dimension of the grid. Each tile is this figure,
                        from where the camera is now, at that distance: the
                        difference between lenses is only ever visible on the
                        figure itself. The grid above re-renders at the chosen
                        lens, so it always shows what a click will give. */}
                    <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', pt: 1, mt: 0.5, borderTop: '1px solid', borderColor: 'divider' }}>
                        <Box sx={{ width: HEADER_WIDTH, flexShrink: 0, pr: 0.5 }}>
                            <Typography variant="caption" component="div" sx={{ fontSize: 11, lineHeight: 1.2 }}>
                                {t('playground.sketch.pose.camera.lensTitle', { defaultValue: 'Lens' })}
                            </Typography>
                            <Typography variant="caption" component="div" sx={{ fontSize: 10, color: 'text.secondary', lineHeight: 1.2 }}>
                                {t('playground.sketch.pose.camera.lensHint', { defaultValue: 'near → far' })}
                            </Typography>
                        </Box>
                        {LENSES.map((option) => {
                            const selected = currentLens === option.key;
                            const name = t(`playground.sketch.pose.camera.lens.${option.key}`, { defaultValue: option.key });
                            const distance = t('playground.sketch.pose.camera.lensDistance', {
                                defaultValue: '{{distance}}× height',
                                distance: option.distance,
                            });
                            return (
                                <ButtonBase
                                    key={option.key}
                                    onClick={() => onPickLens(option.distance)}
                                    aria-label={`${name} · ${distance}`}
                                    title={`${name} · ${distance}`}
                                    aria-pressed={selected}
                                    sx={{
                                        p: '4px',
                                        width: LENS_TILE_WIDTH,
                                        flexShrink: 0,
                                        flexDirection: 'column',
                                        borderRadius: 1,
                                        border: '1px solid',
                                        borderColor: selected ? 'primary.main' : 'divider',
                                        bgcolor: selected ? 'action.selected' : 'background.paper',
                                        '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                    }}
                                >
                                    <ViewThumbnail figure={figure} turn={turn} lens={option.distance} />
                                    <Typography variant="caption" sx={{ fontSize: 10, lineHeight: 1.3, mt: 0.25 }}>
                                        {name}
                                    </Typography>
                                    <Typography variant="caption" sx={{ fontSize: 10, lineHeight: 1.2, color: 'text.secondary' }}>
                                        {distance}
                                    </Typography>
                                </ButtonBase>
                            );
                        })}
                    </Stack>
                    {/* The numbers themselves, not just the name of the nearest
                        cell: once the figure has been turned by hand (the ring
                        handle) no cell matches, and the numbers are the only
                        honest answer. */}
                    <Typography variant="caption" sx={{ color: 'text.secondary', pt: 0.5 }}>
                        {t('playground.sketch.pose.viewValue', {
                            defaultValue: 'Turned {{yaw}}° · camera height {{pitch}}°',
                            yaw: Math.round(turn.yaw),
                            pitch: Math.round(cameraHeightOf(turn)),
                        })}
                        {' · '}
                        {t('playground.sketch.pose.camera.lensDistance', {
                            defaultValue: '{{distance}}× height',
                            distance: Math.round(lens * 10) / 10,
                        })}
                    </Typography>
                </Stack>
            ) : null}
        </Popover>
    );
};

export default ViewAnglePopover;
