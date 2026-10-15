import { clampPane, PANE_MIN } from './PaneResizer';

test('clampPane keeps the pane and the conversation usable', () => {
    expect(clampPane(200, 1440)).toBe(PANE_MIN);
    expect(clampPane(600, 1440)).toBe(600);
    expect(clampPane(1200, 1440)).toBe(960);
    expect(clampPane(800, 700)).toBe(PANE_MIN);
});
