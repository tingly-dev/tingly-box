import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import type {SessionInfo} from '@/services/deskApi';
import DeskSidebar from './DeskSidebar';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
afterEach(cleanup);

it('shows an unused project, starts its first task, and filters it by path', () => {
    const onNew = vi.fn();
    render(<DeskSidebar sessions={[]} projects={['/projects/fresh']} selectedId={null} unseen={new Set()} onSelect={vi.fn()} onNew={onNew}/>);
    fireEvent.click(screen.getByRole('button', {name: 'Create first task'}));
    expect(onNew).toHaveBeenCalledWith('/projects/fresh');
    fireEvent.change(screen.getByLabelText('Search'), {target: {value: '/projects/fresh'}});
    expect(screen.getByText('fresh')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', {name: 'Archived'}));
    expect(screen.queryByText('fresh')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', {name: 'Active'}));
    expect(screen.getByText('fresh')).toBeInTheDocument();
});

it('combines saved projects with existing task groups without duplicating them', () => {
    const sessions = [{id: 'a', project: 'C:\\projects\\app', request: 'Review app', status: 'completed'}] as SessionInfo[];
    render(<DeskSidebar sessions={sessions} projects={['C:\\projects\\app']} selectedId={null} unseen={new Set()} onSelect={vi.fn()} onNew={vi.fn()}/>);
    expect(screen.getAllByText('app')).toHaveLength(1);
    expect(screen.getByRole('button', {name: 'Review app'})).toBeInTheDocument();
    expect(screen.queryByText('Create first task')).not.toBeInTheDocument();
});
