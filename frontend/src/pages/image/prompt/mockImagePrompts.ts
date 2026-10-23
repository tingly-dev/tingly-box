import type { ImagePromptRun } from './imagePromptStore';
import { DEFAULT_INSTRUCTIONS, INSTRUCTION_STARTERS } from './imagePromptTemplates';
import { cat, portrait, svgDataUrl, swatch } from '../profiles/mockProfiles';

const MINUTE = 60 * 1000;
const now = Date.now();

// Mock-mode seed (`pnpm dev:mock` only), never persisted.
export const mockImagePromptRuns: ImagePromptRun[] = [
    {
        id: 'seed-prompt-1',
        createdAt: now - 3 * MINUTE,
        image: svgDataUrl(portrait('#2E3A4A', '#F1D3BC', '#1F1F24', '#CDB89A', true)),
        imageName: 'rainy-night.png',
        model: 'claude-sonnet-5',
        instructions: DEFAULT_INSTRUCTIONS,
        status: 'completed',
        prompt: 'Flat vector illustration of a young woman with short black hair and a cream sweater, standing in front of a convenience store at night, head-and-shoulders portrait, centered, straight-on eye-level view. Rain streaks across a deep slate-blue background, soft warm light from the shop window falls on her face, muted palette of navy, warm beige and charcoal, clean shapes with no outlines, quiet editorial poster style.',
    },
    {
        id: 'seed-prompt-2',
        createdAt: now - 26 * MINUTE,
        image: svgDataUrl(cat('#F4E3C1')),
        imageName: 'sunbath.png',
        model: 'claude-sonnet-5',
        instructions: INSTRUCTION_STARTERS.tags,
        status: 'completed',
        prompt: 'flat illustration, orange tabby cat, curled up, sleeping, sunlight patch, wooden floor, cream background, top-down view, centered, soft shadows, warm light, pastel palette, minimal shapes, cozy, children\'s book style',
    },
    {
        id: 'seed-prompt-3',
        createdAt: now - 70 * MINUTE,
        image: svgDataUrl(swatch(['#16213A', '#2E4A78', '#8FB3E8', '#F7F0D6'], 'poly')),
        imageName: 'snow-mountain.png',
        model: 'gpt-5.6-sol',
        instructions: DEFAULT_INSTRUCTIONS,
        status: 'failed',
        prompt: '',
        error: '400: This model does not accept image input',
    },
];
