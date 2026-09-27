import { describe, expect, it } from 'vitest';
import type { TFunction } from 'i18next';
import { CAMERA_ELEVATIONS, cameraTurn, createFigure, LENSES, setFigureLens } from '@tingly/mannequin';
import { insertSuggestion, promptSuggestionsFor } from './promptSuggestions';
import type { ReferenceImage } from './ImageGenReferenceImages';
import { effectiveRole } from './referenceRoles';

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
        expect(suggestions[0].label).toBe('playground.suggest.role.label(role=playground.referenceRole.pose,n=3)');
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

describe('reference roles', () => {
    const photo = (role?: ReferenceImage['role']): ReferenceImage => ({ file, previewUrl: '', source: 'upload', ...(role !== undefined ? { role } : {}) });

    it('says what to take from an image and what not to, once it has a role', () => {
        const suggestions = promptSuggestionsFor([photo(), photo('expression')], t);
        expect(suggestions).toHaveLength(1);
        expect(suggestions[0].text).toBe('playground.suggest.role.expression(n=2)');
        expect(suggestions[0].label).toBe('playground.suggest.role.label(role=playground.referenceRole.expression,n=2)');
    });

    it('treats a posed sketch as a pose reference unless told otherwise', () => {
        expect(effectiveRole(sketch('skeleton'))).toBe('pose');
        expect(effectiveRole(sketch('skeleton', []))).toBeNull();
        expect(effectiveRole({ ...sketch('skeleton'), role: null })).toBeNull();
        expect(promptSuggestionsFor([{ ...sketch('skeleton'), role: null }], t)).toHaveLength(0);
        // Re-labelled as something else, it gets that role's note instead.
        expect(promptSuggestionsFor([{ ...sketch('skeleton'), role: 'style' }], t)[0].text).toBe('playground.suggest.role.style(n=1)');
    });

    it('gives a pose-tagged photo the plain pose note, not the skeleton one', () => {
        expect(promptSuggestionsFor([photo('pose')], t)[0].text).toBe('playground.suggest.role.pose(n=1)');
    });

    it('offers a fresh chip when the role changes', () => {
        const [a] = promptSuggestionsFor([photo('expression')], t);
        const [b] = promptSuggestionsFor([photo('outfit')], t);
        expect(a.id).not.toBe(b.id);
    });
});

describe('insertSuggestion', () => {
    it('adds its own paragraph and never rewrites what is there', () => {
        expect(insertSuggestion('', 'note')).toBe('note');
        expect(insertSuggestion('a knight  \n', 'note')).toBe('a knight\nnote');
    });
});
