// Which brand icon sits in which cell of the T, one layout per round variant.

import { AGENTS, CHANNELS, PROVIDERS } from '../data/brands';
import { VARIANTS } from './config';
import { rng, shuffled } from './math';
import { N, START, cells as shapeCells, side } from './shape';

export type Kind = 'agent' | 'channel' | 'provider';
export interface Tile { icon: string; name: string; kind: Kind }

// Every distinct icon once (the OpenAI / Anthropic / DeepSeek logos appear as
// both agent-side SDKs and providers; the first claim wins).
export const ALL_TILES: Tile[] = (() => {
  const seen = new Set<string>();
  const out: Tile[] = [];
  const add = (kind: Kind, list: typeof AGENTS): void => {
    for (const b of list) if (!seen.has(b.icon)) { seen.add(b.icon); out.push({ icon: b.icon, name: b.name, kind }); }
  };
  add('provider', PROVIDERS);
  add('agent', AGENTS);
  add('channel', CHANNELS);
  return out;
})();

// Tile per cell for each round. Cells prefer their side's kind (agents left,
// channels centre, providers right); a side that runs short borrows unused
// icons from the rest, so nothing repeats within a T until every icon is used.
// Each round reshuffles, so consecutive rounds show different mixes.
export const layouts: Tile[][] = Array.from({ length: VARIANTS }, (_, v) => {
  const r = rng(1000 + v * 97);
  const unused = new Set(shuffled(ALL_TILES, r));
  const take = (kind: Kind): Tile => {
    if (!unused.size) ALL_TILES.forEach((t) => unused.add(t));   // every icon used: start over
    let pick: Tile | undefined;
    for (const t of unused) if (t.kind === kind) { pick = t; break; }
    pick ??= unused.values().next().value as Tile;
    unused.delete(pick);
    return pick;
  };
  const out: Tile[] = new Array(N * N);
  // fill in random cell order so borrowed icons do not always land in the same spots
  for (const i of shuffled(shapeCells.filter((c) => c !== START), r)) out[i] = take(side(i));
  return out;
});
