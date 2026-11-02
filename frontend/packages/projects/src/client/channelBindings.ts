// IM channel bindings (TEP #2143, M1): which project an IM chat talks to.
// A thin layer over the generated client so the IM components import from one place.
import { api, unwrap, type components } from './index';

/** One IM chat bound to a thread; `project_id` is the project that thread belongs to. */
export type ChannelBinding = components['schemas']['ChannelBinding'];

/**
 * Where a thread event came from when an IM chat wrote it
 * (.design/projects-sessions.md, "Event"). A Web message has none.
 */
export type MessageSource = Pick<components['schemas']['ThreadEvent'], 'source' | 'source_platform' | 'source_chat_id'>;

export async function listChannelBindings(): Promise<ChannelBinding[]> {
    const res = await unwrap(api.GET('/api/v1/projects/channel-bindings', { params: { query: {} } }));
    return res.bindings ?? [];
}

/**
 * Binds a chat to a thread, a project's main thread, or a folder for a new
 * session. A chat bound elsewhere is moved; old threads stay.
 */
export async function bindChannel(req: components['schemas']['BindChannelRequest']): Promise<ChannelBinding> {
    return unwrap(api.PUT('/api/v1/projects/channel-bindings', { body: req }));
}

export async function unbindChannel(id: string): Promise<void> {
    await unwrap(api.DELETE('/api/v1/projects/channel-bindings/{id}', { params: { path: { id } } }));
}
