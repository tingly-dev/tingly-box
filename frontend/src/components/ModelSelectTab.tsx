import { CheckCircle } from '@mui/icons-material';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import NavigateBeforeIcon from '@mui/icons-material/NavigateBefore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import RefreshIcon from '@mui/icons-material/Refresh';
import SearchIcon from '@mui/icons-material/Search';
import {
    Box,
    Button,
    Card,
    CardContent,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    InputAdornment,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import IconButton from '@mui/material/IconButton';
import React, { useEffect, useState } from 'react';
import { dispatchCustomModelUpdate, listenForCustomModelUpdates, useCustomModels } from '../hooks/useCustomModels';
import type { Provider, ProviderModelsData } from '../types/provider';

export interface ProviderSelectTabOption {
    provider: Provider;
    model?: string;
}

interface ProviderSelectTabProps {
    providers: Provider[];
    providerModels?: ProviderModelsData;
    selectedProvider?: string;
    selectedModel?: string;
    activeTab?: number;
    onSelected?: (option: ProviderSelectTabOption) => void;
    onRefresh?: (provider: Provider) => void;
    onCustomModelSave?: (provider: Provider, customModel: string) => void;
}


interface TabPanelProps {
    children?: React.ReactNode;
    index: number;
    value: number;
}

function TabPanel(props: TabPanelProps) {
    const { children, value, index, ...other } = props;

    return (
        <div
            role="tabpanel"
            hidden={value !== index}
            id={`provider-tabpanel-${index}`}
            aria-labelledby={`provider-tab-${index}`}
            {...other}
        >
            {value === index && (
                <Box sx={{ py: 3 }}>
                    {children}
                </Box>
            )}
        </div>
    );
}

function a11yProps(index: number) {
    return {
        id: `provider-tab-${index}`,
        'aria-controls': `provider-tabpanel-${index}`,
    };
}

export default function ModelSelectTab({
    providers,
    providerModels,
    selectedProvider,
    selectedModel,
    activeTab: externalActiveTab,
    onSelected,
    onRefresh,
    onCustomModelSave,
}: ProviderSelectTabProps) {
    const [internalCurrentTab, setInternalCurrentTab] = useState(0);
    const [isInitialized, setIsInitialized] = useState(false);
    const { customModels, saveCustomModel, removeCustomModel, getCustomModels } = useCustomModels();

    // Calculate grid layout based on viewport size
    const calculateGridLayout = () => {
        const viewportWidth = window.innerWidth;
        const viewportHeight = window.innerHeight;

        // Reserve space for UI elements (header, tabs, search, pagination, etc.)
        const headerHeight = 280; // Approximate height for headers, tabs, search, etc.
        const availableHeight = viewportHeight - headerHeight;

        // Card dimensions including gaps
        const cardWidth = 140;
        const cardHeight = 80; // 60px card height + 20px gap
        const minGap = 8;

        // Calculate columns based on viewport width
        const maxColumns = Math.floor((viewportWidth - 100) / (cardWidth + minGap)); // Reserve 100px for padding
        const columns = Math.max(3, Math.min(8, maxColumns)); // Between 3-8 columns

        // Calculate rows based on available height
        const maxRows = Math.floor(availableHeight / cardHeight);
        const rows =  Math.min(2, maxRows);

        const modelsPerPage = columns * rows;

        return {
            columns,
            rows,
            modelsPerPage: Math.max(12, Math.min(48, modelsPerPage)), // Ensure reasonable range
            cardWidth: `${100 / columns}%` // Responsive width
        };
    };

    const [gridLayout, setGridLayout] = useState(calculateGridLayout());
    const [modelsPerPage, setModelsPerPage] = useState(gridLayout.modelsPerPage);

    // Update grid layout when window resizes
    useEffect(() => {
        const handleResize = () => {
            const newLayout = calculateGridLayout();
            setGridLayout(newLayout);
            setModelsPerPage(newLayout.modelsPerPage);
        };

        window.addEventListener('resize', handleResize);
        return () => window.removeEventListener('resize', handleResize);
    }, []);

    // Use external activeTab if provided, otherwise use internal state
    const currentTab = externalActiveTab !== undefined ? externalActiveTab : internalCurrentTab;
    const [searchTerms, setSearchTerms] = useState<{ [key: string]: string }>({});
    const [currentPage, setCurrentPage] = useState<{ [key: string]: number }>({});
    const [customModelDialog, setCustomModelDialog] = useState<{ open: boolean; provider: Provider | null; value: string }>({
        open: false,
        provider: null,
        value: ''
    });

    // Listen for custom model updates from other components
    useEffect(() => {
        const cleanup = listenForCustomModelUpdates((providerName: string, modelName: string) => {
            // Update local state when custom model is updated elsewhere
            // The hook will automatically handle state updates
        });
        return cleanup;
    }, []);

    // Enhanced save function that also saves to local storage
    const handleCustomModelSave = () => {
        const customModel = customModelDialog.value?.trim();
        if (customModel && customModelDialog.provider) {
            // Save to local storage using hook
            if (saveCustomModel(customModelDialog.provider.name, customModel)) {
                // Notify other components of the change
                dispatchCustomModelUpdate(customModelDialog.provider.name, customModel);
            }

            // Then save to persistence through parent component
            if (onCustomModelSave) {
                onCustomModelSave(customModelDialog.provider, customModel);
            }
            // Select the custom model
            if (onSelected) {
                onSelected({ provider: customModelDialog.provider, model: customModel });
            }
        }
        setCustomModelDialog({ open: false, provider: null, value: '' });
    };

    const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
        if (externalActiveTab === undefined) {
            setInternalCurrentTab(newValue);
        }

        // Auto-navigate to page containing selected model when switching tabs
        const targetProvider = (providers || []).filter(provider => provider.enabled)[newValue];
        if (targetProvider && selectedProvider === targetProvider.name && selectedModel) {
            const models = providerModels?.[targetProvider.name]?.models || [];
            const modelIndex = models.indexOf(selectedModel);

            if (modelIndex !== -1) {
                const targetPage = Math.floor(modelIndex / modelsPerPage) + 1;
                const currentPageForProvider = currentPage[targetProvider.name] || 1;

                // Only update if we're not already on the correct page
                if (currentPageForProvider !== targetPage) {
                    setCurrentPage(prev => ({ ...prev, [targetProvider.name]: targetPage }));
                }
            }
        }
    };

    const handleModelSelect = (provider: Provider, model: string) => {
        if (onSelected) {
            onSelected({ provider: provider, model });
        }
    };

    const handleSearchChange = (providerName: string, searchTerm: string) => {
        setSearchTerms(prev => ({ ...prev, [providerName]: searchTerm }));
        // Reset to first page when searching
        setCurrentPage(prev => ({ ...prev, [providerName]: 1 }));
    };

    const handlePageChange = (providerName: string, page: number) => {
        setCurrentPage(prev => ({ ...prev, [providerName]: page }));
    };

    // Delete custom model handler
    const handleDeleteCustomModel = (provider: Provider, customModel: string) => {
        removeCustomModel(provider.name, customModel);
        dispatchCustomModelUpdate(provider.name, customModel);
    };

    const handleCustomModelEdit = (provider: Provider, currentValue?: string) => {
        setCustomModelDialog({
            open: true,
            provider,
            value: currentValue || ''
        });
    };

    const handleCustomModelCancel = () => {
        setCustomModelDialog({ open: false, provider: null, value: '' });
    };

    const getTotalModelsCount = (provider: Provider) => {
        const models = providerModels?.[provider.name]?.models || [];
        const starModels = providerModels?.[provider.name]?.star_models || [];
        const backendCustomModel = providerModels?.[provider.name]?.custom_model;
        const localStorageCustomModels = customModels[provider.name] || [];

        // Count unique models (avoid double counting starred models that are also in models array)
        const uniqueModels = new Set(models);

        // Add starred models if not already counted
        starModels.forEach(model => uniqueModels.add(model));

        // Add backend custom model if it exists
        if (backendCustomModel) {
            uniqueModels.add(backendCustomModel);
        }

        // Add local storage custom models
        localStorageCustomModels.forEach(model => uniqueModels.add(model));

        return uniqueModels.size;
    };

    const isCustomModel = (model: string, provider: Provider) => {
        const models = providerModels?.[provider.name]?.models || [];
        const starModels = providerModels?.[provider.name]?.star_models || [];
        const backendCustomModel = providerModels?.[provider.name]?.custom_model;
        const localStorageCustomModels = customModels[provider.name] || [];

        return !models.includes(model) &&
            !starModels.includes(model) &&
            model !== '' &&
            model !== backendCustomModel &&
            !localStorageCustomModels.includes(model);
    };

    const getAllModelsForSearch = (provider: Provider) => {
        const models = providerModels?.[provider.name]?.models || [];
        const starModels = providerModels?.[provider.name]?.star_models || [];
        const backendCustomModel = providerModels?.[provider.name]?.custom_model;
        const localStorageCustomModels = customModels[provider.name] || [];

        // Combine all models into a single array for searching
        const allModels = [...models];

        // Add starred models that are not already in the models array
        starModels.forEach(model => {
            if (!allModels.includes(model)) {
                allModels.push(model);
            }
        });

        // Add backend custom model if it exists
        if (backendCustomModel && !allModels.includes(backendCustomModel)) {
            allModels.push(backendCustomModel);
        }

        // Add local storage custom models
        localStorageCustomModels.forEach(model => {
            if (!allModels.includes(model)) {
                allModels.push(model);
            }
        });

        return allModels;
    };

    const getFilteredModels = (provider: Provider) => {
        const models = providerModels?.[provider.name]?.models || [];
        const searchTerm = searchTerms[provider.name] || '';
        if (!searchTerm) return models;

        return models.filter(model =>
            model.toLowerCase().includes(searchTerm.toLowerCase())
        );
    };

    // New function to get all filtered models including custom models
    const getFilteredAllModels = (provider: Provider) => {
        const allModels = getAllModelsForSearch(provider);
        const searchTerm = searchTerms[provider.name] || '';
        if (!searchTerm) return allModels;

        return allModels.filter(model =>
            model.toLowerCase().includes(searchTerm.toLowerCase())
        );
    };

    const getPaginatedModels = (provider: Provider) => {
        const filteredModels = getFilteredModels(provider);
        const page = currentPage[provider.name] || 1;
        const startIndex = (page - 1) * modelsPerPage;
        const endIndex = startIndex + modelsPerPage;

        return {
            models: filteredModels.slice(startIndex, endIndex),
            totalPages: Math.ceil(filteredModels.length / modelsPerPage),
            currentPage: page,
            totalModels: filteredModels.length,
        };
    };

    // Get models to display in "All Models" section (excluding custom models which are shown separately)
    const getStandardModelsForDisplay = (provider: Provider) => {
        const allModels = getAllModelsForSearch(provider);
        const starModels = providerModels?.[provider.name]?.star_models || [];
        const backendCustomModel = providerModels?.[provider.name]?.custom_model;
        const localStorageCustomModels = customModels[provider.name] || [];

        // Filter out custom models and starred models
        return allModels.filter(model => {
            // Exclude backend custom model
            if (model === backendCustomModel) {
                return false;
            }
            // Exclude local storage custom models
            if (localStorageCustomModels.includes(model)) {
                return false;
            }
            // Exclude starred models
            if (starModels.includes(model)) {
                return false;
            }
            return true;
        });
    };

    // New pagination function for all models including custom models
    const getPaginatedAllModels = (provider: Provider) => {
        const filteredAllModels = getFilteredAllModels(provider);
        const page = currentPage[provider.name] || 1;
        const startIndex = (page - 1) * modelsPerPage;
        const endIndex = startIndex + modelsPerPage;

        return {
            models: filteredAllModels.slice(startIndex, endIndex),
            totalPages: Math.ceil(filteredAllModels.length / modelsPerPage),
            currentPage: page,
            totalModels: filteredAllModels.length,
        };
    };

    // Pagination function for standard models only (excluding custom and starred models)
    const getPaginatedStandardModels = (provider: Provider) => {
        const standardModels = getStandardModelsForDisplay(provider);
        const searchTerm = searchTerms[provider.name] || '';

        let filteredModels = standardModels;
        if (searchTerm) {
            filteredModels = standardModels.filter(model =>
                model.toLowerCase().includes(searchTerm.toLowerCase())
            );
        }

        const page = currentPage[provider.name] || 1;
        const startIndex = (page - 1) * modelsPerPage;
        const endIndex = startIndex + modelsPerPage;

        return {
            models: filteredModels.slice(startIndex, endIndex),
            totalPages: Math.ceil(filteredModels.length / modelsPerPage),
            currentPage: page,
            totalModels: filteredModels.length,
        };
    };

    // Auto-switch to selected provider tab and navigate to selected model on component mount (only once)
    React.useEffect(() => {
        if (!isInitialized && selectedProvider) {
            const enabledProviders = (providers || []).filter(provider => provider.enabled);
            const targetProviderIndex = enabledProviders.findIndex(provider => provider.name === selectedProvider);

            // Auto-switch to the selected provider's tab
            if (targetProviderIndex !== -1) {
                if (externalActiveTab === undefined) {
                    setInternalCurrentTab(targetProviderIndex);
                }

                // Auto-navigate to selected model if also provided
                if (selectedModel) {
                    const targetProvider = enabledProviders[targetProviderIndex];
                    const models = providerModels?.[targetProvider.name]?.models || [];
                    const modelIndex = models.indexOf(selectedModel);

                    if (modelIndex !== -1) {
                        const targetPage = Math.floor(modelIndex / modelsPerPage) + 1;

                        setCurrentPage(prev => ({ ...prev, [targetProvider.name]: targetPage }));
                    }
                }
            }

            // Mark as initialized to prevent further automatic switching
            setIsInitialized(true);
        }
    }, [isInitialized, selectedProvider, selectedModel, providers, providerModels, externalActiveTab]); // Include dependencies

    return (
        <Box sx={{ width: '100%' }}>
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
                <Tabs
                    value={currentTab}
                    onChange={handleTabChange}
                    aria-label="Credential selection tabs"
                    variant="scrollable"
                    scrollButtons="auto"
                    allowScrollButtonsMobile
                >
                    {(providers || []).filter(provider => provider.enabled).map((provider, index) => {
                        const models = providerModels?.[provider.name]?.models || [];
                        const totalModelsCount = getTotalModelsCount(provider);
                        const isProviderSelected = selectedProvider === provider.name;

                        return (
                            <Tab
                                key={provider.name}
                                label={
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <Typography variant="body1" fontWeight={600}>
                                            {provider.name}
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            ({totalModelsCount})
                                        </Typography>
                                        {isProviderSelected && (
                                            <CheckCircle color="primary" sx={{ fontSize: 16 }} />
                                        )}
                                    </Stack>
                                }
                                {...a11yProps(index)}
                                sx={{
                                    textTransform: 'none',
                                    minWidth: 120,
                                    '&.Mui-selected': {
                                        color: 'primary.main',
                                        fontWeight: 600,
                                    },
                                }}
                            />
                        );
                    })}
                </Tabs>
            </Box>

            {(providers || []).filter(provider => provider.enabled).map((provider, index) => {
                const models = providerModels?.[provider.name]?.models || [];
                const starModels = providerModels?.[provider.name]?.star_models || [];
                const isProviderSelected = selectedProvider === provider.name;
                const pagination = getPaginatedStandardModels(provider);

                return (
                    <TabPanel key={provider.name} value={currentTab} index={index}>
                        {/* Search and Pagination Controls */}
                        <Box sx={{ mb: 3 }}>
                            <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={2}>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <TextField
                                        size="small"
                                        placeholder="Search models..."
                                        value={searchTerms[provider.name] || ''}
                                        onChange={(e) => handleSearchChange(provider.name, e.target.value)}
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
                                        onClick={() => handleCustomModelEdit(provider)}
                                        sx={{
                                            height: 40,
                                            borderColor: 'primary.main',
                                            color: 'primary.main',
                                            '&:hover': {
                                                backgroundColor: 'primary.50',
                                                borderColor: 'primary.dark',
                                            }
                                        }}
                                    >
                                        Custom Model
                                    </Button>
                                    <Button
                                        variant="outlined"
                                        startIcon={<RefreshIcon />}
                                        onClick={() => onRefresh && onRefresh(provider)}
                                        disabled={!onRefresh}
                                        sx={{
                                            height: 40,
                                            borderColor: 'primary.main',
                                            color: 'primary.main',
                                            '&:hover': {
                                                backgroundColor: 'primary.50',
                                                borderColor: 'primary.dark',
                                            },
                                            '&:disabled': {
                                                borderColor: 'grey.300',
                                                color: 'grey.500',
                                            }
                                        }}
                                    >
                                        Fetch Model List
                                    </Button>
                                </Stack>

                                {/* Pagination Controls */}
                                {pagination.totalPages > 1 && (
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <IconButton
                                            size="small"
                                            disabled={pagination.currentPage === 1}
                                            onClick={() => handlePageChange(provider.name, pagination.currentPage - 1)}
                                        >
                                            <NavigateBeforeIcon />
                                        </IconButton>
                                        <Typography variant="body2" sx={{ minWidth: 60, textAlign: 'center' }}>
                                            {pagination.currentPage} / {pagination.totalPages}
                                        </Typography>
                                        <IconButton
                                            size="small"
                                            disabled={pagination.currentPage === pagination.totalPages}
                                            onClick={() => handlePageChange(provider.name, pagination.currentPage + 1)}
                                        >
                                            <NavigateNextIcon />
                                        </IconButton>
                                    </Stack>
                                )}
                            </Stack>
                        </Box>

                        {/* Models Display */}
                        <Stack spacing={2}>
                            {/* Star Models Section */}
                            {starModels.length > 0 && (
                                <Box>
                                    <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                                        Starred Models
                                    </Typography>
                                    <Box
                                        sx={{
                                            display: 'grid',
                                            gridTemplateColumns: `repeat(${gridLayout.columns}, 1fr)`,
                                            gap: 0.8,
                                        }}
                                    >
                                        {starModels.map((starModel) => {
                                            const isModelSelected = isProviderSelected && selectedModel === starModel;
                                            return (
                                                <Card
                                                    key={starModel}
                                                    sx={{
                                                        width: '100%',
                                                        height: 60,
                                                        border: 1,
                                                        borderColor: isModelSelected ? 'primary.main' : 'warning.main',
                                                        borderRadius: 1.5,
                                                        backgroundColor: isModelSelected ? 'primary.50' : 'warning.50',
                                                        cursor: 'pointer',
                                                        transition: 'all 0.2s ease-in-out',
                                                        position: 'relative',
                                                        boxShadow: isModelSelected ? 2 : 0,
                                                        '&:hover': {
                                                            backgroundColor: isModelSelected ? 'primary.100' : 'warning.100',
                                                            boxShadow: 2,
                                                        },
                                                    }}
                                                    onClick={() => handleModelSelect(provider, starModel)}
                                                >
                                                    <CardContent sx={{
                                                        textAlign: 'center',
                                                        py: 1,
                                                        px: 0.8,
                                                        display: 'flex',
                                                        alignItems: 'center',
                                                        justifyContent: 'center',
                                                        height: '100%'
                                                    }}>
                                                        <Typography
                                                            variant="body2"
                                                            sx={{
                                                                fontWeight: 500,
                                                                fontSize: '0.75rem',
                                                                lineHeight: 1.3,
                                                                wordBreak: 'break-word',
                                                                textAlign: 'center'
                                                            }}
                                                        >
                                                            {starModel}
                                                        </Typography>
                                                        {isModelSelected && (
                                                            <CheckCircle
                                                                color="primary"
                                                                sx={{
                                                                    position: 'absolute',
                                                                    top: 4,
                                                                    right: 4,
                                                                    fontSize: 16
                                                                }}
                                                            />
                                                        )}
                                                    </CardContent>
                                                </Card>
                                            );
                                        })}
                                    </Box>
                                </Box>
                            )}


                            {/* All Models Section */}
                            <Box
                                sx={{
                                    minHeight: 200, // Minimum height to prevent layout shifts
                                }}
                            >
                                <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                                    All Models ({pagination.totalModels})
                                </Typography>
                                <Box
                                    sx={{
                                        display: 'grid',
                                        gridTemplateColumns: `repeat(${gridLayout.columns}, 1fr)`,
                                        gap: 0.8,
                                    }}
                                >
                                    {/* Custom models from local storage - prioritized */}
                                    {customModels[provider.name]?.map((customModel, index) => (
                                        <Card
                                            key={`localStorage-custom-model-${index}`}
                                            sx={{
                                                width: '100%',
                                                height: 60,
                                                border: 1,
                                                borderColor: 'primary.main',
                                                borderRadius: 1.5,
                                                backgroundColor: 'primary.50',
                                                cursor: 'pointer',
                                                transition: 'all 0.2s ease-in-out',
                                                position: 'relative',
                                                boxShadow: 0,
                                                '&:hover': {
                                                    backgroundColor: 'primary.100',
                                                    boxShadow: 2,
                                                },
                                            }}
                                            onClick={() => {
                                                if (customModel && onSelected) {
                                                    onSelected({ provider, model: customModel });
                                                }
                                            }}
                                        >
                                            <CardContent sx={{
                                                textAlign: 'center',
                                                py: 1,
                                                px: 0.8,
                                                display: 'flex',
                                                alignItems: 'center',
                                                justifyContent: 'center',
                                                height: '100%',
                                                position: 'relative'
                                            }}>
                                                <Typography
                                                    variant="body2"
                                                    sx={{
                                                        fontWeight: 500,
                                                        fontSize: '0.75rem',
                                                        lineHeight: 1.3,
                                                        wordBreak: 'break-word',
                                                        textAlign: 'center'
                                                    }}
                                                >
                                                    {customModel}
                                                </Typography>
                                                {isProviderSelected && selectedModel === customModel && (
                                                    <CheckCircle
                                                        color="primary"
                                                        sx={{
                                                            position: 'absolute',
                                                            top: 4,
                                                            right: 4,
                                                            fontSize: 16
                                                        }}
                                                    />
                                                )}
                                                <IconButton
                                                    size="small"
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        handleCustomModelEdit(provider, customModel);
                                                    }}
                                                    sx={{
                                                        position: 'absolute',
                                                        bottom: 4,
                                                        right: 4,
                                                        p: 0.5,
                                                        backgroundColor: 'background.paper',
                                                        '&:hover': {
                                                            backgroundColor: 'grey.100',
                                                        }
                                                    }}
                                                >
                                                    <EditIcon fontSize="small" />
                                                </IconButton>
                                                <IconButton
                                                    size="small"
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        handleDeleteCustomModel(provider, customModel);
                                                    }}
                                                    sx={{
                                                        position: 'absolute',
                                                        bottom: 4,
                                                        left: 4,
                                                        p: 0.5,
                                                        backgroundColor: 'background.paper',
                                                        '&:hover': {
                                                            backgroundColor: 'grey.100',
                                                        }
                                                    }}
                                                    title="Delete custom model"
                                                >
                                                    <DeleteIcon fontSize="small" />
                                                </IconButton>
                                            </CardContent>
                                        </Card>
                                    ))}

                                    {/* Persisted custom model card (from backend) */}
                                    {providerModels?.[provider.name]?.custom_model &&
                                        (!customModels[provider.name] || customModels[provider.name].length === 0) && (
                                            <Card
                                                key="persisted-custom-model"
                                                sx={{
                                                    width: '100%',
                                                    height: 60,
                                                    border: 1,
                                                    borderColor: 'primary.main',
                                                    borderRadius: 1.5,
                                                    backgroundColor: 'primary.50',
                                                    cursor: 'pointer',
                                                    transition: 'all 0.2s ease-in-out',
                                                    position: 'relative',
                                                    boxShadow: 0,
                                                    '&:hover': {
                                                        backgroundColor: 'primary.100',
                                                        boxShadow: 2,
                                                    },
                                                }}
                                                onClick={() => {
                                                    const customModel = providerModels?.[provider.name]?.custom_model;
                                                    if (customModel && onSelected) {
                                                        onSelected({ provider, model: customModel });
                                                    }
                                                }}
                                            >
                                                <CardContent sx={{
                                                    textAlign: 'center',
                                                    py: 1,
                                                    px: 0.8,
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    height: '100%',
                                                    position: 'relative'
                                                }}>
                                                    <Typography
                                                        variant="body2"
                                                        sx={{
                                                            fontWeight: 500,
                                                            fontSize: '0.75rem',
                                                            lineHeight: 1.3,
                                                            wordBreak: 'break-word',
                                                            textAlign: 'center'
                                                        }}
                                                    >
                                                        {providerModels?.[provider.name]?.custom_model}
                                                    </Typography>
                                                    {isProviderSelected && selectedModel === providerModels?.[provider.name]?.custom_model && (
                                                        <CheckCircle
                                                            color="primary"
                                                            sx={{
                                                                position: 'absolute',
                                                                top: 4,
                                                                right: 4,
                                                                fontSize: 16
                                                            }}
                                                        />
                                                    )}
                                                    <IconButton
                                                        size="small"
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            handleCustomModelEdit(provider, providerModels?.[provider.name]?.custom_model);
                                                        }}
                                                        sx={{
                                                            position: 'absolute',
                                                            bottom: 4,
                                                            right: 4,
                                                            p: 0.5,
                                                            backgroundColor: 'background.paper',
                                                            '&:hover': {
                                                                backgroundColor: 'grey.100',
                                                            }
                                                        }}
                                                    >
                                                        <EditIcon fontSize="small" />
                                                    </IconButton>
                                                </CardContent>
                                            </Card>
                                        )}

                                    {/* Currently selected custom model card (not persisted) */}
                                    {isProviderSelected && selectedModel && isCustomModel(selectedModel, provider) &&
                                        (!customModels[provider.name] || !customModels[provider.name].includes(selectedModel)) &&
                                        selectedModel !== providerModels?.[provider.name]?.custom_model && (
                                            <Card
                                                key="selected-custom-model"
                                                sx={{
                                                    width: '100%',
                                                    height: 60,
                                                    border: 1,
                                                    borderColor: 'primary.main',
                                                    borderRadius: 1.5,
                                                    backgroundColor: 'primary.50',
                                                    cursor: 'pointer',
                                                    transition: 'all 0.2s ease-in-out',
                                                    position: 'relative',
                                                    boxShadow: 2,
                                                    '&:hover': {
                                                        backgroundColor: 'primary.100',
                                                        boxShadow: 2,
                                                    },
                                                }}
                                            >
                                                <CardContent sx={{
                                                    textAlign: 'center',
                                                    py: 1,
                                                    px: 0.8,
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    height: '100%',
                                                    position: 'relative'
                                                }}>
                                                    <Typography
                                                        variant="body2"
                                                        sx={{
                                                            fontWeight: 500,
                                                            fontSize: '0.75rem',
                                                            lineHeight: 1.3,
                                                            wordBreak: 'break-word',
                                                            textAlign: 'center'
                                                        }}
                                                    >
                                                        {selectedModel}
                                                    </Typography>
                                                    <CheckCircle
                                                        color="primary"
                                                        sx={{
                                                            position: 'absolute',
                                                            top: 4,
                                                            right: 4,
                                                            fontSize: 16
                                                        }}
                                                    />
                                                    <IconButton
                                                        size="small"
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            handleCustomModelEdit(provider, selectedModel);
                                                        }}
                                                        sx={{
                                                            position: 'absolute',
                                                            bottom: 4,
                                                            right: 4,
                                                            p: 0.5,
                                                            backgroundColor: 'background.paper',
                                                            '&:hover': {
                                                                backgroundColor: 'grey.100',
                                                            }
                                                        }}
                                                    >
                                                        <EditIcon fontSize="small" />
                                                    </IconButton>
                                                </CardContent>
                                            </Card>
                                        )}

                                    {pagination.models.map((model) => {
                                        const isModelSelected = isProviderSelected && selectedModel === model;
                                        return (
                                            <Card
                                                key={model}
                                                sx={{
                                                    width: '100%',
                                                    height: 60,
                                                    border: 1,
                                                    borderColor: isModelSelected ? 'primary.main' : 'grey.300',
                                                    borderRadius: 1.5,
                                                    backgroundColor: isModelSelected ? 'primary.50' : 'background.paper',
                                                    cursor: 'pointer',
                                                    transition: 'all 0.2s ease-in-out',
                                                    position: 'relative',
                                                    boxShadow: isModelSelected ? 2 : 0,
                                                    '&:hover': {
                                                        backgroundColor: isModelSelected ? 'primary.100' : 'grey.50',
                                                        boxShadow: 2,
                                                    },
                                                }}
                                                onClick={() => handleModelSelect(provider, model)}
                                            >
                                                <CardContent sx={{
                                                    textAlign: 'center',
                                                    py: 1,
                                                    px: 0.8,
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    height: '100%'
                                                }}>
                                                    <Typography
                                                        variant="body2"
                                                        sx={{
                                                            fontWeight: 500,
                                                            fontSize: '0.75rem',
                                                            lineHeight: 1.3,
                                                            wordBreak: 'break-word',
                                                            textAlign: 'center'
                                                        }}
                                                    >
                                                        {model}
                                                    </Typography>
                                                    {isModelSelected && (
                                                        <CheckCircle
                                                            color="primary"
                                                            sx={{
                                                                position: 'absolute',
                                                                top: 4,
                                                                right: 4,
                                                                fontSize: 16
                                                            }}
                                                        />
                                                    )}
                                                </CardContent>
                                            </Card>
                                        );
                                    })}
                                </Box>
                                {pagination.totalModels === 0 &&
                                    (!customModels[provider.name] || customModels[provider.name].length === 0) &&
                                    !providerModels?.[provider.name]?.custom_model &&
                                    !(isProviderSelected && selectedModel && isCustomModel(selectedModel, provider)) && (
                                        <Box sx={{ textAlign: 'center', py: 4 }}>
                                            <Typography variant="body2" color="text.secondary">
                                                No models found matching "{searchTerms[provider.name] || ''}"
                                            </Typography>
                                        </Box>
                                    )}
                            </Box>
                        </Stack>
                    </TabPanel>
                );
            })}

            {/* Custom Model Dialog */}
            <Dialog
                open={customModelDialog.open}
                onClose={handleCustomModelCancel}
                maxWidth="sm"
                fullWidth
            >
                <DialogTitle>
                    {customModelDialog.value ? 'Edit Custom Model' : 'Add Custom Model'}
                </DialogTitle>
                <DialogContent>
                    <TextField
                        autoFocus
                        margin="dense"
                        label="Model Name"
                        fullWidth
                        variant="outlined"
                        value={customModelDialog.value}
                        onChange={(e) => setCustomModelDialog(prev => ({ ...prev, value: e.target.value }))}
                        placeholder="Enter custom model name..."
                        sx={{
                            mt: 1,
                            '& .MuiOutlinedInput-root': {
                                borderRadius: 1.5,
                            }
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={handleCustomModelCancel}>
                        Cancel
                    </Button>
                    <Button
                        onClick={handleCustomModelSave}
                        variant="contained"
                        disabled={!customModelDialog.value?.trim()}
                    >
                        {customModelDialog.value ? 'Update' : 'Add'}
                    </Button>
                </DialogActions>
            </Dialog>
        </Box>
    );
}