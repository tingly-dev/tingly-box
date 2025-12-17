import { useState } from 'react';

export const usePagination = (providerNames: string[], modelsPerPage: number) => {
    const [searchTerms, setSearchTerms] = useState<{ [key: string]: string }>({});
    const [currentPage, setCurrentPage] = useState<{ [key: string]: number }>({});

    const handleSearchChange = (providerName: string, searchTerm: string) => {
        setSearchTerms(prev => ({ ...prev, [providerName]: searchTerm }));
        // Reset to first page when searching
        setCurrentPage(prev => ({ ...prev, [providerName]: 1 }));
    };

    const handlePageChange = (providerName: string, page: number) => {
        setCurrentPage(prev => ({ ...prev, [providerName]: page }));
    };

    const getPaginatedData = <T,>(items: T[], providerName: string) => {
        const searchTerm = searchTerms[providerName] || '';
        let filteredItems = items;

        if (searchTerm) {
            filteredItems = items.filter(item =>
                typeof item === 'string' && item.toLowerCase().includes(searchTerm.toLowerCase())
            );
        }

        const page = currentPage[providerName] || 1;
        const startIndex = (page - 1) * modelsPerPage;
        const endIndex = startIndex + modelsPerPage;

        return {
            items: filteredItems.slice(startIndex, endIndex),
            totalPages: Math.ceil(filteredItems.length / modelsPerPage),
            currentPage: page,
            totalItems: filteredItems.length,
        };
    };

    return {
        searchTerms,
        currentPage,
        setCurrentPage,
        handleSearchChange,
        handlePageChange,
        getPaginatedData
    };
};