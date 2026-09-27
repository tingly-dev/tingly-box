// An expression, as data: the VRM standard expression weights plus where the
// eyes look. Pure and three-free, so the preset list, the re-edit state and
// the words for the prompt can be tested without WebGL.
//
// Only the VRM 1.0 *preset* names are used — every VRM avatar has them, so
// the reference face can be swapped for another model without touching this.

export const EMOTIONS = ['happy', 'angry', 'sad', 'relaxed', 'surprised'] as const;
export type Emotion = (typeof EMOTIONS)[number];

// The rest of the presets that change the face (the look* ones are driven by
// `gaze`, and `neutral` is the absence of everything).
export type FaceShape = 'blink' | 'blinkLeft' | 'blinkRight' | 'aa' | 'ih' | 'ou' | 'ee' | 'oh';
export type ExpressionWeight = Emotion | FaceShape;

export interface ExpressionState {
    weights: Partial<Record<ExpressionWeight, number>>;
    // −1…1: left/right and down/up, as seen by the viewer.
    gaze: { x: number; y: number };
}

export const NEUTRAL: ExpressionState = { weights: {}, gaze: { x: 0, y: 0 } };

export type ExpressionPresetKey =
    | 'neutral' | 'smile' | 'happy' | 'laugh' | 'surprised'
    | 'angry' | 'sad' | 'pout' | 'wink' | 'content' | 'glance';

// Starting points, not a vocabulary: every preset opens into the same sliders.
// A few are blends, because the five emotions alone miss the expressions
// people most often ask for (a wink, a laugh, a sulk).
export const EXPRESSION_PRESETS: Record<ExpressionPresetKey, ExpressionState> = {
    neutral: NEUTRAL,
    smile: { weights: { relaxed: 1 }, gaze: { x: 0, y: 0 } },
    happy: { weights: { happy: 1 }, gaze: { x: 0, y: 0 } },
    laugh: { weights: { happy: 1, aa: 0.8 }, gaze: { x: 0, y: 0 } },
    surprised: { weights: { surprised: 1 }, gaze: { x: 0, y: 0 } },
    angry: { weights: { angry: 1 }, gaze: { x: 0, y: 0 } },
    sad: { weights: { sad: 1 }, gaze: { x: 0, y: 0 } },
    pout: { weights: { sad: 0.7, ou: 0.5 }, gaze: { x: 0, y: -0.3 } },
    wink: { weights: { happy: 0.4, blinkLeft: 1 }, gaze: { x: 0, y: 0 } },
    content: { weights: { relaxed: 0.6, blink: 1 }, gaze: { x: 0, y: 0 } },
    glance: { weights: { relaxed: 0.3 }, gaze: { x: 0.8, y: 0 } },
};

export const EXPRESSION_PRESET_KEYS = Object.keys(EXPRESSION_PRESETS) as ExpressionPresetKey[];

const EPS = 0.01;
const weightOf = (state: ExpressionState, key: ExpressionWeight) => state.weights[key] ?? 0;
const ALL_WEIGHTS: readonly ExpressionWeight[] = [...EMOTIONS, 'blink', 'blinkLeft', 'blinkRight', 'aa', 'ih', 'ou', 'ee', 'oh'];

export const sameExpression = (a: ExpressionState, b: ExpressionState): boolean =>
    ALL_WEIGHTS.every((key) => Math.abs(weightOf(a, key) - weightOf(b, key)) < EPS)
    && Math.abs(a.gaze.x - b.gaze.x) < EPS
    && Math.abs(a.gaze.y - b.gaze.y) < EPS;

// Which preset a state still is, or null once a slider has moved it.
export const presetOf = (state: ExpressionState): ExpressionPresetKey | null =>
    EXPRESSION_PRESET_KEYS.find((key) => sameExpression(state, EXPRESSION_PRESETS[key])) ?? null;

export const withWeight = (state: ExpressionState, key: ExpressionWeight, value: number): ExpressionState => {
    const weights = { ...state.weights };
    if (value <= EPS) delete weights[key];
    else weights[key] = Math.min(1, value);
    return { ...state, weights };
};

// The expression in words, for the prompt: the picture carries the exact
// face, the words say what it is — the same two channels as the pose
// skeleton. A preset is named; a hand-tuned face is described by its parts.
export type ExpressionWord = Emotion | 'eyesClosed' | 'wink' | 'mouthOpen' | 'lookingAside' | 'lookingUp' | 'lookingDown';

export interface ExpressionDescription { preset: ExpressionPresetKey | null; parts: ExpressionWord[] }

export const describeExpression = (state: ExpressionState): ExpressionDescription => {
    const preset = presetOf(state);
    if (preset) return { preset, parts: [] };
    const parts: ExpressionWord[] = EMOTIONS.filter((e) => weightOf(state, e) >= 0.25);
    if (weightOf(state, 'blink') >= 0.5) parts.push('eyesClosed');
    else if (Math.max(weightOf(state, 'blinkLeft'), weightOf(state, 'blinkRight')) >= 0.5) parts.push('wink');
    if (Math.max(weightOf(state, 'aa'), weightOf(state, 'oh'), weightOf(state, 'ou')) >= 0.3) parts.push('mouthOpen');
    if (Math.abs(state.gaze.x) >= 0.3) parts.push('lookingAside');
    if (state.gaze.y >= 0.3) parts.push('lookingUp');
    if (state.gaze.y <= -0.3) parts.push('lookingDown');
    return { preset: null, parts };
};
