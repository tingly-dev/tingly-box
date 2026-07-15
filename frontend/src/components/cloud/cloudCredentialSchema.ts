// Cloud credential presets for the "Connect AI → Cloud" flow.
//
// This is the frontend mirror of the backend ai.CredentialSchema (ai/credential.go).
// The field keys here MUST match the canonical CredentialBundle keys the server
// validates and the SDK cloud adapters read. Keep the two in lock-step.

export type CloudFieldType = 'text' | 'password' | 'multiline';

export interface CloudField {
    /** CredentialBundle key — must match ai/credential.go exactly. */
    key: string;
    label: string;
    type: CloudFieldType;
    required?: boolean;
    /** Rendered under an "Advanced" divider; used for optional/rare fields. */
    advanced?: boolean;
    placeholder?: string;
    helper?: string;
}

export interface CloudPreset {
    id: string;
    /** Card + default provider name. */
    name: string;
    subtitle: string;
    /** ProviderIcon identifier (falls back gracefully when unknown). */
    icon: string;
    authType: 'aws_sigv4' | 'gcp_sa' | 'azure_key';
    /** Outbound protocol the gateway speaks to this provider. */
    apiStyle: 'anthropic' | 'openai' | 'google';
    fields: CloudField[];
    /**
     * The cloud SDK adapter derives the real endpoint from the credential
     * fields, but the create API still requires a non-empty api_base. Build a
     * meaningful one from the entered values so the provider list shows the
     * actual host.
     */
    buildApiBase: (v: Record<string, string>) => string;
}

export const CLOUD_PRESETS: CloudPreset[] = [
    {
        id: 'aws-bedrock',
        name: 'AWS Bedrock',
        subtitle: 'Claude on Amazon Bedrock',
        icon: 'aws',
        authType: 'aws_sigv4',
        apiStyle: 'anthropic',
        fields: [
            {key: 'region', label: 'Region', type: 'text', required: true, placeholder: 'us-east-1'},
            {key: 'access_key_id', label: 'Access Key ID', type: 'text', required: true, placeholder: 'AKIA…'},
            {key: 'secret_access_key', label: 'Secret Access Key', type: 'password', required: true},
            {key: 'session_token', label: 'Session Token', type: 'password', advanced: true, helper: 'Optional — for temporary (STS) credentials'},
            {key: 'bearer_token', label: 'Bedrock API Key', type: 'password', advanced: true, helper: 'Optional — use instead of access key / secret'},
        ],
        buildApiBase: (v) => `https://bedrock-runtime.${v.region || 'us-east-1'}.amazonaws.com`,
    },
    {
        id: 'gcp-vertex-claude',
        name: 'Vertex — Claude',
        subtitle: 'Claude on GCP Vertex AI',
        icon: 'vertexai',
        authType: 'gcp_sa',
        apiStyle: 'anthropic',
        fields: [
            {key: 'project_id', label: 'Project ID', type: 'text', required: true, placeholder: 'my-gcp-project'},
            {key: 'location', label: 'Location', type: 'text', required: true, placeholder: 'us-east5'},
            {key: 'service_account_json', label: 'Service Account JSON', type: 'multiline', required: true, placeholder: '{ "type": "service_account", … }'},
        ],
        buildApiBase: (v) => `https://${v.location || 'us-east5'}-aiplatform.googleapis.com`,
    },
    {
        id: 'gcp-vertex-gemini',
        name: 'Vertex — Gemini',
        subtitle: 'Gemini on GCP Vertex AI',
        icon: 'vertexai',
        authType: 'gcp_sa',
        apiStyle: 'google',
        fields: [
            {key: 'project_id', label: 'Project ID', type: 'text', required: true, placeholder: 'my-gcp-project'},
            {key: 'location', label: 'Location', type: 'text', required: true, placeholder: 'us-central1'},
            {key: 'service_account_json', label: 'Service Account JSON', type: 'multiline', required: true, placeholder: '{ "type": "service_account", … }'},
        ],
        buildApiBase: (v) => `https://${v.location || 'us-central1'}-aiplatform.googleapis.com`,
    },
    {
        id: 'azure-openai',
        name: 'Azure OpenAI',
        subtitle: 'GPT / o-series on Azure',
        icon: 'azure',
        authType: 'azure_key',
        apiStyle: 'openai',
        fields: [
            {key: 'endpoint', label: 'Endpoint', type: 'text', required: true, placeholder: 'https://my-resource.openai.azure.com'},
            {key: 'api_version', label: 'API Version', type: 'text', required: true, placeholder: '2024-10-21'},
            {key: 'api_key', label: 'API Key', type: 'password', required: true},
            {key: 'deployment', label: 'Deployment', type: 'text', advanced: true, helper: 'Optional — when the model name differs from the deployment name'},
        ],
        buildApiBase: (v) => v.endpoint || 'https://azure.openai.azure.com',
    },
];

export function getCloudPreset(id: string): CloudPreset | undefined {
    return CLOUD_PRESETS.find((p) => p.id === id);
}
