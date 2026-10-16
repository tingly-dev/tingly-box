// GitHub integration API (see .design/github.md). A small, typed slice of the
// generated OpenAPI client; callers use try/catch (these throw on failure).
import type {components} from '@/client';
import type {ApiClient} from './openapi';
import {
    errorMessage,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
} from './openapi';

export type GitHubStatus = components['schemas']['ConnectionStatus'];
export type GitHubInstallation = components['schemas']['InstallationInfo'];
export type GitHubConnectPlan = components['schemas']['ConnectPlan'];

type ClientCall<T> = (client: ApiClient, headers: Record<string, string>) => Promise<{
    data?: T;
    error?: unknown;
    response: Response;
}>;

async function call<T>(fn: ClientCall<T>): Promise<T> {
    const client = await getClient();
    const headers = await getAuthHeaders();
    const {data, error, response} = await fn(client, headers);
    if (error !== undefined) {
        throw new Error(errorMessage(error) || `request failed (${response.status})`);
    }
    return data as T;
}

export const getGitHubStatus = (live = false): Promise<GitHubStatus> =>
    call((client, headers) => client.GET('/api/v1/github/status', {
        headers,
        params: {query: live ? {live: true} : {}},
    }));

export const startGitHubConnect = (ownerType: 'user' | 'org', org: string): Promise<GitHubConnectPlan> =>
    call((client, headers) => client.POST('/api/v1/github/connect', {
        headers,
        body: {owner_type: ownerType, org, base_url: window.location.origin},
    }));

export const syncGitHub = (): Promise<void> =>
    call((client, headers) => client.POST('/api/v1/github/sync', {headers})).then(() => undefined);

export const disconnectGitHub = (): Promise<void> =>
    call((client, headers) => client.DELETE('/api/v1/github', {headers})).then(() => undefined);

export const setGitHubToken = (token: string): Promise<void> =>
    call((client, headers) => client.PUT('/api/v1/github/pat', {headers, body: {token}})).then(() => undefined);

export const clearGitHubToken = (): Promise<void> =>
    call((client, headers) => client.DELETE('/api/v1/github/pat', {headers})).then(() => undefined);

/**
 * Hands the manifest to GitHub. GitHub only accepts it as a browser form POST,
 * so this navigates away from the page.
 */
export function submitManifest(plan: GitHubConnectPlan): void {
    const form = document.createElement('form');
    form.method = 'post';
    form.action = plan.action_url;
    const field = document.createElement('input');
    field.type = 'hidden';
    field.name = 'manifest';
    field.value = plan.manifest;
    form.appendChild(field);
    document.body.appendChild(form);
    form.submit();
}
