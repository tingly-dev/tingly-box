// Tunables for the hero animation: sizes, timings and playback.

import { N } from './shape';

export const TILE = 0.88;          // tile edge as a fraction of the cell pitch
export const SPAN = N - 1 + TILE;  // T edge measured in pitches
export const TICK = 230;           // ms per walker step
export const POP = 620;            // ms tile pop-in
export const FADE = 420;           // ms walker fade in / out
export const HOLD = 2600;          // ms the finished T rests before folding
export const FOLD = 2200;          // ms for the T to drain back into the icon
export const VARIANTS = 6;         // icon shuffles, cycled round after round
export const ROUNDS = 3;           // rounds played on load; a click plays one more
export const DWELL = 2500;         // ms of hovering the resting T that also plays one more
export const ZOOM = 2.8;           // camera zoom when close on the icon
export const CAM_EASE = 700;       // ms time constant of the camera following the growth
