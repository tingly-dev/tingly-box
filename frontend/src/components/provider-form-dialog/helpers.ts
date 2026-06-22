import type {UniqueProvider} from '../../services/serviceProviders';

const OPENAI_RESPONSE_HINTS = ['responses', 'response'];
const OPENAI_CHAT_HINTS = ['chat/completions', '/chat', 'chat'];

export const detectOpenAICapabilities = (provider: UniqueProvider | null): string[] => {
    if (!provider?.baseUrlOpenAI) {
        return [];
    }

    const haystacks = [
        provider.baseUrlOpenAI,
        provider.apiDoc || '',
        provider.name,
        provider.alias || '',
    ].map(value => value.toLowerCase());

    const supportsResponses = OPENAI_RESPONSE_HINTS.some(hint => haystacks.some(text => text.includes(hint)));
    const supportsChat = OPENAI_CHAT_HINTS.some(hint => haystacks.some(text => text.includes(hint)));

    const capabilities: string[] = [];
    if (supportsChat) capabilities.push('Chat');
    if (supportsResponses) capabilities.push('Responses');
    return capabilities;
};
