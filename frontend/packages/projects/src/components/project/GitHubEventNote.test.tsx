import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Transcript from '../../transcript/Transcript';
import { buildTranscript } from '../../transcript/model';
import { GitHubEventNote, coordinatorEventOf } from './GitHubEventNote';
import { threadTranscript } from './threadTranscript';
import type { ThreadEvent } from '../../client';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(cleanup);

const note = (payload: Record<string, unknown>) => ({
    id: 'n', kind: 'system', content: 'PR #11 was merged into main. Since then: no uncommitted changes.', timestamp: '', payload,
});

describe('GitHubEventNote', () => {
    it('says the coordinator was told and leads to its input', () => {
        const onShowReply = vi.fn();
        render(<GitHubEventNote message={note({ github: 'pr_merged', coordinator_event: 'in-1' })} onShowReply={onShowReply} />);
        expect(screen.getByText(/PR #11 was merged into main/)).toBeInTheDocument();
        expect(screen.getByText('thread.githubEvent.told')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'thread.githubEvent.reply' }));
        expect(onShowReply).toHaveBeenCalledWith('in-1');
    });

    it('draws nothing of its own for a note nobody was told of, or a plain system note', () => {
        expect(coordinatorEventOf(note({ github: 'pr_merged' }))).toBeUndefined();
        expect(coordinatorEventOf(note({ coordinator_event: 'x' }))).toBeUndefined();
        expect(coordinatorEventOf({ ...note({ github: 'pr_merged', coordinator_event: 'x' }), payload: '{"github":"pr_merged","coordinator_event":"x"}' })).toBe('x');
        const { container } = render(<GitHubEventNote message={note({ github: 'pr_merged' })} />);
        expect(container).toBeEmptyDOMElement();
    });

    it('is how a thread transcript shows the event, from the stored payload', () => {
        const ev: ThreadEvent = { id: 'e', kind: 'system', thread_id: 't', created_at: '', updated_at: '', content: 'PR #11 was merged into main.', payload: JSON.stringify({ github: 'pr_merged', item: 'acme/w#11', status: 'ready', resolved: false, coordinator_event: 'in-1' }) };
        const blocks = buildTranscript(threadTranscript([ev], false, () => undefined).entries);
        render(<Transcript blocks={blocks} working={false} onRespond={vi.fn()} renderCustom={(m) => coordinatorEventOf(m) ? <GitHubEventNote message={m} /> : null} />);
        expect(screen.getByText('PR #11 was merged into main.')).toBeInTheDocument();
        expect(screen.getByText('thread.githubEvent.told')).toBeInTheDocument();
    });
});
