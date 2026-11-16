import { BAND_MIN, clampBand, clampPane, PANE_MIN } from './PaneResizer';

test('clampPane keeps the pane and the conversation usable', () => {
    expect(clampPane(200, 1440)).toBe(PANE_MIN);
    expect(clampPane(600, 1440)).toBe(600);
    expect(clampPane(1200, 1440)).toBe(960);
    expect(clampPane(800, 700)).toBe(PANE_MIN);
});

test('clampBand keeps the band readable and leaves the conversation room', () => {
    expect(clampBand(40, 900)).toBe(BAND_MIN);
    expect(clampBand(300, 900)).toBe(300);
    expect(clampBand(2000, 900)).toBe(480);
    // 700 - chrome (104) - conversation (240) leaves 356.
    expect(clampBand(2000, 700)).toBe(356);
    expect(clampBand(300, 200)).toBe(BAND_MIN);
    // Not measured yet: the window stands in.
    expect(clampBand(2000, 0)).toBe(clampBand(2000, window.innerHeight));
});
