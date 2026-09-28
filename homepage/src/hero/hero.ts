// Hero animation: agents and providers assemble into the Tingly Box "T".
//
// Growth starts from the Tingly Box icon. Walkers — requests — hop between free
// neighbouring cells of a T-shaped grid; every cell they land on flips in as an
// agent (left), IM channel (middle) or model provider (right). The camera
// starts close on the icon and pulls back as the T grows; once the T is
// complete it folds back into the icon while the camera pushes in again, and
// assembly restarts from there with a different mix. After a few rounds the
// hero rests on the icon and stops drawing until it is clicked or hovered for
// a long while.
//
// Idea adapted from the recursive hero on anthropic.com/institute; the
// implementation here is independent.
import { type Camera, camTarget, foldCam, follow, startCam, wideCam } from './camera';
import { DWELL, FADE, FOLD, HOLD, POP, ROUNDS, SPAN, TICK, TILE, VARIANTS } from './config';
import { ALL_TILES, layouts } from './layouts';
import { clamp01, easeInOut, rng } from './math';
import { type Round, newRound, step } from './round';
import { CENTER, N, START, cells as shapeCells, col, row } from './shape';
import { ctx2d, iconTile, loadSvg, makeCanvas, tTile, type Palette } from './tiles';

const CAPTIONS = [
  'Any agent ⇄ any provider',
  'Shared keys, rules and usage for your team',
  'Guardrails, MCP tools and remote control',
];

// farthest cell from the start, in grid steps; orders the fold
const MAX_DIST = Math.max(...shapeCells.map((i) => Math.abs(col(i) - col(START)) + Math.abs(row(i) - row(START))));

export interface HeroOptions {
  /** receives the line describing the current round */
  caption?: HTMLElement | null;
  /** the hero copy; on wide screens the T is fitted into the space to its right */
  textColumn?: HTMLElement | null;
  /** positioned label shown when hovering a tile */
  tooltip?: HTMLElement | null;
}

export function startHero(canvas: HTMLCanvasElement, { caption, textColumn, tooltip }: HeroOptions = {}): void {
  const ctx = ctx2d(canvas);
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  const icons = new Map<string, HTMLCanvasElement>();
  let brand = makeCanvas(1);
  let palette: Palette | null = null;
  let W = 0, H = 0, originX = 0, originY = 0, boxSize = 0;
  let dotColor = '#b9bcc4';
  let accent = '#2563eb';

  let round: Round = newRound(0, 0);
  const random = rng(0xc0ffee);
  // `resting`: the loop is stopped on the icon (or on the whole T, for reduced motion)
  let running = false, resting = false, visible = false, raf = 0, lastFrame = 0;
  let roundsLeft = ROUNDS - 1;       // rounds still to start after the current one
  let cam: Camera = startCam();

  function setCaption(i: number): void {
    if (caption && caption.dataset.round !== String(i)) {
      caption.dataset.round = String(i);
      caption.textContent = CAPTIONS[i % CAPTIONS.length];
    }
  }

  /** Screen position of a grid coordinate under the current camera. */
  const sx = (c: number): number => originX + (c - CENTER - cam.x) * (boxSize / SPAN) * cam.z;
  const sy = (r: number): number => originY + (r - CENTER - cam.y) * (boxSize / SPAN) * cam.z;

  // ---------- drawing ----------
  function drawTile(img: CanvasImageSource, x: number, y: number, size: number, alpha: number): void {
    if (alpha <= 0.004 || size < 0.5) return;
    ctx.globalAlpha = alpha;
    ctx.drawImage(img, x - size / 2, y - size / 2, size, size);
  }

  /** Draws the grown cells of the T. */
  function drawT(r: Round, tile: number, now: number): void {
    for (const [i, bornAt] of r.cells) {
      const x = sx(col(i));
      const y = sy(row(i));
      let s = tile;
      let a = 1;
      const age = now - bornAt;
      if (age < POP) {
        const t = age / POP;
        s *= Math.max(0, 1 + 2.70158 * (t - 1) ** 3 + 1.70158 * (t - 1) ** 2);
        a *= Math.min(1, age / 120);
      }
      const img = i === START ? brand : icons.get(layouts[r.variant][i].icon);
      if (img) drawTile(img, x, y, s, a);
    }
    ctx.globalAlpha = 1;
  }

  function dots(spacing: number, ox: number, oy: number, alpha: number): void {
    const sx = ((ox % spacing) + spacing) % spacing;
    const sy = ((oy % spacing) + spacing) % spacing;
    ctx.fillStyle = dotColor;
    ctx.globalAlpha = alpha;
    for (let y = sy; y <= H; y += spacing) for (let x = sx; x <= W; x += spacing) ctx.fillRect(x - 1, y - 1, 2, 2);
    ctx.globalAlpha = 1;
  }

  function draw(now: number): void {
    ctx.clearRect(0, 0, W, H);
    const p = (boxSize / SPAN) * cam.z;
    const tile = p * TILE;
    dots(p, sx(0.5), sy(0.5), 0.5);

    const fold = foldAt(now);
    if (fold > 0) drawFold(fold, tile);
    else drawT(round, tile, now);

    // walkers: small "requests" hopping between cells, with a short trail
    const radius = Math.max(2, Math.min(7, tile * 0.075));
    ctx.fillStyle = accent;
    ctx.strokeStyle = accent;
    ctx.lineCap = 'round';
    ctx.lineWidth = radius * 1.2;
    for (const w of round.walkers) {
      const life = w.dieAt
        ? (1 - Math.min(1, (now - w.dieAt) / FADE)) ** 2
        : 1 - (1 - Math.min(1, (now - w.bornAt) / FADE)) ** 3;
      if (life <= 0) continue;
      const e = 1 - (1 - Math.min(1, (now - w.since) / (TICK * 0.9))) ** 2;
      const ax = sx(col(w.prev)), ay = sy(row(w.prev));
      const bx = sx(col(w.idx)), by = sy(row(w.idx));
      const x = ax + (bx - ax) * e, y = ay + (by - ay) * e;
      if (e < 1) {
        ctx.globalAlpha = 0.3 * life * (1 - e);
        ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(x, y); ctx.stroke();
      }
      ctx.globalAlpha = 0.14 * life;
      ctx.beginPath(); ctx.arc(x, y, radius * 2.2, 0, Math.PI * 2); ctx.fill();
      ctx.globalAlpha = life;
      ctx.beginPath(); ctx.arc(x, y, radius, 0, Math.PI * 2); ctx.fill();
    }
    ctx.globalAlpha = 1;
  }

  /**
   * The finished T drains into the Tingly Box icon: tiles fly to the start cell
   * nearest-first and vanish into it, the icon swells as it takes them in, and
   * a ring goes out as the last one lands.
   */
  function drawFold(t: number, tile: number): void {
    const bx = sx(col(START));
    const by = sy(row(START));
    for (const [i] of round.cells) {
      if (i === START) continue;
      const d = Math.abs(col(i) - col(START)) + Math.abs(row(i) - row(START));
      const local = clamp01((t - (d / MAX_DIST) * 0.5) / 0.42);
      const e = local * local;
      const x = sx(col(i));
      const y = sy(row(i));
      const img = icons.get(layouts[round.variant][i].icon);
      if (img) drawTile(img, x + (bx - x) * e, y + (by - y) * e, tile * (1 - 0.7 * e), 1 - clamp01((e - 0.55) / 0.45));
    }
    const ring = clamp01((t - 0.62) / 0.38);
    if (ring > 0 && ring < 1) {
      const r = tile * (1 + 1.1 * easeInOut(ring));
      ctx.globalAlpha = 0.45 * (1 - ring);
      ctx.strokeStyle = accent;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.roundRect(bx - r / 2, by - r / 2, r, r, r * 0.3);
      ctx.stroke();
    }
    const swell = 1 + 0.12 * Math.sin(Math.PI * clamp01((t - 0.1) / 0.9));
    drawTile(brand, bx, by, tile * swell, 1);
    ctx.globalAlpha = 1;
  }

  // ---------- hover labels ----------
  function brandAt(i: number): { name: string; kind: string } | null {
    if (i === START) return { name: 'Tingly Box', kind: 'gateway' };
    const t = layouts[round.variant][i];
    return t ? { name: t.name, kind: t.kind === 'channel' ? 'remote channel' : t.kind } : null;
  }

  function mountTooltip(tip: HTMLElement): void {
    if (!window.matchMedia('(hover: hover)').matches) return;
    const hide = (): void => { tip.hidden = true; };
    canvas.addEventListener('pointerleave', hide);
    canvas.addEventListener('pointermove', (ev) => {
      const rect = canvas.getBoundingClientRect();
      const p = (boxSize / SPAN) * cam.z;
      const c = Math.round((ev.clientX - rect.left - originX) / p + CENTER + cam.x);
      const r = Math.round((ev.clientY - rect.top - originY) / p + CENTER + cam.y);
      const i = r * N + c;
      const folding = foldAt(performance.now()) > 0;
      const info = c >= 0 && r >= 0 && c < N && r < N && round.cells.has(i) && !folding ? brandAt(i) : null;
      if (!info) { hide(); return; }
      tip.innerHTML = '';
      const kind = document.createElement('small');
      kind.textContent = info.kind;
      tip.append(kind, info.name);
      tip.style.left = `${canvas.offsetLeft + sx(c)}px`;
      tip.style.top = `${canvas.offsetTop + sy(r) - (p * TILE) / 2}px`;
      tip.hidden = false;
    });
  }

  // ---------- loop ----------
  /** Fold progress 0..1 of the current round (0 while growing, holding or resting). */
  function foldAt(now: number): number {
    return round.doneAt && !resting ? clamp01((now - round.doneAt - HOLD) / FOLD) : 0;
  }

  function frame(now: number): void {
    if (!running) return;
    const dt = Math.min(100, now - (lastFrame || now));
    lastFrame = now;
    step(round, now, random);
    if (round.doneAt && now - round.doneAt > HOLD + FOLD) {
      // folded back into the icon: grow again, or rest there once out of rounds
      if (!roundsLeft) { rest(); return; }
      roundsLeft--;
      round = newRound((round.variant + 1) % VARIANTS, now);
      setCaption(round.variant);
    }
    const fold = foldAt(now);
    cam = fold > 0 ? foldCam(fold) : follow(cam, camTarget(round), dt);
    draw(now);
    raf = requestAnimationFrame(frame);
  }

  function pause(): void {
    running = false;
    cancelAnimationFrame(raf);
  }

  function resume(): void {
    if (resting || !visible) return;
    cancelAnimationFrame(raf);
    running = true;
    lastFrame = 0;
    raf = requestAnimationFrame(frame);
  }

  function setIdle(idle: boolean): void {
    canvas.classList.toggle('is-idle', idle);
    caption?.closest('.hero-level')?.classList.toggle('is-idle', idle);
  }

  /** Stops on the Tingly Box icon, which the next round (on wake) grows from. */
  function rest(): void {
    pause();
    resting = true;
    round = newRound((round.variant + 1) % VARIANTS, -1e6);
    round.walkers = [];
    cam = startCam();
    setIdle(true);
    draw(performance.now());
    armDwell();   // the pointer may already be resting on the icon
  }

  // a long hover also wakes the resting icon; a passing pointer does not
  let hovering = false, dwell = 0;
  function armDwell(): void {
    window.clearTimeout(dwell);
    if (hovering && resting) dwell = window.setTimeout(wake, DWELL);
  }

  /** Clicking the resting icon plays one more round. */
  function wake(): void {
    if (!resting || reducedMotion) return;
    resting = false;
    roundsLeft = 0;
    round = newRound(round.variant, performance.now());
    setCaption(round.variant);
    if (tooltip) tooltip.hidden = true;
    setIdle(false);
    resume();
  }

  /** The resting T, without animation (reduced motion). */
  function showFinal(): void {
    round = newRound(0, -1e6);
    for (const i of shapeCells) round.cells.set(i, -1e6);
    round.walkers = [];
    round.doneAt = -1e6;
    resting = true;
    cam = wideCam();
    setCaption(0);
    draw(0);
  }

  function readPalette(): Palette {
    const css = getComputedStyle(canvas);
    const read = (name: string, fallback: string): string => css.getPropertyValue(name).trim() || fallback;
    dotColor = read('--hero-dot', '#b9bcc4');
    accent = read('--accent', '#2563eb');
    return { brandBg: read('--hero-brand-bg', '#1d1f24'), brandFg: read('--hero-brand-fg', '#ffffff') };
  }

  function layout(): void {
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    W = canvas.clientWidth;
    H = canvas.clientHeight;
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingQuality = 'high';
    const wide = W >= 960;
    if (wide) {
      // fit between the text column and the right edge, with breathing room
      let textRight = W * 0.45;
      if (textColumn) {
        const left = canvas.getBoundingClientRect().left;
        textRight = Math.max(...[...textColumn.children].map((c) => c.getBoundingClientRect().right)) - left;
      }
      const from = textRight + 64;
      const to = W - Math.max(48, (W - 1200) / 2);
      originX = (from + to) / 2;
      boxSize = Math.max(160, Math.min(to - from, H * 0.78));
    } else {
      originX = W * 0.5;
      boxSize = Math.min(W * 0.9, H * 0.9);
    }
    originY = H * 0.5;
    const next = readPalette();
    if (!palette || next.brandBg !== palette.brandBg || next.brandFg !== palette.brandFg) {
      palette = next;
      brand = tTile(next.brandBg, next.brandFg);
    }
    if (!running) draw(performance.now() + (resting ? 1e6 : 0));
  }

  // ---------- boot ----------
  const urls = ALL_TILES.map((t) => t.icon);
  const fontsReady = document.fonts ? document.fonts.ready : Promise.resolve();
  Promise.all([fontsReady, ...urls.map(async (u) => icons.set(u, iconTile(await loadSvg(u))))]).then(() => {
    layout();
    new ResizeObserver(layout).observe(canvas);
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', layout);

    if (tooltip) mountTooltip(tooltip);
    if (reducedMotion) { showFinal(); return; }
    round = newRound(0, performance.now());
    cam = startCam();
    setCaption(0);
    canvas.addEventListener('click', wake);
    canvas.addEventListener('pointerenter', (ev) => { hovering = ev.pointerType === 'mouse'; armDwell(); });
    canvas.addEventListener('pointerleave', () => { hovering = false; window.clearTimeout(dwell); });
    // loop while visible, pause (keeping the current frame) while scrolled away
    new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting;
      if (visible) resume();
      else pause();
    }, { threshold: 0.15 }).observe(canvas);
  });
}
