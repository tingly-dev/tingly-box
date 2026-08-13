// Peer API — external tools registered on tingly-box as if tingly-box were
// an IM platform (.design/peer.md): control-plane CRUD + token rotation.
//
// Split out of services/api.ts following the botApi.ts / modelApi.ts
// convention. Calls THROW on failure (PeersPage.tsx relies on try/catch),
// matching the access-control contract in botApi.ts's botAccessCall.
//
// Raw fetch rather than the generated client for now: the backend already
// declares full swagger models for these routes (peerapi.RegisterControlRoutes),
// so this can move onto the typed client the next time schema.d.ts is
// regenerated — see CLAUDE.md's codegen note.
import type {CreatePeerRequest, Peer, UpdatePeerRequest} from '@/types/peer';
import {getApiBaseUrl} from '../utils/protocol';
import {getControlApiHeaders as getAuthHeaders} from './openapi';

async function peerCall<T>(path: string, options: RequestInit = {}): Promise<T> {
    const base = await getApiBaseUrl();
    const headers = await getAuthHeaders();
    const response = await fetch(`${base}${path}`, {
        ...options,
        headers: {...headers, 'Content-Type': 'application/json', ...(options.headers || {})},
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `request failed (${response.status})`);
    return data;
}

export const listPeers = (): Promise<{peers: Peer[]}> =>
    peerCall('/api/v1/peers');

export const createPeer = (body: CreatePeerRequest): Promise<{peer: Peer; token: string}> =>
    peerCall('/api/v1/peers', {method: 'POST', body: JSON.stringify(body)});

export const updatePeer = (uuid: string, body: UpdatePeerRequest): Promise<{peer: Peer}> =>
    peerCall(`/api/v1/peers/${encodeURIComponent(uuid)}`, {method: 'PUT', body: JSON.stringify(body)});

export const deletePeer = (uuid: string): Promise<{ok: boolean}> =>
    peerCall(`/api/v1/peers/${encodeURIComponent(uuid)}`, {method: 'DELETE'});

export const rotatePeerToken = (uuid: string): Promise<{token: string}> =>
    peerCall(`/api/v1/peers/${encodeURIComponent(uuid)}/token`, {method: 'POST'});
