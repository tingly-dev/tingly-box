// The camera starts close on the icon, pulls back to frame the T as it grows,
// and pushes in again while the T folds back into the icon.

import { CAM_EASE, SPAN, TILE, ZOOM } from './config';
import { easeInOut } from './math';
import type { Round } from './round';
import { CENTER, START, col, row } from './shape';

/** Focus point (grid units from the T's centre) and zoom. */
export interface Camera { x: number; y: number; z: number }

/** Close on the start cell, where every round begins and ends. */
export const startCam = (): Camera => ({ x: col(START) - CENTER, y: row(START) - CENTER, z: ZOOM });
/** The whole T in frame. */
export const wideCam = (): Camera => ({ x: 0, y: 0, z: 1 });

/** Where the camera wants to be: framing what has grown so far, with room to grow. */
export function camTarget(round: Round): Camera {
  let x0 = Infinity, x1 = -Infinity, y0 = Infinity, y1 = -Infinity;
  const grow = (i: number): void => {
    x0 = Math.min(x0, col(i)); x1 = Math.max(x1, col(i));
    y0 = Math.min(y0, row(i)); y1 = Math.max(y1, row(i));
  };
  for (const i of round.cells.keys()) grow(i);
  for (const w of round.walkers) grow(w.idx);
  const extent = Math.max(x1 - x0, y1 - y0) + TILE + (round.doneAt ? 0 : 1.2);
  const z = Math.max(1, Math.min(ZOOM, SPAN / extent));
  // at zoom 1 the whole T is framed, so centre on it; closer in, centre on the growth
  const k = (z - 1) / (ZOOM - 1);
  return { x: ((x0 + x1) / 2 - CENTER) * k, y: ((y0 + y1) / 2 - CENTER) * k, z };
}

/** Eases `cam` toward `target` over `dt` ms. */
export function follow(cam: Camera, target: Camera, dt: number): Camera {
  const f = 1 - Math.exp(-dt / CAM_EASE);
  return { x: cam.x + (target.x - cam.x) * f, y: cam.y + (target.y - cam.y) * f, z: cam.z + (target.z - cam.z) * f };
}

/**
 * During the fold the camera glides from the whole T back to the icon, tied to
 * the fold's progress so it settles exactly as the last tile lands.
 */
export function foldCam(t: number): Camera {
  const from = wideCam(), to = startCam(), e = easeInOut(t);
  return { x: from.x + (to.x - from.x) * e, y: from.y + (to.y - from.y) * e, z: from.z + (to.z - from.z) * e };
}
