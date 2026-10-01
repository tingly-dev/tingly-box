import type { ReferenceImage } from '../components/ImageGenReferenceImages';
import type { ImageProfile } from './imageProfileTypes';

// Placeholder artwork for the prototype: flat SVGs, so the library looks like
// a library without shipping binary fixtures.
// Each one is returned as raw SVG markup; mockRef() turns it into a reference
// image the playground's row accepts like any upload.
const svgMarkup = (body: string, w: number, h: number) =>
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w}" height="${h}">${body}</svg>`;

const portrait = (bg: string, skin: string, hair: string, coat: string, glasses = false) => svgMarkup(
    `<rect width="300" height="400" fill="${bg}"/>`
    + `<path d="M40 400 C50 300 100 270 150 270 C200 270 250 300 260 400 Z" fill="${coat}"/>`
    + `<rect x="132" y="225" width="36" height="50" fill="${skin}"/>`
    + `<ellipse cx="150" cy="180" rx="58" ry="68" fill="${skin}"/>`
    + `<path d="M90 185 C85 110 120 98 150 98 C185 98 215 112 210 185 C200 140 180 128 150 128 C120 128 100 140 90 185 Z" fill="${hair}"/>`
    + (glasses
        ? `<circle cx="126" cy="185" r="15" fill="none" stroke="#2a2a2a" stroke-width="4"/><circle cx="174" cy="185" r="15" fill="none" stroke="#2a2a2a" stroke-width="4"/><path d="M141 185 H159" stroke="#2a2a2a" stroke-width="4"/>`
        : ''),
    300, 400,
);

const cat = (bg: string) => svgMarkup(
    `<rect width="300" height="400" fill="${bg}"/>`
    + `<ellipse cx="150" cy="300" rx="95" ry="80" fill="#E08A3C"/>`
    + `<ellipse cx="150" cy="320" rx="45" ry="50" fill="#FBF3E8"/>`
    + `<circle cx="150" cy="190" r="70" fill="#E08A3C"/>`
    + `<path d="M92 160 L98 100 L135 135 Z M208 160 L202 100 L165 135 Z" fill="#E08A3C"/>`
    + `<circle cx="125" cy="190" r="8" fill="#2a2a2a"/><circle cx="175" cy="190" r="8" fill="#2a2a2a"/>`
    + `<path d="M143 212 L157 212 L150 220 Z" fill="#C0563B"/>`,
    300, 400,
);

const swatch = (colors: string[], shape: 'film' | 'wash' | 'poly') => {
    const [a, b, c, d] = colors;
    if (shape === 'film') {
        return svgMarkup(
            `<rect width="400" height="400" fill="${a}"/>`
            + `<rect x="0" y="250" width="400" height="150" fill="${b}"/>`
            + `<circle cx="280" cy="140" r="60" fill="${c}"/>`
            + `<rect x="60" y="170" width="70" height="130" fill="${d}"/>`
            + `<rect x="0" y="0" width="400" height="400" fill="#000" opacity="0.06"/>`,
            400, 400,
        );
    }
    if (shape === 'wash') {
        return svgMarkup(
            `<rect width="400" height="400" fill="${a}"/>`
            + `<ellipse cx="140" cy="160" rx="120" ry="90" fill="${b}" opacity="0.7"/>`
            + `<ellipse cx="260" cy="250" rx="130" ry="100" fill="${c}" opacity="0.6"/>`
            + `<ellipse cx="200" cy="330" rx="150" ry="40" fill="${d}" opacity="0.55"/>`,
            400, 400,
        );
    }
    return svgMarkup(
        `<rect width="400" height="400" fill="${a}"/>`
        + `<polygon points="0,400 140,180 260,400" fill="${b}"/>`
        + `<polygon points="140,400 290,120 400,400" fill="${c}"/>`
        + `<polygon points="200,90 260,60 300,140 230,150" fill="${d}"/>`,
        400, 400,
    );
};

const mockRef = (name: string, svg: string, width: number, height: number): ReferenceImage => ({
    file: new File([svg], `${name}.svg`, { type: 'image/svg+xml' }),
    previewUrl: `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`,
    source: 'upload',
    width,
    height,
});

const DAY = 24 * 60 * 60 * 1000;
const now = Date.now();

// PROTOTYPE seed data — what a user with a few profiles would see.
export const mockProfiles = (): ImageProfile[] => [
    {
        id: 'linxia-film',
        name: '林夏 · 胶片人像',
        refs: [
            mockRef('linxia-front', portrait('#D9DFE6', '#F1D3BC', '#1F1F24', '#CDB89A', true), 300, 400),
            mockRef('linxia-side', portrait('#E6DED3', '#EBC9B0', '#1F1F24', '#CDB89A', true), 300, 400),
            mockRef('film-look', swatch(['#E9D8BF', '#B9875A', '#F3B562', '#6B4E3D'], 'film'), 400, 400),
        ],
        basePrompt: '林夏：二十多岁的女性，齐肩黑色短发，圆框眼镜，米色风衣。35mm 胶片质感，柔和颗粒，暖色偏移，浅景深，自然光。',
        model: '',
        size: '1024x1792',
        quality: 'high',
        count: 2,
        updatedAt: now - DAY,
    },
    {
        id: 'tuanzi-book',
        name: '团子绘本',
        refs: [
            mockRef('tuanzi', cat('#E8E2D6'), 300, 400),
            mockRef('watercolor', swatch(['#F7F3EA', '#9CC5D9', '#F2B8A2', '#B7D3A8'], 'wash'), 400, 400),
        ],
        basePrompt: '团子：胖胖的橘色短毛猫，白色胸口，尾巴尖是白色。儿童绘本水彩插画，纸张纹理，柔和晕染边缘，留白多。',
        model: '',
        size: '1024x1024',
        quality: 'medium',
        count: 1,
        updatedAt: now - 4 * DAY,
    },
    {
        id: 'lowpoly-poster',
        name: '低多边形海报',
        refs: [mockRef('lowpoly', swatch(['#1E2A44', '#3E5C8A', '#6C8FC7', '#F2C14E'], 'poly'), 400, 400)],
        basePrompt: '低多边形 3D 风格，平面着色，硬朗棱角，简洁配色，横版海报构图。',
        model: '',
        size: '1792x1024',
        quality: 'auto',
        count: 1,
        updatedAt: now - 10 * DAY,
    },
];
