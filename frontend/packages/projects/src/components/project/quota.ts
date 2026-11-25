// Per-thread resource limits of a project (.design/sandbox.md §2.5): empty is
// no limit, a value has the minimums the server checks too.
export const MIN_MEMORY_BYTES = 512 * 1024 * 1024;
export const MIN_TASKS = 64;
export const MIN_CPU_MILLIS = 100;

export type MemoryUnit = 'MiB' | 'GiB';
const UNIT_BYTES: Record<MemoryUnit, number> = { MiB: 1024 ** 2, GiB: 1024 ** 3 };

export type Quota = { memory: string; unit: MemoryUnit; tasks: string; cores: string };
export type QuotaValues = { thread_memory_bytes: number; thread_tasks: number; thread_cpu: number };
export type QuotaErrors = { memory: boolean; tasks: boolean; cores: boolean };

const num = (s: string) => (/^(\d+\.?\d*|\.\d+)$/.test(s.trim()) ? Number(s) : NaN);

// The inputs showing what a project has stored (zero shows empty).
export function quotaOf(p: { thread_memory_bytes?: number; thread_tasks?: number; thread_cpu?: number }): Quota {
    const bytes = p.thread_memory_bytes ?? 0;
    const unit: MemoryUnit = bytes > 0 && bytes % UNIT_BYTES.GiB === 0 ? 'GiB' : 'MiB';
    return {
        memory: bytes > 0 ? String(+(bytes / UNIT_BYTES[unit]).toFixed(2)) : '',
        unit,
        tasks: p.thread_tasks ? String(p.thread_tasks) : '',
        cores: p.thread_cpu ? String(p.thread_cpu / 1000) : '',
    };
}

// What the inputs mean, in the server's units; NaN for one that does not parse.
export function quotaValues(q: Quota): QuotaValues {
    const blank = (s: string) => s.trim() === '';
    return {
        thread_memory_bytes: blank(q.memory) ? 0 : Math.round(num(q.memory) * UNIT_BYTES[q.unit]),
        thread_tasks: blank(q.tasks) ? 0 : /^\d+$/.test(q.tasks.trim()) ? Number(q.tasks) : NaN,
        thread_cpu: blank(q.cores) ? 0 : Math.round(num(q.cores) * 1000),
    };
}

export function quotaErrors(q: Quota): QuotaErrors {
    const v = quotaValues(q);
    const bad = (n: number, min: number) => n !== 0 && !(n >= min);
    return {
        memory: bad(v.thread_memory_bytes, MIN_MEMORY_BYTES),
        tasks: bad(v.thread_tasks, MIN_TASKS),
        cores: bad(v.thread_cpu, MIN_CPU_MILLIS),
    };
}
