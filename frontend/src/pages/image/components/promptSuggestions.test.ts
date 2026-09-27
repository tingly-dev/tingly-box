import { describe, expect, it } from 'vitest';
import type { TFunction } from 'i18next';
import { CAMERA_ELEVATIONS, cameraTurn, createFigure, LENSES, setFigureLens } from '@tingly/mannequin';
import { insertSuggestion, promptSuggestionsFor } from './promptSuggestions';
import type { ReferenceImage } from './ImageGenReferenceImages';

// Echoes the key and its values, so the tests read what was asked for rather
// than any one language's wording.
const t = ((key: string, values?: Record<string, unknown>) =>
    values ? `${key}(${Object.entries(values).map(([k, v]) => `${k}=${v}`).join(',')})` : key) as unknown as TFunction;

const DIMS = { width: 512, height: 512 };
const file = new File([], 'x.png');
const high = CAMERA_ELEVATIONS.find((row) => row.key === 'high')!;

const sketch = (poseAs: 'skeleton' | 'mannequin' | undefined, figures = [createFigure('standing', DIMS)]): ReferenceImage => ({
    file,
    previewUrl: '',
    source: 'sketch',
    layers: { size: DIMS, strokes: [], figures, backdrop: null, ...(poseAs ? { poseAs } : {}) },
});

describe('promptSuggestionsFor', () => {
    it('offers a pose note only for sketches that have a figure, numbered by position', () => {
        const upload: ReferenceImage = { file, previewUrl: '', source: 'upload' };
        const suggestions = promptSuggestionsFor([upload, sketch('skeleton', []), sketch('skeleton')], t);
        expect(suggestions).toHaveLength(1);
        expect(suggestions[0].label).toBe('playground.suggest.pose.label(n=3)');
        expect(suggestions[0].text).toContain('pose=playground.suggest.pose.skeleton(n=3)');
    });

    it('says what was actually sent — skeleton, mannequin, or mannequin for an old sketch', () => {
        expect(promptSuggestionsFor([sketch('mannequin')], t)[0].text).toContain('pose.mannequin');
        expect(promptSuggestionsFor([sketch(undefined)], t)[0].text).toContain('pose.mannequin');
    });

    it('puts the camera the skeleton cannot carry into words, lens only when it is not standard', () => {
        const figure = setFigureLens(createFigure('standing', DIMS, undefined, 0, cameraTurn(high, 45)), LENSES[1].distance);
        const text = promptSuggestionsFor([sketch('skeleton', [figure])], t)[0].text;
        expect(text).toContain('shot.height.high');
        expect(text).toContain('shot.side.threeQuarter');
        expect(text).toContain('shot.lens.wide');
        const plain = promptSuggestionsFor([sketch('skeleton')], t)[0].text;
        expect(plain).not.toContain('shot.lens');
    });
});

describe('insertSuggestion', () => {
    it('adds its own paragraph and never rewrites what is there', () => {
        expect(insertSuggestion('', 'note')).toBe('note');
        expect(insertSuggestion('a knight  \n', 'note')).toBe('a knight\nnote');
    });
});
