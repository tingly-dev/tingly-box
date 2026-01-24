import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RefreshIcon from '@mui/icons-material/Refresh';
import SearchIcon from '@mui/icons-material/Search';
import {
    Box,
    Button,
    CircularProgress,
    IconButton,
    InputAdornment,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import React, { useCallback, useMemo } from 'react';
import type { Provider } from '../../types/provider';
import type { ProviderModelsDataByUuid } from '../../types/provider';
import { ModelGrid } from './ModelGrid';
import { useModelSelectContext } from '../../contexts/ModelSelectContext';
import { getModelTypeInfo } from '../../utils/modelUtils';
import { usePagination } from '../../hooks/usePagination';

export interface ModelsPanelProps {
    flattenedProviders: Provider[];
    providerModels?: ProviderModelsDataByUuid;
    selectedProvider?: string;
    selectedModel?: string;
    currentTab: number;
    refreshingProviders: string[];
    columns: number;
    modelsPerPage: number;
    onModelSelect: (provider: Provider, model: string) => void;
    onRefresh?: (provider: Provider) => void;
    onCustomModelEdit: (provider: Provider, value?: string) => void;
    onCustomModelDelete: (provider: Provider, customModel: string) => void;
    onTest?: (model: string) => void;
    testing?: boolean;
}

export function ModelsPanel({
    flattenedProviders,
    providerModels,
    selectedProvider,
    selectedModel,
    currentTab,
    refreshingProviders,
    columns,
    modelsPerPage,
    onModelSelect,
    onRefresh,
    onCustomModelEdit,
    onCustomModelDelete,
    onTest,
    testing = false,
}: ModelsPanelProps) {
    const { openCustomModelDialog } = useModelSelectContext();

    // Pagination and search - use useMemo to stabilize the provider names array
    const providerNames = React.useMemo(
        () => flattenedProviders.map(p => p.uuid),
        [flattenedProviders]
    );

    const { searchTerms, setCurrentPage, handleSearchChange, getPaginatedData } = usePagination(
        providerNames,
        modelsPerPage
    );

    const handleCustomModelEditClick = useCallback((provider: Provider, currentValue?: string) => {
        openCustomModelDialog(provider, currentValue);
    }, [openCustomModelDialog]);

    const handlePageChange = useCallback((providerUuid: string, newPage: number) => {
        setCurrentPage(providerUuid, newPage);
    }, [setCurrentPage]);

    // Only render the current tab to avoid unnecessary re-renders
    const currentProvider = flattenedProviders[currentTab];
    if (!currentProvider) return null;

    const modelTypeInfo = useMemo(
        () => getModelTypeInfo(currentProvider, providerModels, {}),
        [currentProvider, providerModels]
    );
    const { standardModelsForDisplay } = modelTypeInfo;

    const isProviderSelected = selectedProvider === currentProvider.uuid;
    const pagination = useMemo(
        () => getPaginatedData(standardModelsForDisplay, currentProvider.uuid),
        [getPaginatedData, standardModelsForDisplay, currentProvider.uuid]
    );
    const isRefreshing = refreshingProviders.includes(currentProvider.uuid);

    return (
        <Box sx={{ flex: 1, overflowY: 'auto', p: 2 }}>
            {/* Models Display */}
            <Stack spacing={2}>
                {/* Title and Controls */}
                <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                        Models ({pagination.totalItems})
                    </Typography>
                    <Stack direction="row" alignItems="center" spacing={1}>
                        <TextField
                            size="small"
                            placeholder="Search models..."
                            value={searchTerms[currentProvider.uuid] || ''}
                            onChange={(e) => handleSearchChange(currentProvider.uuid, e.target.value)}
                            slotProps={{
                                input: {
                                    startAdornment: (
                                        <InputAdornment position="start">
                                            <SearchIcon />
                                        </InputAdornment>
                                    ),
                                },
                            }}
                            sx={{ width: 300 }}
                        />
                        <Button
                            variant="outlined"
                            startIcon={<AddCircleOutlineIcon />}
                            onClick={() => handleCustomModelEditClick(currentProvider)}
                            sx={{
                                height: 40,
                                minWidth: 110,
                                borderColor: 'primary.main',
                                color: 'primary.main',
                                '&:hover': {
                                    backgroundColor: 'primary.50',
                                    borderColor: 'primary.dark',
                                }
                            }}
                        >
                            Customize
                        </Button>
                        <Button
                            variant="outlined"
                            startIcon={isRefreshing ? <CircularProgress size={16} /> : <RefreshIcon />}
                            onClick={() => onRefresh && onRefresh(currentProvider)}
                            disabled={!onRefresh || isRefreshing}
                            sx={{
                                height: 40,
                                minWidth: 110,
                                borderColor: isRefreshing ? 'grey.300' : 'primary.main',
                                color: isRefreshing ? 'grey.500' : 'primary.main',
                                '&:hover': !isRefreshing ? {
                                    backgroundColor: 'primary.50',
                                    borderColor: 'primary.dark',
                                } : {},
                                '&:disabled': {
                                    borderColor: 'grey.300',
                                    color: 'grey.500',
                                }
                            }}
                        >
                            {isRefreshing ? 'Fetching...' : 'Refresh'}
                        </Button>
                        {onTest && (
                            <Button
                                variant="outlined"
                                startIcon={testing ? <CircularProgress size={16} /> : <PlayArrowIcon />}
                                onClick={() => selectedModel && onTest(selectedModel)}
                                disabled={!selectedModel || testing}
                                sx={{
                                    height: 40,
                                    minWidth: 110,
                                    borderColor: !selectedModel || testing ? 'grey.300' : 'primary.main',
                                    color: !selectedModel || testing ? 'grey.500' : 'primary.main',
                                    '&:hover': (!selectedModel || testing) ? {} : {
                                        backgroundColor: 'primary.50',
                                        borderColor: 'primary.dark',
                                    },
                                    '&:disabled': {
                                        borderColor: 'grey.300',
                                        color: 'grey.500',
                                    }
                                }}
                            >
                                {testing ? 'Testing...' : 'Test'}
                            </Button>
                        )}
                    </Stack>
                </Stack>

                <ModelGrid
                    provider={currentProvider}
                    providerModels={providerModels}
                    selectedProvider={selectedProvider}
                    selectedModel={selectedModel}
                    onModelSelect={onModelSelect}
                    onCustomModelEdit={handleCustomModelEditClick}
                    onCustomModelDelete={onCustomModelDelete}
                    columns={columns}
                    searchTerms={searchTerms}
                    paginatedModels={pagination}
                />

                {/* Pagination Controls */}
                {pagination.totalPages > 1 && (
                    <Box sx={{ display: 'flex', justifyContent: 'center', mt: 3 }}>
                        <Stack direction="row" alignItems="center" spacing={1}>
                            <IconButton
                                size="small"
                                disabled={pagination.currentPage === 1}
                                onClick={() => handlePageChange(currentProvider.uuid, pagination.currentPage - 1)}
                            >
                                <NavigateBeforeIcon />
                            </IconButton>
                            <Typography variant="body2" sx={{ minWidth: 60, textAlign: 'center' }}>
                                {pagination.currentPage} / {pagination.totalPages}
                            </Typography>
                            <IconButton
                                size="small"
                                disabled={pagination.currentPage === pagination.totalPages}
                                onClick={() => handlePageChange(currentProvider.uuid, pagination.currentPage + 1)}
                            >
                                <NavigateNextIcon />
                            </IconButton>
                        </Stack>
                    </Box>
                )}
            </Stack>
        </Box>
    );
}

// Memoize to prevent unnecessary re-renders
const MemoizedModelsPanel = React.memo(ModelsPanel);
export default MemoizedModelsPanel;
export { ModelsPanel };
