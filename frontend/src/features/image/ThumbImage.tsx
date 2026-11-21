import { Box, type SxProps, type Theme } from '@mui/material';
import { useLazyThumbnail } from './imageThumbnails';

interface ThumbImageProps {
    src: string;
    alt: string;
    // Longest edge of the downscaled copy (THUMB_EDGE_*).
    edge: number;
    fit?: 'contain' | 'cover';
    sx?: SxProps<Theme>;
}

// An image shown well below its own size. It renders from a downscaled copy
// made once it scrolls near the viewport, and fades in when that copy has
// loaded; until then the box holds its place empty. The original — the thing
// the lightbox, downloads and "use as reference" all work from — is never
// decoded here.
const ThumbImage: React.FC<ThumbImageProps> = ({ src, alt, edge, fit = 'cover', sx }) => {
    const { ref, url } = useLazyThumbnail<HTMLDivElement>(src, edge);
    return (
        <Box ref={ref} sx={[{ width: '100%', height: '100%', display: 'block' }, ...(Array.isArray(sx) ? sx : [sx])]}>
            {url && (
                <Box
                    component="img"
                    src={url}
                    alt={alt}
                    decoding="async"
                    onLoad={(event) => { event.currentTarget.style.opacity = '1'; }}
                    sx={{
                        width: '100%',
                        height: '100%',
                        objectFit: fit,
                        display: 'block',
                        opacity: 0,
                        transition: 'opacity 0.15s ease-out',
                    }}
                />
            )}
        </Box>
    );
};

export default ThumbImage;
