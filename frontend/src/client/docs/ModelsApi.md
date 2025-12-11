# ModelsApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1ProviderModelsGet**](#apiv1providermodelsget) | **GET** /api/v1/provider-models | Get all provider models|
|[**apiV1ProviderModelsNamePost**](#apiv1providermodelsnamepost) | **POST** /api/v1/provider-models/:name | Fetch models for a specific provider|

# **apiV1ProviderModelsGet**
> ProviderModelsResponse apiV1ProviderModelsGet()

Get all provider models

### Example

```typescript
import {
    ModelsApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ModelsApi(configuration);

const { status, data } = await apiInstance.apiV1ProviderModelsGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**ProviderModelsResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **apiV1ProviderModelsNamePost**
> FetchProviderModelsResponse apiV1ProviderModelsNamePost()

Fetch models for a specific provider

### Example

```typescript
import {
    ModelsApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ModelsApi(configuration);

const { status, data } = await apiInstance.apiV1ProviderModelsNamePost();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**FetchProviderModelsResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

