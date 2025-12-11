# ProvidersApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1ProvidersGet**](#apiv1providersget) | **GET** /api/v1/providers | Get all configured providers with masked tokens|
|[**apiV1ProvidersNameDelete**](#apiv1providersnamedelete) | **DELETE** /api/v1/providers/:name | Delete a provider configuration|
|[**apiV1ProvidersNameGet**](#apiv1providersnameget) | **GET** /api/v1/providers/:name | Get specific provider details with masked token|
|[**apiV1ProvidersNamePut**](#apiv1providersnameput) | **PUT** /api/v1/providers/:name | Update existing provider configuration|
|[**apiV1ProvidersNameTogglePost**](#apiv1providersnametogglepost) | **POST** /api/v1/providers/:name/toggle | Toggle provider enabled/disabled status|
|[**apiV1ProvidersPost**](#apiv1providerspost) | **POST** /api/v1/providers | Add a new provider configuration|

# **apiV1ProvidersGet**
> ProvidersResponse apiV1ProvidersGet()

Get all configured providers with masked tokens

### Example

```typescript
import {
    ProvidersApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ProvidersApi(configuration);

const { status, data } = await apiInstance.apiV1ProvidersGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**ProvidersResponse**

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

# **apiV1ProvidersNameDelete**
> DeleteProviderResponse apiV1ProvidersNameDelete()

Delete a provider configuration

### Example

```typescript
import {
    ProvidersApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ProvidersApi(configuration);

const { status, data } = await apiInstance.apiV1ProvidersNameDelete();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**DeleteProviderResponse**

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

# **apiV1ProvidersNameGet**
> ProviderResponse apiV1ProvidersNameGet()

Get specific provider details with masked token

### Example

```typescript
import {
    ProvidersApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ProvidersApi(configuration);

const { status, data } = await apiInstance.apiV1ProvidersNameGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**ProviderResponse**

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

# **apiV1ProvidersNamePut**
> UpdateProviderResponse apiV1ProvidersNamePut(request)

Update existing provider configuration

### Example

```typescript
import {
    ProvidersApi,
    Configuration,
    UpdateProviderRequest
} from './api';

const configuration = new Configuration();
const apiInstance = new ProvidersApi(configuration);

let request: UpdateProviderRequest; //Request body

const { status, data } = await apiInstance.apiV1ProvidersNamePut(
    request
);
```

### Parameters

|Name | Type | Description  | Notes|
|------------- | ------------- | ------------- | -------------|
| **request** | **UpdateProviderRequest**| Request body | |


### Return type

**UpdateProviderResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **apiV1ProvidersNameTogglePost**
> ToggleProviderResponse apiV1ProvidersNameTogglePost()

Toggle provider enabled/disabled status

### Example

```typescript
import {
    ProvidersApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ProvidersApi(configuration);

const { status, data } = await apiInstance.apiV1ProvidersNameTogglePost();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**ToggleProviderResponse**

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

# **apiV1ProvidersPost**
> AddProviderResponse apiV1ProvidersPost(request)

Add a new provider configuration

### Example

```typescript
import {
    ProvidersApi,
    Configuration,
    AddProviderRequest
} from './api';

const configuration = new Configuration();
const apiInstance = new ProvidersApi(configuration);

let request: AddProviderRequest; //Request body

const { status, data } = await apiInstance.apiV1ProvidersPost(
    request
);
```

### Parameters

|Name | Type | Description  | Notes|
|------------- | ------------- | ------------- | -------------|
| **request** | **AddProviderRequest**| Request body | |


### Return type

**AddProviderResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

