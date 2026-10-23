import { describe, expect, it } from 'vitest';
import { fittedSize } from './preparePicture';

describe('fittedSize', () => {
    it('leaves a picture that already fits alone', () => {
        expect(fittedSize(800, 600)).toEqual({ width: 800, height: 600 });
    });

    it('scales the long side down to the limit and keeps the aspect', () => {
        expect(fittedSize(4000, 3000)).toEqual({ width: 1568, height: 1176 });
        expect(fittedSize(1000, 5000)).toEqual({ width: 314, height: 1568 });
    });
});
