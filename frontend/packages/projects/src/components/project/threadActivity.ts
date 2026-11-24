// What one thread is running now, for the status area's click-through: its
// live background tasks and its subagents, each with the id that opens it.
import { api, unwrap, type BackgroundTask, type Subagent } from '../../client';

/** A thread's running background tasks and its subagents (oldest first). */
export interface ThreadActivity {
    tasks: BackgroundTask[];
    subagents: Subagent[];
}

/**
 * Reads what a thread runs now: the live background tasks that come with the
 * thread, and its subagent list. The subagents may be unreadable on their own
 * (an old session's transcript gone): they then come back empty while the
 * tasks still load; only a failed thread read fails the whole activity.
 */
export async function loadThreadActivity(threadId: string): Promise<ThreadActivity> {
    const [detail, subagents] = await Promise.all([
        unwrap(api.GET('/api/v1/threads/{id}', { params: { path: { id: threadId } } })),
        unwrap(api.GET('/api/v1/threads/{id}/subagents', { params: { path: { id: threadId } } })).catch(() => ({ subagents: [] as Subagent[] })),
    ]);
    return { tasks: detail.background_tasks ?? [], subagents: subagents.subagents };
}
