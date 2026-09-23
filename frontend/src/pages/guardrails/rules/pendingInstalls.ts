// Pending registry installs are tracked in sessionStorage so an in-flight
// install survives an accidental reload without blocking retries forever.
import { PENDING_REGISTRY_INSTALLS_STORAGE_KEY } from './types';

export const readPendingRegistryInstallIds = (): Set<string> => {
    if (typeof window === 'undefined') {
        return new Set<string>();
    }
    try {
        const raw = window.sessionStorage.getItem(PENDING_REGISTRY_INSTALLS_STORAGE_KEY);
        if (!raw) {
            return new Set<string>();
        }
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) {
            return new Set<string>();
        }
        return new Set<string>(
            parsed
                .map((value) => (typeof value === 'string' ? value.trim() : ''))
                .filter(Boolean)
        );
    } catch {
        return new Set<string>();
    }
};

export const writePendingRegistryInstallIds = (pending: Set<string>) => {
    if (typeof window === 'undefined') {
        return;
    }
    try {
        if (pending.size === 0) {
            window.sessionStorage.removeItem(PENDING_REGISTRY_INSTALLS_STORAGE_KEY);
            return;
        }
        window.sessionStorage.setItem(PENDING_REGISTRY_INSTALLS_STORAGE_KEY, JSON.stringify(Array.from(pending)));
    } catch {
    }
};

export const addPendingRegistryInstallId = (policyId: string) => {
    const next = readPendingRegistryInstallIds();
    next.add(policyId);
    writePendingRegistryInstallIds(next);
};

export const removePendingRegistryInstallId = (policyId: string) => {
    const next = readPendingRegistryInstallIds();
    next.delete(policyId);
    writePendingRegistryInstallIds(next);
};
