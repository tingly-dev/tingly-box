// The Image Playground's repeated styling, in one place. Every image in this
// feature — a reference thumbnail, a run's output, an imported file, an
// overview tile — wears the same two pieces of chrome over the picture: a dark
// round action button and a scrim that appears on hover to say "click to
// enlarge". They were written out at each of those places, drifting an alpha
// value at a time; a change to how an image reads should be one edit.

/** A round, translucent action button sitting over an image. */
export const overlayActionSx = (size = 30) => ({
    width: size,
    height: size,
    color: 'common.white',
    bgcolor: 'rgba(15, 23, 42, 0.58)',
    backdropFilter: 'blur(4px)',
    '&:hover': { bgcolor: 'rgba(15, 23, 42, 0.82)' },
});

/** The hover scrim behind the zoom cue. Revealed by the parent's hover rule. */
export const zoomScrimSx = {
    position: 'absolute',
    inset: 0,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: 'common.white',
    bgcolor: 'rgba(15, 23, 42, 0.38)',
    opacity: 0,
    transition: 'opacity 0.16s ease-out',
};

/** Actions that fade in with the pointer, and are always there on touch. */
export const hoverRevealSx = {
    opacity: { xs: 1, md: 0 },
    transition: 'opacity 0.16s ease-out',
};

/**
 * The paper of a dialog that takes over the window, minus a margin: the
 * lightbox and the overview are the same shape and must stay that way on a
 * phone, where the margin is what keeps them off the edges.
 */
export const fullBleedDialogPaperSx = {
    width: { xs: 'calc(100vw - 16px)', sm: 'calc(100vw - 48px)' },
    height: { xs: 'calc(100dvh - 16px)', sm: 'calc(100dvh - 48px)' },
    maxWidth: 'none',
    maxHeight: 'none',
    m: { xs: 1, sm: 3 },
    borderRadius: 3,
};

/**
 * The translucent plate a group of small thumbnails sits on when it floats over
 * something else — the lightbox's filmstrip, an overview tile's source badge.
 */
export const overlayPlateSx = {
    borderRadius: 1.5,
    bgcolor: 'rgba(15, 23, 42, 0.62)',
    backdropFilter: 'blur(6px)',
};

/** A results-panel header action: label beside the icon, icon alone on a phone. */
export const panelActionSx = {
    color: 'text.secondary',
    minWidth: 0,
    px: { xs: 0.75, sm: 1 },
    '& .MuiButton-startIcon': { mr: { xs: 0, sm: 0.5 } },
    '&:hover': { color: 'primary.main' },
};

export const panelActionLabelSx = { display: { xs: 'none', sm: 'inline' } };
