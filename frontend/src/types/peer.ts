// Peer — an external tool registered on tingly-box as if tingly-box were an
// IM platform (.design/peer.md). Mirrors peerapi.PeerView; swap to the
// generated schema types once the client SDK exposes them.

export interface Peer {
    uuid: string;
    /** Mention word (@name) and attribution prefix (【name】). */
    name: string;
    bot_uuid: string;
    /** Bound external chat id — the binding IS the authorization. */
    chat_id: string;
    /** true = every plain message in the bound chat goes to this peer. */
    exclusive: boolean;
    enabled: boolean;
    /** A poller is connected right now. */
    online: boolean;
    created_at: string;
    updated_at: string;
}

export interface CreatePeerRequest {
    name: string;
    bot_uuid: string;
    chat_id: string;
    exclusive?: boolean;
    enabled?: boolean;
}

export interface UpdatePeerRequest {
    name?: string;
    bot_uuid?: string;
    chat_id?: string;
    exclusive?: boolean;
    enabled?: boolean;
}
