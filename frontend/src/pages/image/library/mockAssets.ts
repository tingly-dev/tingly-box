import type { ImageAsset, PromptSnippet } from './assetTypes';
import { cat, mockProfiles, portrait, svgDataUrl, swatch } from '../profiles/mockProfiles';

const DAY = 24 * 60 * 60 * 1000;
const now = Date.now();

// Mock-mode seed (`pnpm dev:mock` only): a few kept results and the mock profiles' references.
const profileRefs: ImageAsset[] = mockProfiles().flatMap((profile, p) => profile.refs.map((ref, i) => ({
    id: `seed-ref-${p}-${i}`,
    src: ref.previewUrl,
    name: ref.file.name.replace(/\.svg$/, '.png'),
    width: ref.width,
    height: ref.height,
    createdAt: now - (10 + p) * DAY,
})));

const kept = (id: string, name: string, svg: string, width: number, height: number, daysAgo: number): ImageAsset => ({
    id, name, src: svgDataUrl(svg), width, height, createdAt: now - daysAgo * DAY,
});

export const mockAssets: ImageAsset[] = [
    kept('seed-out-1', '便利店雨夜-1.png', portrait('#2E3A4A', '#F1D3BC', '#1F1F24', '#CDB89A', true), 300, 400, 0.2),
    kept('seed-out-2', '咖啡馆窗边-2.png', portrait('#E9D3A8', '#F1D3BC', '#1F1F24', '#CDB89A', true), 300, 400, 1),
    kept('seed-out-3', '晒太阳-1.png', cat('#F4E3C1'), 300, 400, 2),
    kept('seed-out-4', '雪山-1.png', swatch(['#16213A', '#2E4A78', '#8FB3E8', '#F7F0D6'], 'poly'), 400, 400, 6),
    ...profileRefs,
    {
        id: 'seed-ref-old',
        name: 'linxia-old.png',
        src: svgDataUrl(portrait('#E3E3E3', '#F1D3BC', '#3A2A20', '#7A8C9E')),
        width: 300,
        height: 400,
        createdAt: now - 30 * DAY,
    },
];

export const mockSnippets: PromptSnippet[] = [
    { id: 'snip-film', name: '胶片质感', text: '35mm 胶片质感，柔和颗粒，暖色偏移，浅景深', createdAt: now - DAY },
    { id: 'snip-watercolor', name: '水彩绘本', text: '儿童绘本水彩插画，纸张纹理，柔和晕染边缘，留白多', createdAt: now - 3 * DAY },
    { id: 'snip-cinema', name: '宽银幕', text: '电影感构图，2.39:1 宽银幕，低饱和，冷暖对比', createdAt: now - 8 * DAY },
];
