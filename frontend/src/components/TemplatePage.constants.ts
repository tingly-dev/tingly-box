export const SCROLLBAR_STYLES = {
    width: '8px',
    track: {
        backgroundColor: 'transparent',
    },
    thumb: {
        backgroundColor: 'rgba(0, 0, 0, 0.2)',
        borderRadius: '4px',
        '&:hover': {
            backgroundColor: 'rgba(0, 0, 0, 0.3)',
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
