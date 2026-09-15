// Peer API — external tools registered on tingly-box as if tingly-box were
// an IM platform (.design/peer.md): control-plane CRUD + token rotation.
//
// Split out of services/api.ts following the botApi.ts / modelApi.ts
// convention, and — now that codegen has caught up with the peer routes —
// goes through the generated OpenAPI client rather than raw fetch, exactly
// like botApi.ts's bot-access calls. Calls THROW on failure (PeersPage.tsx
// relies on try/catch), matching that same botAccessCall contract.
import type {CreatePeerRequest, Peer, UpdatePeerRequest} from '@/types/peer';
import type {ApiClient} from './openapi';
import {
    errorMessage,
    getControlApiClient as getClient,
    getControlApiHeaders as getAuthHeaders,
} from './openapi';

type ClientCall<T> = (client: ApiClient, headers: Record<string, string>) => Promise<{
    data?: T;
    error?: unknown;
    response: Response;
}>;

// Throws on failure so PeersPage.tsx's try/catch keeps working unchanged —
// same shape as botApi.ts's botAccessCall.
async function peerCall<T>(call: ClientCall<T>): Promise<T> {
    const client = await getClient();
    const headers = await getAuthHeaders();
    const {data, error, response} = await call(client, headers);
    if (data === undefined || error !== undefined) {
        throw new Error(errorMessage(error) || `request failed (${response.status})`);
    }
    return data;
}

export const listPeers = (): Promise<{peers: Peer[]}> =>
    peerCall((client, headers) => client.GET('/api/v1/peers', {headers})) as Promise<{peers: Peer[]}>;

export const createPeer = (body: CreatePeerRequest): Promise<{peer: Peer; token: string}> =>
    peerCall((client, headers) => client.POST('/api/v1/peers', {headers, body})) as Promise<{peer: Peer; token: string}>;

export const updatePeer = (uuid: string, body: UpdatePeerRequest): Promise<{peer: Peer}> =>
    peerCall((client, headers) => client.PUT('/api/v1/peers/{id}', {
        headers,
        params: {path: {id: uuid}},
        body,
    })) as Promise<{peer: Peer}>;

export const deletePeer = (uuid: string): Promise<{ok: boolean}> =>
    peerCall((client, headers) => client.DELETE('/api/v1/peers/{id}', {
        headers,
        params: {path: {id: uuid}},
    })) as Promise<{ok: boolean}>;

export const rotatePeerToken = (uuid: string): Promise<{token: string}> =>
    peerCall((client, headers) => client.POST('/api/v1/peers/{id}/token', {
        headers,
        params: {path: {id: uuid}},
    })) as Promise<{token: string}>;
