import type { ImageEntity } from './entityTypes';

// Placeholder artwork for the prototype: flat SVGs, so the library looks like
// a library without shipping binary fixtures.
const svgUrl = (body: string, w: number, h: number) =>
    `data:image/svg+xml;charset=utf-8,${encodeURIComponent(
        `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w}" height="${h}">${body}</svg>`,
    )}`;

const portrait = (bg: string, skin: string, hair: string, coat: string, glasses = false) => svgUrl(
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

const cat = (bg: string) => svgUrl(
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
        return svgUrl(
            `<rect width="400" height="400" fill="${a}"/>`
            + `<rect x="0" y="250" width="400" height="150" fill="${b}"/>`
            + `<circle cx="280" cy="140" r="60" fill="${c}"/>`
            + `<rect x="60" y="170" width="70" height="130" fill="${d}"/>`
            + `<rect x="0" y="0" width="400" height="400" fill="#000" opacity="0.06"/>`,
            400, 400,
        );
    }
    if (shape === 'wash') {
        return svgUrl(
            `<rect width="400" height="400" fill="${a}"/>`
            + `<ellipse cx="140" cy="160" rx="120" ry="90" fill="${b}" opacity="0.7"/>`
            + `<ellipse cx="260" cy="250" rx="130" ry="100" fill="${c}" opacity="0.6"/>`
            + `<ellipse cx="200" cy="330" rx="150" ry="40" fill="${d}" opacity="0.55"/>`,
            400, 400,
        );
    }
    return svgUrl(
        `<rect width="400" height="400" fill="${a}"/>`
        + `<polygon points="0,400 140,180 260,400" fill="${b}"/>`
        + `<polygon points="140,400 290,120 400,400" fill="${c}"/>`
        + `<polygon points="200,90 260,60 300,140 230,150" fill="${d}"/>`,
        400, 400,
    );
};

const DAY = 24 * 60 * 60 * 1000;
const now = Date.now();

export const MOCK_ENTITIES: ImageEntity[] = [
    {
        id: 'ent-linxia',
        kind: 'character',
        name: '林夏',
        refs: [
            portrait('#D9DFE6', '#F1D3BC', '#1F1F24', '#CDB89A', true),
            portrait('#E6DED3', '#EBC9B0', '#1F1F24', '#CDB89A', true),
            portrait('#D3D9CF', '#F1D3BC', '#1F1F24', '#8A9A86', true),
            portrait('#E9E3EC', '#F1D3BC', '#1F1F24', '#CDB89A', true),
        ],
        prompt: '二十多岁的女性，齐肩黑色短发，圆框眼镜，米色风衣，神情安静',
        uses: 12,
        updatedAt: now - 2 * DAY,
    },
    {
        id: 'ent-ayan',
        kind: 'character',
        name: '阿岩',
        refs: [
            portrait('#CFD6DC', '#D6A887', '#2B2520', '#3C4652'),
            portrait('#DCD6CC', '#D6A887', '#2B2520', '#3C4652'),
            portrait('#D2DAD2', '#D6A887', '#2B2520', '#55603F'),
        ],
        prompt: '三十岁左右的男性，寸头，深色工装夹克，左眉有一道浅疤',
        uses: 7,
        updatedAt: now - 5 * DAY,
    },
    {
        id: 'ent-tuanzi',
        kind: 'character',
        name: '团子',
        refs: [cat('#E8E2D6'), cat('#DCE3E8')],
        prompt: '胖胖的橘色短毛猫，白色胸口，尾巴尖是白色',
        uses: 5,
        updatedAt: now - 9 * DAY,
    },
    {
        id: 'ent-film',
        kind: 'style',
        name: '胶片人像',
        refs: [
            swatch(['#E9D8BF', '#B9875A', '#F3B562', '#6B4E3D'], 'film'),
            swatch(['#D6C6A8', '#8C6A4A', '#E8A95B', '#4F3B2E'], 'film'),
        ],
        prompt: '35mm 胶片质感，柔和颗粒，暖色偏移，浅景深，自然光',
        negative: '过度锐化，塑料质感的皮肤',
        uses: 18,
        updatedAt: now - 1 * DAY,
    },
    {
        id: 'ent-watercolor',
        kind: 'style',
        name: '水彩绘本',
        refs: [
            swatch(['#F7F3EA', '#9CC5D9', '#F2B8A2', '#B7D3A8'], 'wash'),
            swatch(['#F7F3EA', '#C9B6E4', '#F6D58E', '#9CC5D9'], 'wash'),
        ],
        prompt: '儿童绘本水彩插画，纸张纹理，柔和晕染边缘，留白多',
        uses: 4,
        updatedAt: now - 12 * DAY,
    },
    {
        id: 'ent-lowpoly',
        kind: 'style',
        name: '低多边形',
        refs: [swatch(['#1E2A44', '#3E5C8A', '#6C8FC7', '#F2C14E'], 'poly')],
        prompt: '低多边形 3D 风格，平面着色，硬朗棱角，简洁配色',
        uses: 2,
        updatedAt: now - 20 * DAY,
    },
];
