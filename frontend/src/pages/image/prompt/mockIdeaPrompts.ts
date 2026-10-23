import type { IdeaPromptRun } from './ideaPromptStore';
import { DEFAULT_IDEA_INSTRUCTIONS, IDEA_STARTERS } from './ideaPromptTemplates';

const MINUTE = 60 * 1000;
const now = Date.now();

// Mock-mode seed (`pnpm dev:mock` only), never persisted.
export const mockIdeaPromptRuns: IdeaPromptRun[] = [
    {
        id: 'seed-idea-1',
        createdAt: now - 4 * MINUTE,
        description: '一只猫在月球上看地球',
        model: 'claude-sonnet-5',
        instructions: DEFAULT_IDEA_INSTRUCTIONS,
        status: 'completed',
        prompt: 'Cinematic digital painting of a small orange tabby cat sitting on the grey dusty surface of the Moon, seen from behind, looking up at a glowing blue Earth rising over the horizon. Footprints trail behind it through fine regolith, a few craters in the foreground. Wide shot, low angle, shallow depth of field on the cat. Cool rim light from the Earth outlines its fur against a deep black, star-filled sky. Palette of slate grey, deep navy and soft cyan with a warm touch on the cat. Quiet, wondering mood.',
    },
    {
        id: 'seed-idea-2',
        createdAt: now - 31 * MINUTE,
        description: 'cozy coffee shop poster, autumn',
        model: 'claude-sonnet-5',
        instructions: IDEA_STARTERS.tags,
        status: 'completed',
        prompt: 'flat vector poster, cozy coffee shop interior, autumn, large window, falling maple leaves outside, steaming latte on wooden counter, potted plants, warm pendant lights, eye-level view, centered composition, soft afternoon light, orange and brown palette, cream background, minimal shapes, grain texture, editorial poster',
    },
    {
        id: 'seed-idea-3',
        createdAt: now - 75 * MINUTE,
        description: 'product shot of a glass perfume bottle on wet black stone',
        model: 'local-llama',
        instructions: DEFAULT_IDEA_INSTRUCTIONS,
        status: 'failed',
        prompt: '',
        error: '502: upstream connection refused',
    },
];
