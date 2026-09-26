import { useCallback } from 'react';
import { Box, ButtonBase, Popover, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    CAMERA_AZIMUTHS,
    CAMERA_ELEVATIONS,
    cameraCellOf,
    cameraTurn,
    figureTurn,
    fitFigureIntoTile,
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

const ViewThumbnail: React.FC<{ figure: PoseFigure; turn: FigureTurn }> = ({ figure, turn }) => {
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
        drawFigure(ctx, fitFigureIntoTile(setFigureTurn(figure, { yaw, pitch }), box, width * 0.08));
    }, [figure, yaw, pitch]);

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
}

const HEADER_WIDTH = 64;

// The landmarks of the orbit get a word under their number; the diagonals are
// just their number — "front-left three-quarter" is longer than the tile.
const AZIMUTH_NAMES: Record<number, 'front' | 'side' | 'back'> = { 0: 'front', 90: 'side', [-90]: 'side', 180: 'back' };

const ViewAnglePopover: React.FC<ViewAnglePopoverProps> = ({ anchorEl, figure, onClose, onPick }) => {
    const { t } = useTranslation();
    const current = figure ? cameraCellOf(figure) : null;
    const turn = figure ? figureTurn(figure) : { yaw: 0, pitch: 0 };

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
                                        {elevation.pitch > 0 ? '+' : ''}{elevation.pitch}°
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
                                            <ViewThumbnail figure={figure} turn={cameraTurn(elevation, azimuth)} />
                                        </ButtonBase>
                                    );
                                })}
                            </Stack>
                        );
                    })}
                    {/* The angles themselves, not just the name of the nearest
                        cell: once the figure has been turned by hand (the ring
                        handle) no cell matches, and the two numbers are the
                        only honest answer. */}
                    <Typography variant="caption" sx={{ color: 'text.secondary', pt: 0.5 }}>
                        {t('playground.sketch.pose.viewValue', {
                            defaultValue: 'Turned {{yaw}}° · camera {{pitch}}°',
                            yaw: Math.round(turn.yaw),
                            pitch: Math.round(turn.pitch),
                        })}
                    </Typography>
                </Stack>
            ) : null}
        </Popover>
    );
};

export default ViewAnglePopover;
