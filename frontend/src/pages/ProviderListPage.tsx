import React, { useEffect, useState } from 'react';
import { api } from '@/services/api';
import { Box, Stack } from '@mui/material';
import PageHeader from '@/components/PageHeader';
import PageLayout from '@/components/PageLayout';
import Surface from '@/components/Surface';
import ProviderTable from '@/components/provider-list/ProviderTable';
import ProviderFilterBar from '@/components/provider-list/ProviderFilterBar';
import { useNotify } from '@/hooks/useNotify';

interface ProviderData {
  id: string;
  name: string;
  name_zh?: string;
  description: string;
  description_zh?: string;
  icon: string;
  website: string;
  api_doc: string;
  base_url_openai?: string;
  base_url_anthropic?: string;
}

interface FilterState {
  search: string;
  protocol: 'all' | 'openai' | 'anthropic' | 'both';
  sort: 'default' | 'name' | 'nameZh';
}

const DEFAULT_FILTER: FilterState = {
  search: '',
  protocol: 'all',
  sort: 'default',
};

const ProviderListPage: React.FC = () => {
  const [providers, setProviders] = useState<ProviderData[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<FilterState>(DEFAULT_FILTER);
  const { success } = useNotify();

  // Load providers from API
  useEffect(() => {
    const loadProviders = async () => {
      try {
        const result = await api.getProviderTemplates();
        if (result.success && result.data) {
          // result.data is a map/object, convert to array
          const providersMap = result.data as Record<string, ProviderData>;
          const providersArray = Object.values(providersMap);
          setProviders(providersArray);
        }
      } catch (error) {
        console.error('Failed to load providers:', error);
      } finally {
        setLoading(false);
      }
    };

    loadProviders();
  }, []);

  const handleFilterChange = (updates: Partial<FilterState>) => {
    setFilter((prev) => ({ ...prev, ...updates }));
  };

  // Filter providers
  const filteredProviders = providers.filter((provider) => {
    // Search filter
    const searchLower = filter.search.toLowerCase();
    const matchesSearch =
      provider.name.toLowerCase().includes(searchLower) ||
      (provider.name_zh?.toLowerCase().includes(searchLower) ?? false);

    // Protocol filter
    let matchesProtocol = true;
    if (filter.protocol === 'openai') {
      matchesProtocol = !!provider.base_url_openai && !provider.base_url_anthropic;
    } else if (filter.protocol === 'anthropic') {
      matchesProtocol = !!provider.base_url_anthropic && !provider.base_url_openai;
    } else if (filter.protocol === 'both') {
      matchesProtocol = !!provider.base_url_openai && !!provider.base_url_anthropic;
    }

    return matchesSearch && matchesProtocol;
  });

  // Sort providers
  const sortedProviders = [...filteredProviders].sort((a, b) => {
    if (filter.sort === 'name') {
      return a.name.localeCompare(b.name);
    } else if (filter.sort === 'nameZh') {
      const aZh = a.name_zh || a.name;
      const bZh = b.name_zh || b.name;
      return aZh.localeCompare(bZh, 'zh-CN');
    }
    return 0; // default order
  });

  return (
    <PageLayout loading={loading}>
      <Box sx={{ py: 3, px: { xs: 2, md: 3 } }}>
        <Stack spacing={2.5}>
          <PageHeader
            title="Provider List"
            subtitle="Browse all available AI service providers and their API documentation."
          />

          <Surface padding={{ xs: 2, sm: 2.5 }}>
            <ProviderFilterBar
              filter={filter}
              resultCount={sortedProviders.length}
              totalCount={providers.length}
              onFilterChange={handleFilterChange}
            />
            <ProviderTable
              providers={sortedProviders}
              onCopySuccess={success}
            />
          </Surface>
        </Stack>
      </Box>
    </PageLayout>
  );
};

export default ProviderListPage;
