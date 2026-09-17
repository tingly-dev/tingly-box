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
 *
 * The tile is horizontally seamless (`stitchTiles="stitch"`, and the filter
 * region is pinned to exactly the tile's own box rather than the default
 * padded region) and consumed with `background-repeat: repeat-x` at a fixed
 * pixel height (see components/ds.ts) — never stretched to a viewport's full
 * width. A naive `background-size: 100% <height>` would smear the cloud
 * puffs into thin horizontal streaks on an ultrawide monitor and squash them
 * on a narrow one; tiling a fixed-aspect unit keeps the puffs' proportions
 * correct at every viewport width.
 *
 * No feGaussianBlur: it's the one filter primitive here that samples a pixel
 * neighborhood, and it does so within its own rendered subregion only — it
 * can't "see" the identical noise that continues past the tile edge, so it
 * fades toward transparent right at the seam instead of blending into the
 * next repeat. Every other primitive (feColorMatrix, feComponentTransfer)
 * remaps each pixel in isolation, so it can't introduce a seam; feTurbulence
 * itself is exactly periodic under stitching. The softness instead comes
 * from numOctaves/baseFrequency below and the gamma curve's ramp.
 */
const TILE_WIDTH = 480;
const TILE_HEIGHT = 640;

const CLOUD_SVG = `
<svg xmlns="http://www.w3.org/2000/svg" width="${TILE_WIDTH}" height="${TILE_HEIGHT}" viewBox="0 0 ${TILE_WIDTH} ${TILE_HEIGHT}">
  <defs>
    <filter id="clouds" x="0" y="0" width="100%" height="100%" primitiveUnits="userSpaceOnUse">
      <feTurbulence type="fractalNoise" baseFrequency="0.01 0.011" numOctaves="4" seed="11"
        stitchTiles="stitch" result="noise" />
      <feColorMatrix in="noise" type="matrix"
        values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 3.1 -1.2"
        result="puffs" />
      <feComponentTransfer in="puffs">
        <feFuncA type="gamma" amplitude="1" exponent="0.7" offset="0" />
      </feComponentTransfer>
    </filter>
    <linearGradient id="fade" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#fff" />
      <stop offset="55%" stop-color="#fff" />
      <stop offset="100%" stop-color="#000" />
    </linearGradient>
    <mask id="fadeMask">
      <rect width="${TILE_WIDTH}" height="${TILE_HEIGHT}" fill="url(#fade)" />
    </mask>
  </defs>
  <rect width="${TILE_WIDTH}" height="${TILE_HEIGHT}" filter="url(#clouds)" mask="url(#fadeMask)" />
</svg>
`.trim();

export const dsCloudTextureUrl = `url("data:image/svg+xml,${encodeURIComponent(CLOUD_SVG)}")`;
export const dsCloudTextureTileHeight = TILE_HEIGHT;
