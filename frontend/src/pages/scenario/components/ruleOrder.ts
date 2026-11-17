/**
 * Pure list-order moves for the rule list. Both work on uuids, never on the
 * list the user saw when the gesture started: a refetch may have changed the
 * list mid-drag, so each move is resolved against the latest order.
 * They return null when there is nothing to do (unknown uuid, no-op move).
 */

export function moveBefore(order: readonly string[], activeId: string, overId: string): string[] | null {
    const from = order.indexOf(activeId);
    const to = order.indexOf(overId);
    if (from < 0 || to < 0 || from === to) return null;
    const next = [...order];
    next.splice(from, 1);
    next.splice(to, 0, activeId);
    return next;
}

export function pinToTop(order: readonly string[], uuid: string): string[] | null {
    const from = order.indexOf(uuid);
    if (from <= 0) return null;
    return [uuid, ...order.slice(0, from), ...order.slice(from + 1)];
}
