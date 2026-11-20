import {cleanup, render, screen, within} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import type {AttachmentsComponent} from './context';
import Transcript from './Transcript';
import type {TranscriptEntry} from './model';
import {buildTranscript} from './model';
import {trajectoryRows} from './trajectoryRows';

vi.mock('react-i18next', () => ({useTranslation: () => ({i18n: {language: 'en'}, t: (_key: string, opts: {defaultValue: string; count?: number}) => opts.defaultValue.replace('{{count}}', String(opts.count))})}));
afterEach(cleanup);

const Files: AttachmentsComponent = ({items, captions}) => (
    <ul data-testid="files">{items.map((f) => <li key={f.path} title={captions?.[f.path]}>{f.name}</li>)}</ul>
);
const file = (name: string, caption = ''): TranscriptEntry => ({
    role: 'assistant', content: caption, timestamp: '', attachments: [{name, path: name, type: 'image/png', size: 1}],
});
const sendCall = (id: string): TranscriptEntry[] => [
    {kind: 'tool_use', content: 'mcp__projects__send_file', request_id: id, payload: {path: 'x'}, timestamp: ''},
    {kind: 'tool_result', content: 'sent', request_id: id, timestamp: ''},
];
const show = (entries: TranscriptEntry[]) => render(<Transcript blocks={buildTranscript(entries)} working={false} onRespond={vi.fn()} attachments={Files}/>);

describe('files sent into the conversation', () => {
    it('draws files sent one after another as one row, with their send calls folded away', () => {
        show([...sendCall('c1'), file('a.png'), ...sendCall('c2'), file('b.png'), ...sendCall('c3'), file('c.png')]);
        const group = screen.getByRole('group', {name: '3 files sent'});
        expect(within(group).getAllByRole('listitem').map((li) => li.textContent)).toEqual(['a.png', 'b.png', 'c.png']);
        expect(screen.getAllByTestId('files')).toHaveLength(1);
        // Only the call that sent the first file stays above the row.
        expect(screen.queryAllByText(/send_file/)).toHaveLength(1);
    });

    it('keeps files apart when something else was said between them', () => {
        show([file('a.png'), {role: 'assistant', content: 'And now the result', timestamp: ''}, file('b.png')]);
        expect(screen.queryByRole('group')).toBeNull();
        expect(screen.getAllByTestId('files')).toHaveLength(2);
    });

    it('does not merge past other tool calls', () => {
        show([file('a.png'), {kind: 'tool_use', content: 'Bash', request_id: 'b', payload: {command: 'ls'}, timestamp: ''}, file('b.png')]);
        expect(screen.queryByRole('group')).toBeNull();
    });

    it('keeps each caption as the file\'s own hover text, never as a message', () => {
        show([file('a.png', 'The diagram'), file('b.png'), file('c.png', 'The result')]);
        expect(screen.getByText('a.png')).toHaveAttribute('title', 'The diagram');
        expect(screen.getByText('b.png')).not.toHaveAttribute('title');
        expect(screen.getByText('c.png')).toHaveAttribute('title', 'The result');
        expect(screen.queryByText('The diagram', {selector: 'p'})).toBeNull();
        expect(screen.queryByRole('button', {name: 'Captions'})).toBeNull();
    });

    it('draws a single file as its file alone, its caption only on the file', () => {
        show([file('a.png', 'The diagram')]);
        expect(screen.queryByRole('group')).toBeNull();
        expect(screen.getByText('a.png')).toHaveAttribute('title', 'The diagram');
        expect(screen.queryByText('The diagram', {selector: 'p'})).toBeNull();
    });

    it('does not read a caption as something said in the trajectory', () => {
        const rows = trajectoryRows(buildTranscript([{role: 'user', content: 'Draw it', timestamp: ''}, file('a.png', 'The diagram')]), false);
        expect(rows.map((r) => r.title)).toEqual(['Draw it']);
    });
});
