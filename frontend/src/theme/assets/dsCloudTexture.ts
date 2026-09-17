/**
 * A static (non-animated) SVG cloud texture for the ds theme's hero background.
 *
 * deepseek.com's real hero draws its cloud wisps with an animated <canvas>
 * "flow field" — not something we can reasonably run behind every dashboard/
 * table page in an admin app. This gets the same *look* — soft, irregular
 * cloud puffs, not a flat gradient band — without any runtime computation:
 * an SVG `feTurbulence` filter renders the noise once, as a plain image, the
 * same way a `background-image` renders once. `feColorMatrix` thresholds that
 * noise into puffy cloud shapes (rather than uniform haze), and a mask fades
 * them out toward the bottom so they sit near the top like the reference.
 */
const CLOUD_SVG = `
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="640" viewBox="0 0 1200 640">
  <defs>
    <filter id="clouds" x="-20%" y="-20%" width="140%" height="140%">
      <feTurbulence type="fractalNoise" baseFrequency="0.006 0.014" numOctaves="5" seed="11" result="noise" />
      <feColorMatrix in="noise" type="matrix"
        values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 3.4 -1.35"
        result="puffs" />
      <feComponentTransfer in="puffs" result="soft">
        <feFuncA type="gamma" amplitude="1" exponent="0.55" offset="0" />
      </feComponentTransfer>
      <feGaussianBlur in="soft" stdDeviation="3.5" />
    </filter>
    <linearGradient id="fade" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#fff" />
      <stop offset="55%" stop-color="#fff" />
      <stop offset="100%" stop-color="#000" />
    </linearGradient>
    <mask id="fadeMask">
      <rect width="1200" height="640" fill="url(#fade)" />
    </mask>
  </defs>
  <rect width="1200" height="640" filter="url(#clouds)" mask="url(#fadeMask)" />
</svg>
`.trim();

export const dsCloudTextureUrl = `url("data:image/svg+xml,${encodeURIComponent(CLOUD_SVG)}")`;
