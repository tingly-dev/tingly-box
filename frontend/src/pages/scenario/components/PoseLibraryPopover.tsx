import { useCallback } from 'react';
import { Box, ButtonBase, Popover, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    createFigure,
    drawFigure,
    figureVisualBounds,
    POSE_LIBRARY,
    scaleFigure,
    translateFigure,
    type PosePresetKey,
} from '@/utils/poseFigure';

// Thumbnails are drawn by the same renderer as the canvas, not by hand-drawn
// icons: a library of poses is only useful if what you pick is what you get,
// and an icon set would drift the moment the manikin changes.
const THUMB = { width: 54, height: 76, scale: 2 };

const PoseThumbnail: React.FC<{ pose: PosePresetKey }> = ({ pose }) => {
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
        const figure = createFigure(pose, { width, height });
        const bounds = figureVisualBounds(figure);
        const pad = width * 0.08;
        const factor = Math.min((width - pad * 2) / bounds.width, (height - pad * 2) / bounds.height);
        const scaled = scaleFigure(figure, factor);
        const scaledBounds = figureVisualBounds(scaled);
        drawFigure(ctx, translateFigure(
            scaled,
            width / 2 - (scaledBounds.x + scaledBounds.width / 2),
            height / 2 - (scaledBounds.y + scaledBounds.height / 2),
        ));
    }, [pose]);

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
    onClose: () => void;
    onPick: (pose: PosePresetKey) => void;
}

const PoseLibraryPopover: React.FC<PoseLibraryPopoverProps> = ({ anchorEl, onClose, onPick }) => {
    const { t } = useTranslation();

    return (
        <Popover
            open={anchorEl !== null}
            anchorEl={anchorEl}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            slotProps={{ paper: { sx: { p: 1.5, maxWidth: 452 } } }}
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
                                            <PoseThumbnail pose={pose} />
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
