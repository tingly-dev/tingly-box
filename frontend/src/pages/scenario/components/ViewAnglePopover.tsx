import { useCallback } from 'react';
import { Box, ButtonBase, Popover, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    figureTurn,
    fitFigureIntoTile,
    setFigureTurn,
    VIEW_PRESET_KEYS,
    VIEW_PRESETS,
    viewPresetOf,
    type PoseFigure,
    type ViewPresetKey,
} from '@/utils/poseFigure';
import { drawFigure } from '@/utils/poseFigure3d';

// Where the figure is looked at from. Deliberately not more entries in the
// pose grid: the same body seen from the side is the same body, and folding a
// camera move into the pose list would mean picking "sitting" could also spin
// the model round (principle 4 — orthogonal axes stay separate).
//
// Each tile is *this* figure at that angle, not a generic icon of a camera:
// the answer to "what would the side view look like" is the side view.
const THUMB = { width: 54, height: 70, scale: 2 };

const ViewThumbnail: React.FC<{ figure: PoseFigure; view: ViewPresetKey }> = ({ figure, view }) => {
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
        drawFigure(ctx, fitFigureIntoTile(setFigureTurn(figure, VIEW_PRESETS[view]), box, width * 0.08));
    }, [figure, view]);

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
    onPick: (view: ViewPresetKey) => void;
}

const ViewAnglePopover: React.FC<ViewAnglePopoverProps> = ({ anchorEl, figure, onClose, onPick }) => {
    const { t } = useTranslation();
    const current = figure ? viewPresetOf(figure) : null;
    const turn = figure ? figureTurn(figure) : { yaw: 0, pitch: 0 };

    return (
        <Popover
            open={anchorEl !== null && figure !== null}
            anchorEl={anchorEl}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            slotProps={{ paper: { sx: { p: 1.5, maxWidth: 476 } } }}
        >
            <Stack spacing={1}>
                <Stack direction="row" spacing={0.5} useFlexGap sx={{ flexWrap: 'wrap' }}>
                    {figure ? VIEW_PRESET_KEYS.map((view) => {
                        const label = t(`playground.sketch.pose.view.${view}`, { defaultValue: view });
                        const selected = current === view;
                        return (
                            <ButtonBase
                                key={view}
                                onClick={() => onPick(view)}
                                aria-label={label}
                                aria-pressed={selected}
                                sx={{
                                    p: 0.5,
                                    borderRadius: 1,
                                    border: '1px solid',
                                    flexDirection: 'column',
                                    borderColor: selected ? 'primary.main' : 'divider',
                                    bgcolor: selected ? 'action.selected' : 'background.paper',
                                    '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                }}
                            >
                                <ViewThumbnail figure={figure} view={view} />
                                <Typography variant="caption" sx={{ fontSize: 10, color: 'text.secondary', mt: 0.25 }}>
                                    {label}
                                </Typography>
                            </ButtonBase>
                        );
                    }) : null}
                </Stack>
                {/* The angles themselves, not just the name of the nearest
                    preset: once the figure has been turned by hand the name is
                    gone, and the two numbers are the only honest answer. */}
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {t('playground.sketch.pose.viewValue', {
                        defaultValue: 'Turned {{yaw}}° · camera {{pitch}}°',
                        yaw: Math.round(turn.yaw),
                        pitch: Math.round(turn.pitch),
                    })}
                </Typography>
            </Stack>
        </Popover>
    );
};

export default ViewAnglePopover;
