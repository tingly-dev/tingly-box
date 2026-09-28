// One round of growth: walkers hop outward from the start cell, and every cell
// they land on joins the T. `step` advances a round in fixed ticks.

import { FADE, POP, TICK } from './config';
import { START, neighbours } from './shape';

export interface Walker { idx: number; prev: number; since: number; bornAt: number; dieAt: number }
export interface Round { variant: number; cells: Map<number, number>; walkers: Walker[]; lastTick: number; ticks: number; doneAt: number }

export function newRound(variant: number, now: number): Round {
  return {
    variant,
    cells: new Map([[START, now - POP * 2]]),
    walkers: [{ idx: START, prev: START, since: now, bornAt: now, dieAt: 0 }],
    lastTick: now, ticks: 0, doneAt: 0,
  };
}

export function step(r: Round, now: number, random: () => number): void {
  // after a background tab or a pause, pick up where we were instead of catching up
  if (now - r.lastTick > 1000) r.lastTick = now - TICK;
  while (!r.doneAt && now - r.lastTick >= TICK) {
    r.lastTick += TICK;
    r.ticks++;
    const t = r.lastTick;
    const taken = new Set(r.walkers.filter((w) => !w.dieAt).map((w) => w.idx));
    const kept: Walker[] = [];
    for (const w of r.walkers) {
      if (w.dieAt) { if (t - w.dieAt < FADE) kept.push(w); continue; }
      if (!r.cells.has(w.idx)) r.cells.set(w.idx, t);
      const options = (neighbours.get(w.idx) ?? []).filter((j) => !r.cells.has(j) && !taken.has(j));
      if (!options.length) { w.dieAt = t; kept.push(w); continue; }
      const next = options[(random() * options.length) | 0];
      w.prev = w.idx; w.idx = next; w.since = t;
      taken.add(next);
      kept.push(w);
    }
    r.walkers = kept;

    const alive = kept.filter((w) => !w.dieAt).length;
    const frontier: number[] = [];
    const seen = new Set<number>();
    for (const i of r.cells.keys()) {
      for (const j of neighbours.get(i) ?? []) {
        if (!r.cells.has(j) && !taken.has(j) && !seen.has(j)) { seen.add(j); frontier.push(j); }
      }
    }
    if (!frontier.length && !alive) { r.doneAt = t; break; }
    const target = Math.min(8, Math.max(1, Math.floor((r.ticks / 3) ** 2)));
    for (let n = alive; n < target && frontier.length; n++) {
      const k = (random() * frontier.length) | 0;
      const j = frontier[k];
      frontier[k] = frontier[frontier.length - 1];
      frontier.pop();
      r.walkers.push({ idx: j, prev: j, since: t, bornAt: t, dieAt: 0 });
    }
  }
}
