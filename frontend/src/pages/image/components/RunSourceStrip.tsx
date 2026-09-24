import { Box, ButtonBase, IconButton, Stack, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Edit, ZoomIn } from '@/components/icons';
import { hoverRevealSx, overlayActionSx, zoomScrimSx } from './ImageGenPlayground.chrome';

interface RunSourceStripProps {
    sources: string[];
    onOpen: (index: number) => void;
    // Puts one of these images back into the request as a reference. The
    // materials of a past run are the most likely input of the next one, so
    // getting them there is a click on the thumbnail, not a download and a
    // re-upload.
    onUseAsReference: (src: string) => void;
    // The in-flight card centres its content; the other two are left-aligned.
    align?: 'flex-start' | 'center';
}

// The images a run was built from, as thumbnails that open in the lightbox.
// Shown in every card state — while a run is in flight and after it failed is
// exactly when "what did I actually send?" needs an answer, and a retry that
// can't show its own materials asks the user to remember them.
const RunSourceStrip: React.FC<RunSourceStripProps> = ({ sources, onOpen, onUseAsReference, align = 'flex-start' }) => {
    const { t } = useTranslation();
    if (sources.length === 0) return null;
    return (
        <Stack
            direction="row"
            spacing={0.5}
            data-testid="imagegen-run-sources"
            sx={{ width: '100%', mt: 0.75, justifyContent: align, overflowX: 'auto', flexShrink: 0, scrollbarWidth: 'thin' }}
        >
            {sources.map((src, i) => (
                <Box
                    key={i}
                    sx={{
                        position: 'relative',
                        width: 36,
                        height: 36,
                        flexShrink: 0,
                        '&:hover .source-reuse, &:focus-within .source-reuse': { opacity: 1 },
                    }}
                >
                    <ButtonBase
                        onClick={() => onOpen(i)}
                        aria-label={t('playground.viewSourceImage', {
                            defaultValue: 'View original image {{number}}',
                            number: i + 1,
                        })}
                        sx={{
                            display: 'block',
                            width: '100%',
                            height: '100%',
                            borderRadius: 0.5,
                            overflow: 'hidden',
                            border: '1px solid',
                            borderColor: 'divider',
                            // The same cue every other openable image on the
                            // panel gives: it zooms, so it says so on hover.
                            '&:hover .source-zoom, &:focus-visible .source-zoom': { opacity: 1 },
                        }}
                    >
                        <Box
                            component="img"
                            src={src}
                            alt={t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: i + 1 })}
                            sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                        />
                        <Box className="source-zoom" sx={zoomScrimSx}>
                            <ZoomIn sx={{ fontSize: 16 }} />
                        </Box>
                    </ButtonBase>
                    <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                        <IconButton
                            className="source-reuse"
                            size="small"
                            onClick={(event) => { event.stopPropagation(); onUseAsReference(src); }}
                            aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                            data-testid="imagegen-source-use-as-reference"
                            sx={{
                                ...overlayActionSx(16),
                                position: 'absolute',
                                bottom: -2,
                                right: -2,
                                ...hoverRevealSx,
                                '&:hover, &:focus-visible': { opacity: 1 },
                            }}
                        >
                            <Edit sx={{ fontSize: 10 }} />
                        </IconButton>
                    </Tooltip>
                </Box>
            ))}
        </Stack>
    );
};

export default RunSourceStrip;
