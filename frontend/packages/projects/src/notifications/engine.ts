// Desktop notifications for the workspace: while the tab is hidden, the
// moments that need the user surface as system notifications, and clicking
// one brings them to the right place.
//
// The triggers ride the one all-projects event stream every mounted
// workspace surface already keeps alive (projectListStore): a thread whose
// status moved to `waiting` (a question, an approval, a report that needs
// input) or `failed`, a turn that finished, and the coordinator's decision
// cards (the one transcript entry the stream forwards — see the Projects
// hub). Nothing fires while the tab is visible: the page itself is the
// notification then.
import { isFolder, type Thread, type ThreadEvent } from '../client';
import { getProjectList as listRows, subscribeProjectList } from '../hooks/projectListStore';
import type { ProjectEvent } from '../hooks/useProjectEvents';
import { readLocal, writeLocal } from '../utils/storage';
import { notificationPermission, showBrowserNotification, type BrowserNotification } from './browser';

export const NOTIFICATIONS_SETTING = 'notifications';

/** What a thread's status change asks of the user. */
export type ThreadAttention = 'waiting' | 'failed' | 'finished';

export interface NotificationUi {
    /** The workspace's strings, in the host's language. */
    t: (key: string, values?: Record<string, unknown>) => string;
    navigate: (to: string) => void;
    /** Where the workspace pages live in the host's router. */
    paths: { project: (id: string) => string; folder: (path?: string) => string };
}

/** Finished-turn pings repeat while a project works; one per thread per window. */
export const FINISHED_COOLDOWN_MS = 30_000;

interface Engine {
    ui: NotificationUi | null;
    stop: () => void;
    /** The last status seen per thread; the first sight of a thread baselines it. */
    status: Map<string, string>;
    /** The last `finished` ping per thread, for the cooldown. */
    finishedAt: Map<string, number>;
    /** Open decision notifications, so an answered card closes its ping. */
    decisions: Map<string, BrowserNotification>;
}

const engine: Engine = { ui: null, stop: () => undefined, status: new Map(), finishedAt: new Map(), decisions: new Map() };

/** The user's choice; permission granted is part of "on" — a browser that
 * revoked it silently turns the feature off again. */
export function notificationsEnabled(): boolean {
    return readLocal(NOTIFICATIONS_SETTING) === '1' && notificationPermission() === 'granted';
}

/** Remembers the user's choice and starts or stops the engine to match. */
export function setNotificationsEnabled(on: boolean): void {
    writeLocal(NOTIFICATIONS_SETTING, on ? '1' : '0');
    resetBaseline();
    stopEngine();
    if (on) startEngine();
}

/** The engine does not speak without the workspace's strings and routes; the
 * bridge every mounted surface renders provides them. */
export function setNotificationUi(ui: NotificationUi): void {
    engine.ui = ui;
}

function startEngine(): void {
    if (engine.stop !== returnVoid) return;
    engine.status = new Map();
    engine.finishedAt = new Map();
    engine.stop = subscribeProjectList(returnVoid, onProjectEvent);
}

function stopEngine(): void {
    if (engine.stop === returnVoid) return;
    engine.stop();
    engine.stop = returnVoid;
    for (const n of engine.decisions.values()) n.close();
    engine.decisions.clear();
}

function resetBaseline(): void {
    engine.status = new Map();
    engine.finishedAt = new Map();
}

const returnVoid = () => undefined;

/**
 * The status change a thread event asks of the user, or null. The first
 * status seen for a thread only baselines it: what was already on screen
 * when the engine started is the app's news, not a notification's.
 */
export function classifyTransition(prev: string | undefined, next: string): ThreadAttention | null {
    if (!prev) return null;
    if (next === 'waiting') return prev === 'waiting' ? null : 'waiting';
    if (next === 'failed') return prev === 'failed' ? null : 'failed';
    if ((next === 'ready' || next === 'idle') && (prev === 'working' || prev === 'waiting')) return 'finished';
    return null;
}

/** The pending decision in a decision card's payload, if it still is one. */
export function parseDecision(ev: ThreadEvent): { question: string } | null {
    try {
        const d = JSON.parse(ev.payload ?? '') as { question?: string; chosen?: number | null };
        return d.chosen == null ? { question: d.question ?? '' } : null;
    } catch {
        return null;
    }
}

const onProjectEvent = (ev: ProjectEvent): void => {
    if (!notificationsEnabled()) return;
    if (ev.type === 'thread') onThread(ev.data as Thread);
    else if (ev.type === 'thread_event') onThreadEvent(ev.data as ThreadEvent, ev.project_id);
};

function onThread(thread: Thread): void {
    if (thread.archived_at) return;
    const prev = engine.status.get(thread.id);
    engine.status.set(thread.id, thread.status);
    const attention = classifyTransition(prev, thread.status);
    if (!attention) return;
    // The gate the feature lives by: a visible page already shows this.
    if (document.visibilityState !== 'hidden') return;
    if (attention === 'finished' && Date.now() - (engine.finishedAt.get(thread.id) ?? 0) < FINISHED_COOLDOWN_MS) return;
    const t = engine.ui?.t ?? ((key: string) => key);
    // The concrete word where there is one: why it waits, how it failed,
    // what it reported. The generic line only when the thread has neither.
    const body = attention !== 'finished' && thread.wait_reason
        ? thread.wait_reason
        : attention === 'finished' && firstLine(thread.last_report)
            ? firstLine(thread.last_report)
            : t(`notifications.${attention}`);
    engine.finishedAt.set(thread.id, Date.now());
    notify(thread.title, body, `tb-thread-${thread.id}`, () => threadUrl(thread, attention === 'waiting'));
}

function onThreadEvent(event: ThreadEvent, projectId: string): void {
    if (event.kind !== 'decision') return;
    const decision = parseDecision(event);
    if (!decision) {
        // The user picked (or the card went away): its ping no longer matters.
        engine.decisions.get(event.id)?.close();
        engine.decisions.delete(event.id);
        return;
    }
    if (document.visibilityState !== 'hidden') return;
    const t = engine.ui?.t ?? ((key: string) => key);
    const project = listRows()?.find((p) => p.id === projectId);
    const n = notify(project?.name || t('nav.projects'), decision.question || event.content, `tb-decision-${event.id}`, () => projectUrl(projectId));
    if (n) engine.decisions.set(event.id, n);
}

/** One notification: the title names the place, the body says what it wants,
 * and a click lands on the place that answers it. */
function notify(title: string, body: string, tag: string, url: () => string | undefined): BrowserNotification | null {
    const n = showBrowserNotification(clip(title) || 'Tingly Box', {
        body: clip(body),
        // One ping per thread or card: a newer replaces an older, never piles up.
        tag,
        renotify: true,
    });
    if (!n) return null;
    n.onclick = () => {
        const to = url();
        if (to) engine.ui?.navigate(to);
        window.focus();
        n.close();
    };
    return n;
}

/** Where answering a thread's ping happens: the folder session or the
 * project page with the thread open; the conversation itself is the project page.
 * A thread that waits on the user opens its card's requests on the project page
 * (`review`), where the approval or question is answered in place; the page
 * opens the thread instead when nothing answerable is pending. */
export function threadUrl(thread: Thread, review = false): string | undefined {
    const ui = engine.ui;
    const project = listRows()?.find((p) => p.id === thread.project_id);
    if (!ui || !project) return undefined;
    if (isFolder(project)) return withParams(ui.paths.folder(project.path), { session: thread.id });
    if (thread.id === project.main_thread_id) return ui.paths.project(project.id);
    return withParams(ui.paths.project(project.id), review ? { review: thread.id } : { thread: thread.id });
}

function projectUrl(projectId: string): string | undefined {
    return engine.ui && listRows()?.some((p) => p.id === projectId) ? engine.ui.paths.project(projectId) : undefined;
}

const withParams = (base: string, params: Record<string, string>) =>
    `${base}${base.includes('?') ? '&' : '?'}${Object.entries(params).map(([k, v]) => `${k}=${encodeURIComponent(v)}`).join('&')}`;

const clip = (s: string | undefined): string => (s ?? '').trim().slice(0, 120);
const firstLine = (s: string | undefined): string => (s ?? '').split('\n').find((l) => l.trim() !== '') ?? '';

// Arming at import: when the user turned the feature on, any mounted
// workspace surface starts watching (the store starts its stream for the
// first subscriber). A browser without the API, or one that revoked the
// permission, simply never starts it.
if (notificationsEnabled()) startEngine();
