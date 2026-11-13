// Colours for what is drawn over pictures and over dark surfaces: scrims that
// dim a region, translucent action buttons and plates on an image, and the
// light tints on the dark lightbox. They sit on content, not on the page
// background, so they are the same in every theme.

const slate = (alpha: number) => `rgba(15, 23, 42, ${alpha})`;
const white = (alpha: number) => `rgba(255, 255, 255, ${alpha})`;

export const overlay = {
  /** Hover cue over an image ("click to enlarge"). */
  dimSoft: slate(0.38),
  /** Dims the region that is out of play (outside a crop, an excluded tile). */
  dim: slate(0.55),
  dimHover: slate(0.42),
  /** Round action button over an image. */
  button: slate(0.58),
  buttonHover: slate(0.82),
  /** Plate a group of thumbnails floats on. */
  plate: slate(0.62),
  /** Lightbox backdrop. */
  backdrop: slate(0.96),

  /** Light tints on the dark lightbox / scrims. */
  onDark: {
    fill: white(0.08),
    fillHover: white(0.16),
    fillStrong: white(0.14),
    fillActive: white(0.2),
    line: white(0.24),
    lineSoft: white(0.1),
    lineStrong: white(0.9),
    text: white(0.85),
    textSoft: white(0.75),
    positive: '#86efac',
    lineMedium: white(0.4),
  },

  /** Dark caption badge over an image. */
  badge: 'rgba(0, 0, 0, 0.7)',
  /** Diagonal hatch on a disabled canvas area. */
  hatch: 'rgba(0, 0, 0, 0.03)',
  /** Scrollbar thumb over a light surface. */
  scrollThumb: 'rgba(0, 0, 0, 0.2)',
  scrollThumbHover: 'rgba(0, 0, 0, 0.3)',

  /** Grey squares behind a picture with transparency. */
  checker: 'rgba(128, 128, 128, 0.18)',
} as const;

/** `rgb(r, g, b)` for a runtime colour, e.g. one picked from an image. */
export const cssRgb = ([r, g, b]: readonly [number, number, number]): string => `rgb(${r}, ${g}, ${b})`;
