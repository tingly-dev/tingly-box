// The pose library: every preset as bone angles, and how they are grouped.
// Data only — add a pose here and nothing else needs to change.
import { buildPose, type PoseSpec, type PresetPoints } from './spec';

export type PosePresetKey =
    | 'standing' | 'contrapposto' | 'handsOnHips' | 'armsCrossed' | 'tPose' | 'armsUp' | 'armsBehind'
    | 'walking' | 'running' | 'jumping' | 'kicking' | 'reaching' | 'bowing'
    | 'throwing' | 'dancing' | 'climbing'
    | 'sitting' | 'sittingFloor' | 'crossLegged' | 'kneeling' | 'crouching' | 'hugKnees' | 'reclining'
    | 'lying' | 'lyingSide' | 'prone' | 'pushUp'
    | 'wave' | 'pointing' | 'thinking' | 'leaning' | 'shrug' | 'armsOpen' | 'lookingBack'
    | 'salute' | 'presenting';


const POSE_SPECS: Record<PosePresetKey, PoseSpec> = {
    standing: {
        arms: { l: [[-8, 8], [-6, 12]], r: [[8, 8], [6, 12]] },
        legs: { l: [-3, -2], r: [3, 2] },
    },
    contrapposto: {
        lean: 4, shoulderTilt: -4, hipTilt: 5, twist: -6,
        arms: { l: [[-10, 6], [-14, 10]], r: [[6, 10], [10, 14]] },
        legs: { l: [-1, 0], r: [9, 4] },
    },
    handsOnHips: {
        arms: { l: [[-52, -14], [26, 26]], r: [[52, -14], [-26, 26]] },
        legs: { l: [-5, -3], r: [5, 3] },
    },
    armsCrossed: {
        arms: { l: [[-38, -6], [62, 34]], r: [[38, -6], [-62, 34]] },
        legs: { l: [-4, -2], r: [4, 2] },
    },
    // The calibration pose: deliberately flat, so "arms out" is still the one
    // preset that shows the skeleton with nothing foreshortened.
    tPose: { arms: { l: [-90, -90], r: [90, 90] }, legs: { l: [-4, -3], r: [4, 3] } },
    armsUp: {
        arms: { l: [[-166, 12], [-176, 18]], r: [[166, 12], [176, 18]] },
        legs: { l: [-5, -4], r: [5, 4] },
    },
    armsBehind: {
        arms: { l: [[-14, -22], [34, -50]], r: [[14, -22], [-34, -50]] },
        legs: { l: [-6, -4], r: [6, 4] },
    },

    walking: {
        lean: 2,
        arms: { l: [[20, 18], [15, 20]], r: [[-20, -18], [-14, -20]] },
        legs: { l: [[-25, -14], [-12, -8]], r: [[25, 16], [14, 10]] },
    },
    running: {
        lean: -10, bend: 6,
        arms: { l: [[-28, 34], [-88, 50]], r: [[28, -30], [88, -40]] },
        legs: { l: [[-42, -40], [-74, -30]], r: [[38, 34], [24, 20]] },
    },
    jumping: {
        arms: { l: [[-158, 8], [-172, 10]], r: [[158, 8], [172, 10]] },
        legs: { l: [[-22, 26], [-48, 10]], r: [[22, 26], [48, 10]] },
    },
    kicking: {
        lean: 10, bend: -8,
        arms: { l: [[-34, 10], [-22, 14]], r: [[34, -10], [22, -14]] },
        legs: { l: [-4, -2], r: [[58, 42], [48, 30]] },
    },
    // Reaching *out of the picture* rather than off to one side: the whole
    // reason the mannequin has a third dimension.
    reaching: {
        lean: -8, bend: 8, headNod: 6,
        arms: { l: [[-12, 6], [-8, 10]], r: [[64, 58], [58, 68]] },
        legs: { l: [-6, -3], r: [8, 4] },
    },
    bowing: {
        lean: 10, bend: 44, headNod: 16,
        arms: { l: [[-6, 10], [-4, 12]], r: [[6, 10], [4, 12]] },
        legs: { l: [-3, 0], r: [3, 0] },
    },
    throwing: {
        lean: 8, twist: -26,
        arms: { l: [[-66, 42], [-58, 50]], r: [[148, -40], [118, -58]] },
        legs: { l: [[-16, -24], [-8, -14]], r: [[16, 26], [8, 12]] },
    },
    dancing: {
        lean: -10, shoulderTilt: -10, hipTilt: 12, twist: 12, headTilt: -8,
        arms: { l: [[-150, 20], [-168, 26]], r: [[60, -26], [86, -10]] },
        legs: { l: [[-10, 14], [-4, 8]], r: [[26, -10], [10, -6]] },
    },
    climbing: {
        bend: 10,
        arms: { l: [[-160, -24], [-172, -28]], r: [[30, -20], [70, -30]] },
        legs: { l: [[-6, -6], [-2, -4]], r: [[40, -30], [86, -26]] },
    },

    // Seated poses are where a flat figure fails hardest: a thigh coming
    // toward the camera is a *short* thigh, and there is no flat angle that
    // says that without breaking the bone length.
    sitting: {
        lean: 6,
        arms: { l: [[-10, 24], [16, 58]], r: [[10, 24], [-16, 58]] },
        legs: { l: [[-4, 84], [-2, 2]], r: [[4, 80], [2, 0]] },
    },
    sittingFloor: {
        lean: -14,
        arms: { l: [[-26, -30], [-10, -46]], r: [[26, -30], [10, -46]] },
        legs: { l: [[-6, 78], [-4, 74]], r: [[6, 74], [4, 70]] },
    },
    crossLegged: {
        lean: -2,
        arms: { l: [[-30, 18], [-4, 52]], r: [[30, 18], [4, 52]] },
        legs: { l: [[-74, 26], [76, 30]], r: [[74, 26], [-76, 30]] },
    },
    kneeling: {
        lean: 4,
        arms: { l: [[-14, 6], [-8, 10]], r: [[14, 6], [8, 10]] },
        legs: { l: [[4, -10], [6, -92]], r: [[10, 70], [6, 4]] },
    },
    crouching: {
        lean: -14, bend: 12,
        arms: { l: [[-18, 40], [18, 58]], r: [[18, 40], [-18, 58]] },
        legs: { l: [[-18, 58], [-8, -4]], r: [[18, 54], [8, -8]] },
    },
    hugKnees: {
        lean: -8, bend: 10, headNod: 4,
        arms: { l: [[-14, 48], [44, 46]], r: [[14, 48], [-44, 46]] },
        legs: { l: [[-10, 72], [-6, -30]], r: [[10, 70], [6, -32]] },
    },
    reclining: {
        lean: 26, bend: -8, hipTilt: -8, headTilt: -8,
        arms: { l: [[-40, -34], [-24, -56]], r: [[34, 34], [62, 30]] },
        legs: { l: [[26, 52], [-18, -10]], r: [[44, 26], [34, 10]] },
    },

    lying: {
        lean: 88,
        arms: { l: [[86, 10], [88, 12]], r: [[94, 10], [96, 12]] },
        legs: { l: [[94, 6], [92, 4]], r: [[84, 6], [82, 4]] },
    },
    lyingSide: {
        lean: 86, headTilt: 8,
        arms: { l: [[80, -30], [70, -46]], r: [[96, 30], [84, 46]] },
        legs: { l: [[92, -24], [74, -34]], r: [[88, 22], [70, 30]] },
    },
    prone: {
        lean: 86, bend: -12, headTilt: -12, headNod: -12,
        arms: { l: [[70, -40], [26, -30]], r: [[110, -40], [154, -30]] },
        legs: { l: [[92, -10], [96, -40]], r: [[86, -10], [82, -40]] },
    },
    pushUp: {
        lean: 76, bend: -6,
        arms: { l: [[-10, -70], [-8, -72]], r: [[10, -70], [8, -72]] },
        legs: { l: [[92, -8], [90, -8]], r: [[88, -8], [86, -8]] },
    },

    wave: {
        headTilt: -4,
        arms: { l: [[-8, 8], [-6, 10]], r: [[148, 14], [172, 26]] },
        legs: { l: [-4, -2], r: [4, 2] },
    },
    // Pointing at the viewer, not off to the side.
    pointing: {
        headTurn: -8,
        arms: { l: [[-10, 6], [-8, 8]], r: [[56, 62], [48, 78]] },
        legs: { l: [-4, -2], r: [6, 3] },
    },
    thinking: {
        lean: -3, headTilt: 6, headNod: 6,
        arms: { l: [[-26, -4], [64, 30]], r: [[26, 6], [-140, 44]] },
        legs: { l: [-4, -2], r: [4, 2] },
    },
    leaning: {
        lean: -12, twist: 8,
        arms: { l: [[-20, -14], [-10, -18]], r: [[16, 8], [10, 12]] },
        legs: { l: [[-6, 0], [0, 0]], r: [[11, 6], [6, 4]] },
    },
    shrug: {
        headNod: -6,
        arms: { l: [[-40, 12], [-96, 40]], r: [[40, 12], [96, 40]] },
        legs: { l: [-5, -3], r: [5, 3] },
    },
    armsOpen: {
        bend: -4,
        arms: { l: [[-104, 26], [-96, 34]], r: [[104, 26], [96, 34]] },
        legs: { l: [-6, -3], r: [6, 3] },
    },
    lookingBack: {
        lean: 4, twist: 18, headTurn: 62,
        arms: { l: [[-10, -8], [-8, -12]], r: [[10, -8], [8, -12]] },
        legs: { l: [[-6, -8], [-4, -6]], r: [[8, 6], [4, 4]] },
    },
    salute: {
        arms: { l: [[-8, 4], [-6, 6]], r: [[36, 20], [-150, 52]] },
        legs: { l: [-2, -1], r: [2, 1] },
    },
    presenting: {
        lean: 3, headTurn: -14,
        arms: { l: [[-10, 6], [-8, 8]], r: [[86, 30], [78, 40]] },
        legs: { l: [-5, -3], r: [7, 4] },
    },
};

export const POSE_PRESETS: Record<PosePresetKey, PresetPoints> = Object.fromEntries(
    (Object.keys(POSE_SPECS) as PosePresetKey[]).map((key) => [key, buildPose(POSE_SPECS[key])]),
) as Record<PosePresetKey, PresetPoints>;

// Grouped the way someone looks for a pose — by what the body is doing, not by
// how the data was authored.
export const POSE_LIBRARY: readonly { group: string; poses: readonly PosePresetKey[] }[] = [
    { group: 'standing', poses: ['standing', 'contrapposto', 'handsOnHips', 'armsCrossed', 'tPose', 'armsUp', 'armsBehind'] },
    { group: 'motion', poses: ['walking', 'running', 'jumping', 'kicking', 'reaching', 'bowing', 'throwing', 'dancing', 'climbing'] },
    { group: 'seated', poses: ['sitting', 'sittingFloor', 'crossLegged', 'kneeling', 'crouching', 'hugKnees', 'reclining'] },
    { group: 'lying', poses: ['lying', 'lyingSide', 'prone', 'pushUp'] },
    { group: 'gesture', poses: ['wave', 'pointing', 'thinking', 'leaning', 'shrug', 'armsOpen', 'lookingBack', 'salute', 'presenting'] },
];
