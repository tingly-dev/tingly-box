# ProviderSelect Component

A specialized component for selecting provider and model combinations with an intuitive visual interface.

## Features

- **Visual Provider Selection**: Left side displays provider cards with status indicators
- **Model Selection**: Right side shows up to 3 model slots per provider
- **Interactive Feedback**: Hover effects and selection states
- **Empty Slots**: "Add Model" slots for providers with fewer than 3 models
- **Selection Callback**: Triggers `onSelected` when provider or model is chosen
- **Customizable**: Configurable number of model slots

## Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `providers` | `Provider[]` | **Required** | Array of provider objects |
| `providerModels` | `ProviderModelsData` | - | Models data for each provider |
| `onSelected` | `(option: ProviderSelectOption) => void` | - | Callback when selection changes |
| `selectedProvider` | `string` | - | Currently selected provider name |
| `selectedModel` | `string` | - | Currently selected model name |
| `maxModelSlots` | `number` | `3` | Maximum number of model display slots |

## ProviderSelectOption Interface

```typescript
interface ProviderSelectOption {
    provider: Provider;
    model?: string;  // Optional, only present when model is selected
}
```

## Usage Example

```tsx
import ProviderSelect from '../components/ProviderSelect';
import { api } from '../services/api';
import { useState } from 'react';

const MyComponent = () => {
    const [providers, setProviders] = useState<Provider[]>([]);
    const [providerModels, setProviderModels] = useState<ProviderModelsData>({});
    const [selectedOption, setSelectedOption] = useState<ProviderSelectOption | null>(null);

    const loadData = async () => {
        const providersResult = await api.getProviders();
        const modelsResult = await api.getProviderModels();

        if (providersResult.success) {
            setProviders(providersResult.data);
        }
        if (modelsResult.success) {
            setProviderModels(modelsResult.data);
        }
    };

    const handleSelection = (option: ProviderSelectOption) => {
        setSelectedOption(option);
        console.log('Selected provider:', option.provider.name);
        if (option.model) {
            console.log('Selected model:', option.model);
        }
    };

    return (
        <ProviderSelect
            providers={providers}
            providerModels={providerModels}
            onSelected={handleSelection}
            selectedProvider={selectedOption?.provider.name}
            selectedModel={selectedOption?.model}
            maxModelSlots={3}
        />
    );
};
```

## Visual States

### Provider States
- **Default**: Gray border, normal opacity
- **Hover**: Primary color border
- **Selected**: Primary color border with primary background and check icon
- **Disabled**: Lower opacity, not clickable

### Model States
- **Model Available**: Gray border, shows model name
- **Model Selected**: Primary color border with primary background and check icon
- **Empty Slot**: Dashed border with "Add Model" text and plus icon

## Layout

```
┌─────────────────────────────────────────────────────────────┐
│ [Provider Card]  [Model 1] [Model 2] [Model 3/Add]          │
│      ▲                                                ▲      │
│      │                                                │      │
│  Left Side                                       Right Side   │
└─────────────────────────────────────────────────────────────┘
```

## Integration Points

This component can be used in:
- Model configuration screens
- Provider selection dialogs
- Quick setup wizards
- Dashboard provider management

## Related Components

- `ProviderCard`: Individual provider display
- `ProviderModelsData`: Provider models data structure
- `Provider`: Provider data interface
