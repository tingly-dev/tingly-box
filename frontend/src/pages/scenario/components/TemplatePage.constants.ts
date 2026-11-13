import { radius } from '@tingly/ui/theme/radius'
import { overlay } from '@tingly/ui/theme/overlay';
export const SCROLLBAR_STYLES = {
    width: '8px',
    track: {
        backgroundColor: 'transparent',
    },
    thumb: {
        backgroundColor: overlay.scrollThumb,
        borderRadius: radius.pill,
        '&:hover': {
            backgroundColor: overlay.scrollThumbHover,
        },
    },
};

export const SCROLLBOX_SX = (headerHeight: number) => ({
    maxHeight: headerHeight > 0
        ? `calc(100vh - ${headerHeight + 180}px)`
        : 'calc(100vh - 300px)',
    overflowY: 'auto' as const,
    '&::-webkit-scrollbar': {
        width: SCROLLBAR_STYLES.width,
    },
    '&::-webkit-scrollbar-track': {
        backgroundColor: SCROLLBAR_STYLES.track.backgroundColor,
    },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: SCROLLBAR_STYLES.thumb.backgroundColor,
        borderRadius: SCROLLBAR_STYLES.thumb.borderRadius,
        '&:hover': {
            backgroundColor: SCROLLBAR_STYLES.thumb['&:hover'].backgroundColor,
        },
    },
});
