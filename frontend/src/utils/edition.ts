/**
 * Edition utilities for controlling feature visibility based on GUI edition
 *
 * The edition is controlled by the VITE_GUI_EDITION environment variable:
 * - 'lite': Reduced feature set (hide Skills, Remote, Quick Config Apply)
 * - 'full' or undefined: Complete feature set (default)
 */

/**
 * Check if the current build is a lite edition
 * Lite edition hides: Skills, Remote Control, Quick Config Apply buttons
 */
export const isLiteEdition = import.meta.env.VITE_GUI_EDITION === 'lite';

/**
 * Check if the current build is a full edition
 * Full edition includes all features
 */
export const isFullEdition = !isLiteEdition;
