import { describe, expect, it } from 'vitest';
import {
    describeExpression,
    EXPRESSION_PRESET_KEYS,
    EXPRESSION_PRESETS,
    NEUTRAL,
    presetOf,
    withWeight,
} from './expressionState';

describe('expression presets', () => {
    it('are all distinct, so a face is never two presets at once', () => {
        for (const key of EXPRESSION_PRESET_KEYS) expect(presetOf(EXPRESSION_PRESETS[key])).toBe(key);
    });

    it('stop being a preset once a slider moves', () => {
        const nudged = withWeight(EXPRESSION_PRESETS.happy, 'angry', 0.3);
        expect(presetOf(nudged)).toBeNull();
        // …and become one again when it is put back.
        expect(presetOf(withWeight(nudged, 'angry', 0))).toBe('happy');
    });

    it('keep weights within 0…1 and drop the ones at zero', () => {
        expect(withWeight(NEUTRAL, 'happy', 1.4).weights.happy).toBe(1);
        expect('happy' in withWeight(EXPRESSION_PRESETS.happy, 'happy', 0).weights).toBe(false);
    });
});

describe('describeExpression', () => {
    it('names a preset', () => {
        expect(describeExpression(EXPRESSION_PRESETS.wink)).toEqual({ preset: 'wink', parts: [] });
    });

    it('describes a hand-tuned face by its visible parts', () => {
        const face = { weights: { happy: 0.6, surprised: 0.5, blink: 1 }, gaze: { x: -0.5, y: 0.4 } };
        expect(describeExpression(face)).toEqual({ preset: null, parts: ['happy', 'surprised', 'eyesClosed', 'lookingAside', 'lookingUp'] });
    });

    it('ignores traces too faint to see', () => {
        const face = { weights: { angry: 0.1 }, gaze: { x: 0.1, y: 0 } };
        expect(describeExpression(face).parts).toEqual([]);
    });
});
