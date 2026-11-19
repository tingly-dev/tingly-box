import type { Provider } from '@/types/provider';

export type CredentialSort = 'name' | 'provider';

/** Sort a copy by natural name, or by recognized provider then natural name. */
export function sortCredentials(providers: Provider[], locale?: string, sort: CredentialSort = 'name', category: (provider: Provider) => string | undefined = () => undefined): Provider[] {
    const collator = new Intl.Collator(locale, { numeric: true, sensitivity: 'base' });
    return [...providers].sort((a, b) => {
        if (sort === 'provider') {
            const aCategory = category(a);
            const bCategory = category(b);
            if (!aCategory && bCategory) return 1;
            if (aCategory && !bCategory) return -1;
            const categoryOrder = collator.compare(aCategory || '', bCategory || '');
            if (categoryOrder) return categoryOrder;
        }
        return collator.compare(a.name, b.name) || a.uuid.localeCompare(b.uuid);
    });
}
