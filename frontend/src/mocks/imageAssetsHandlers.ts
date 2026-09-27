import { http, HttpResponse } from 'msw'

// Mock-mode image assets API (/api/v1/image-assets), in memory: starts empty
// and behaves like the server — batch saves, updates keep created_at, the
// same image bytes are kept once, content is served back as bytes.

type Piece = {
    id: string
    kind: string
    title: string
    text: string
    tags: string[]
    source_id?: string
    created_at: number
    updated_at: number
}
type Reference = {
    id: string
    name: string
    mime: string
    width?: number
    height?: number
    bytes: number
    created_at: number
    dataUrl: string
}

const pieces = new Map<string, Piece>()
const references = new Map<string, Reference>()
const newId = () => crypto.randomUUID()
const normalizeTags = (tags: string[] = []) => [...new Set(tags.map((tag) => tag.trim().toLowerCase()).filter(Boolean))]
const publicReference = ({ dataUrl: _dataUrl, ...reference }: Reference) => reference
const notFound = () => HttpResponse.json({ success: false, error: { message: 'not found', type: 'not_found_error' } }, { status: 404 })

const imageSize = (dataUrl: string): Promise<{ width?: number; height?: number }> => new Promise((resolve) => {
    const image = new Image()
    image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight })
    image.onerror = () => resolve({})
    image.src = dataUrl
})

export const imageAssetsHandlers = [
    http.get('/api/v1/image-assets/pieces', () => HttpResponse.json({
        success: true,
        pieces: [...pieces.values()].sort((a, b) => b.updated_at - a.updated_at),
    })),
    http.post('/api/v1/image-assets/pieces', async ({ request }) => {
        const body = await request.json() as { pieces: Array<Partial<Piece>> }
        const now = Date.now()
        if (body.pieces.some((input) => input.id && !pieces.has(input.id))) return notFound()
        const saved = body.pieces.map((input, index): Piece => {
            const previous = input.id ? pieces.get(input.id) : undefined
            const piece: Piece = {
                id: previous?.id ?? newId(),
                kind: input.kind ?? 'prompt',
                title: input.kind === 'prompt' ? (input.title ?? '').trim() : '',
                text: (input.text ?? '').trim(),
                tags: normalizeTags(input.tags),
                ...(input.source_id ? { source_id: input.source_id } : {}),
                created_at: previous?.created_at ?? now + index,
                updated_at: now + index,
            }
            pieces.set(piece.id, piece)
            return piece
        })
        return HttpResponse.json({ success: true, pieces: saved })
    }),
    http.delete('/api/v1/image-assets/pieces/:id', ({ params }) => (
        pieces.delete(String(params.id)) ? HttpResponse.json({ success: true }) : notFound()
    )),

    http.get('/api/v1/image-assets/references', () => HttpResponse.json({
        success: true,
        references: [...references.values()].sort((a, b) => b.created_at - a.created_at).map(publicReference),
    })),
    http.post('/api/v1/image-assets/references', async ({ request }) => {
        const body = await request.json() as { references: Array<{ name: string; data_url: string }> }
        const now = Date.now()
        const added = []
        for (const [index, upload] of body.references.entries()) {
            const existing = [...references.values()].find((reference) => reference.dataUrl === upload.data_url)
            if (existing) {
                added.push({ ...publicReference(existing), existing: true })
                continue
            }
            const blob = await (await fetch(upload.data_url)).blob()
            const reference: Reference = {
                id: newId(),
                name: upload.name.trim(),
                mime: blob.type || 'image/png',
                bytes: blob.size,
                created_at: now + index,
                dataUrl: upload.data_url,
                ...(await imageSize(upload.data_url)),
            }
            references.set(reference.id, reference)
            added.push({ ...publicReference(reference), existing: false })
        }
        return HttpResponse.json({ success: true, references: added })
    }),
    http.put('/api/v1/image-assets/references/:id', async ({ params, request }) => {
        const reference = references.get(String(params.id))
        if (!reference) return notFound()
        const { name } = await request.json() as { name: string }
        reference.name = name.trim() || reference.name
        return HttpResponse.json({ success: true, reference: publicReference(reference) })
    }),
    http.delete('/api/v1/image-assets/references/:id', ({ params }) => (
        references.delete(String(params.id)) ? HttpResponse.json({ success: true }) : notFound()
    )),
    http.get('/api/v1/image-assets/references/:id/content', async ({ params }) => {
        const reference = references.get(String(params.id))
        if (!reference) return notFound()
        const blob = await (await fetch(reference.dataUrl)).blob()
        return new HttpResponse(blob, { headers: { 'Content-Type': reference.mime } })
    }),
]
