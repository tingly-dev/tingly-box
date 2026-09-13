import { useCallback } from 'react';
import { Box, ButtonBase, Popover, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    createFigure,
    drawFigure,
    fitFigureIntoTile,
    POSE_LIBRARY,
    type FigureTurn,
    type PosePresetKey,
} from '@/utils/poseFigure';

// Thumbnails are drawn by the same renderer as the canvas, not by hand-drawn
// icons: a library of poses is only useful if what you pick is what you get,
// and an icon set would drift the moment the manikin changes.
// Sized so that the widest group — nine poses — is one row: a group that wraps
// to 8 + 1 reads as two groups, and the grouping is the only navigation this
// grid has.
const THUMB = { width: 46, height: 64, scale: 2 };

// Drawn from the angle the figure on the canvas is being looked at, not from
// a fixed elevation: the grid's whole value is that what you pick is what you
// get, and a pose seen from the front and the same pose seen from above are
// not the same picture.
const PoseThumbnail: React.FC<{ pose: PosePresetKey; turn: FigureTurn }> = ({ pose, turn }) => {
    const paint = useCallback((canvas: HTMLCanvasElement | null) => {
        if (!canvas) return;
        const width = THUMB.width * THUMB.scale;
        const height = THUMB.height * THUMB.scale;
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        if (!ctx) return;
        ctx.clearRect(0, 0, width, height);
        // Fitted to the tile rather than drawn at the canvas's own sizing:
        // arms out or lying down, a pose is much wider than a standing one and
        // would be cropped at exactly the poses that need to be recognisable.
        const box = { width, height };
        drawFigure(ctx, fitFigureIntoTile(createFigure(pose, box, undefined, 0, turn), box, width * 0.08));
    }, [pose, turn]);

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

interface PoseLibraryPopoverProps {
    anchorEl: HTMLElement | null;
    turn: FigureTurn;
    onClose: () => void;
    onPick: (pose: PosePresetKey) => void;
}

const PoseLibraryPopover: React.FC<PoseLibraryPopoverProps> = ({ anchorEl, turn, onClose, onPick }) => {
    const { t } = useTranslation();

    return (
        <Popover
            open={anchorEl !== null}
            anchorEl={anchorEl}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            slotProps={{ paper: { sx: { p: 1.5, maxWidth: 588, maxHeight: '72vh' } } }}
        >
            <Stack spacing={1.25}>
                {POSE_LIBRARY.map(({ group, poses }) => (
                    <Box key={group}>
                        <Typography
                            variant="caption"
                            sx={{ display: 'block', color: 'text.secondary', mb: 0.5 }}
                        >
                            {t(`playground.sketch.pose.group.${group}`, { defaultValue: group })}
                        </Typography>
                        <Stack direction="row" spacing={0.5} useFlexGap sx={{ flexWrap: 'wrap' }}>
                            {poses.map((pose) => {
                                const label = t(`playground.sketch.pose.preset.${pose}`, { defaultValue: pose });
                                return (
                                    <Tooltip key={pose} title={label}>
                                        <ButtonBase
                                            onClick={() => onPick(pose)}
                                            aria-label={label}
                                            sx={{
                                                p: 0.5,
                                                borderRadius: 1,
                                                border: '1px solid',
                                                borderColor: 'divider',
                                                bgcolor: 'background.paper',
                                                '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                            }}
                                        >
                                            <PoseThumbnail pose={pose} turn={turn} />
                                        </ButtonBase>
                                    </Tooltip>
                                );
                            })}
                        </Stack>
                    </Box>
                ))}
            </Stack>
        </Popover>
    );
};

export default PoseLibraryPopover;
