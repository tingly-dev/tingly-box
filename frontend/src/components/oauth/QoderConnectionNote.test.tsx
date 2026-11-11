import {renderToStaticMarkup} from 'react-dom/server';
import {describe, expect, it, vi} from 'vitest';
import QoderConnectionNote from './QoderConnectionNote';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (key: string) => key})}));

describe('QoderConnectionNote', () => {
    it.each(['qoder', 'qoder_cn'])('shows the scope notice for %s', (providerId) => {
        expect(renderToStaticMarkup(<QoderConnectionNote providerId={providerId}/>)).toContain('qoderConnection.scope');
    });

    it.each(['workbuddy', 'codex', undefined])('does not show the notice for %s', (providerId) => {
        expect(renderToStaticMarkup(<QoderConnectionNote providerId={providerId}/>)).toBe('');
    });
});
