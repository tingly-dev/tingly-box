import { describe, expect, it } from 'vitest';
import { connectionSummary } from './toolPresentation';

describe('connectionSummary', () => {
    it('flags what a connection still needs', () => {
        expect(connectionSummary({ id: 'a', transport: 'http', endpoint: '' }).missing).toEqual(['endpoint']);
        expect(connectionSummary({ id: 'a', transport: 'stdio', command: '' }).missing).toEqual(['command']);
        expect(connectionSummary({ id: 'a', transport: 'advisor', advisor: {} }).missing).toEqual(['advisorModel']);
        expect(
            connectionSummary({ id: 'a', transport: 'http', endpoint: 'https://x', headers: { Authorization: '' } }).missing
        ).toEqual(['headerValue']);
    });
    it('summarises what is set', () => {
        const s = connectionSummary({
            id: 'a',
            transport: 'stdio',
            command: 'npx',
            args: ['-y', 'srv'],
            env: { KEY: 'v' },
        });
        expect(s.target).toBe('npx -y srv');
        expect(s.details).toEqual([{ key: 'env', count: 1 }]);
        expect(s.missing).toEqual([]);
    });
});
