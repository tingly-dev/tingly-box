// Copy-ready examples for POST /api/v1/bots/{bot}/notify. Pure string builders
// so the guide's tabs, its Send test button and the tests all work from one
// payload.

export type SampleLang = 'curl' | 'json' | 'javascript' | 'python';
export const SAMPLE_LANGS: SampleLang[] = ['curl', 'json', 'javascript', 'python'];

export interface NotifyPayload {
    target: {kind: 'direct_chat' | 'group'; id: string};
    title: string;
    body: string;
    level: 'info' | 'warn' | 'error';
}

export const SAMPLE_TOKEN_ENV = 'TINGLY_TOKEN';
export const BOT_PLACEHOLDER = '<BOT_ID>';
export const TARGET_PLACEHOLDER = '<TARGET_ID>';

export const SAMPLE_MESSAGE = {
    title: 'Hello from Tingly Box',
    body: 'This is a test notification from the API guide.',
    level: 'info' as const,
};

const indent = (text: string, prefix: string) =>
    text.split('\n').map((line, i) => (i === 0 ? line : prefix + line)).join('\n');

export function buildSample(lang: SampleLang, origin: string, botID: string, payload: NotifyPayload): string {
    const url = `${origin}/api/v1/bots/${botID}/notify`;
    const json = JSON.stringify(payload, null, 2);
    switch (lang) {
        case 'json':
            return json;
        case 'curl':
            return `curl -X POST ${url} \\
  -H "Authorization: Bearer $${SAMPLE_TOKEN_ENV}" \\
  -H "Content-Type: application/json" \\
  -d '${indent(json, '  ')}'`;
        case 'javascript':
            return `const res = await fetch("${url}", {
  method: "POST",
  headers: {
    "Authorization": \`Bearer \${process.env.${SAMPLE_TOKEN_ENV}}\`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify(${indent(json, '  ')}),
});
console.log(res.status, await res.json());`;
        case 'python':
            return `import os
import requests

res = requests.post(
    "${url}",
    headers={"Authorization": f"Bearer {os.environ['${SAMPLE_TOKEN_ENV}']}"},
    json=${indent(json, '    ')},
)
print(res.status_code, res.json())`;
    }
}

export const SAMPLE_PRISM: Record<SampleLang, string> = {
    curl: 'bash',
    json: 'json',
    javascript: 'javascript',
    python: 'python',
};

export const SAMPLE_LABELS: Record<SampleLang, string> = {
    curl: 'curl',
    json: 'JSON',
    javascript: 'JavaScript',
    python: 'Python',
};
