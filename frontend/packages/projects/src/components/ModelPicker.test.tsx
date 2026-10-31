import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Composer } from './Composer';
import { ModelPicker } from './ModelPicker';
import { ConversationPane } from './project/ConversationPane';
import { invalidateModels } from '../hooks/useModels';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

const { fetchModels } = vi.hoisted(() => ({ fetchModels: vi.fn() }));
vi.mock('../client', async (actual) => ({ ...await actual<typeof import('../client')>(), api: { GET: fetchModels } }));

// A host that resolves routes (Tingly Box, like its status line) lists the
// alias it rules ask for together with the real model each stands for.
const hostModels = [
    { id: 'sonnet', display_name: 'sonnet', provider_model: 'claude-sonnet-4-5', provider_name: 'work' },
    { id: 'claude-opus-4-6', display_name: 'claude-opus-4-6' },
];

beforeEach(() => {
    invalidateModels();
    fetchModels.mockReset();
    fetchModels.mockImplementation(async (path: string) => {
        const id = path.endsWith('/projects/models') ? 'host-model' : 'chat-model';
        return { data: { models: [{ id, display_name: id }] }, response: new Response() };
    });
});
afterEach(cleanup);

it('uses the host model list in project composers and worker pickers', async () => {
    const choose = vi.fn();
    const view = render(<ConversationPane projectId="p1" threadId="main" messages={[]} threads={new Map()} busy={false}
        disabled={false} model="" onModelChange={choose} effort="" onEffortChange={choose} onSend={vi.fn()} onOpenThread={vi.fn()} onStartSuggestions={vi.fn()} onChooseDecision={vi.fn()} />);
    await waitFor(() => expect(fetchModels).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole('button', { name: /model\.auto/ }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'host-model' }));
    expect(choose).toHaveBeenCalledWith('host-model');
    expect(fetchModels).toHaveBeenCalledWith('/api/v1/projects/models');
    // The coordinator's effort picker sits beside its model picker.
    expect(screen.getByRole('button', { name: 'conversation.coordinatorEffort · effort.default' })).toBeInTheDocument();
    view.unmount();

    render(<ModelPicker source="projects" value="" defaultLabel="Project default · host-model" onChange={choose} />);
    fireEvent.click(screen.getByRole('button', { name: 'Project default · host-model' }));
    expect(await screen.findByRole('menuitem', { name: 'Project default · host-model' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'host-model' })).toBeInTheDocument();
    expect(fetchModels).toHaveBeenCalledOnce();
});

it('can restore the inherited model without confusing an explicit matching model', async () => {
    const choose = vi.fn();
    render(<ModelPicker source="projects" value="host-model" defaultLabel="Project default · host-model" onChange={choose} />);
    fireEvent.click(screen.getByRole('button', { name: 'host-model' }));
    const reset = await screen.findByRole('menuitem', { name: 'Project default · host-model' });
    expect(reset).not.toHaveClass('Mui-selected');
    expect(screen.getByRole('menuitem', { name: 'host-model' })).toHaveClass('Mui-selected');
    fireEvent.click(reset);
    expect(choose).toHaveBeenCalledWith('');
});

it('keeps standalone Chats on their connection and allows custom model IDs', async () => {
    const choose = vi.fn();
    render(<Composer model="" onModelChange={choose} onSend={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'model.auto' }));
    expect(await screen.findByRole('menuitem', { name: 'chat-model' })).toBeInTheDocument();
    expect(fetchModels).toHaveBeenCalledWith('/api/v1/models');
    expect(screen.queryByRole('menuitem', { name: 'model.auto' })).not.toBeInTheDocument();
    const input = screen.getByPlaceholderText('model.customPlaceholder');
    fireEvent.change(input, { target: { value: 'custom-model' } });
    fireEvent.submit(input.closest('form')!);
    expect(choose).toHaveBeenCalledWith('custom-model');
});

it('shows what an alias resolves to, the way the status line routes it', async () => {
    fetchModels.mockImplementation(async () => ({ data: { models: hostModels }, response: new Response() }));
    const choose = vi.fn();
    render(<ModelPicker source="projects" value="sonnet" onChange={choose} />);
    // The button shows the alias and the real model behind it (once the list arrives).
    fireEvent.click(await screen.findByRole('button', { name: 'sonnet → claude-sonnet-4-5' }));
    // The menu keeps the pickable alias and shows the route as its second line.
    const alias = await screen.findByRole('menuitem', { name: 'sonnet claude-sonnet-4-5 @ work' });
    expect(alias).toHaveClass('Mui-selected');
    // An id that is its own model shows no routing line.
    expect(screen.getByRole('menuitem', { name: 'claude-opus-4-6' })).toBeInTheDocument();
    fireEvent.click(alias);
    expect(choose).toHaveBeenCalledWith('sonnet');
});

it('shows the provider route for an inherited project model without changing the selected value', async () => {
    fetchModels.mockImplementation(async () => ({ data: { models: hostModels }, response: new Response() }));
    const choose = vi.fn();
    render(<ModelPicker source="projects" value="" inheritedModel="sonnet" defaultLabel="Project default · sonnet" onChange={choose} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Project default · sonnet → claude-sonnet-4-5' }));
    expect(screen.getByRole('menuitem', { name: 'Project default · sonnet → claude-sonnet-4-5' })).toHaveClass('Mui-selected');
    fireEvent.click(screen.getByRole('menuitem', { name: 'claude-opus-4-6' }));
    expect(choose).toHaveBeenCalledWith('claude-opus-4-6');
});
