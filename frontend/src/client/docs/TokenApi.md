# TokenApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1TokenGet**](#apiv1tokenget) | **GET** /api/v1/token | Get existing API token or generate new one|
|[**apiV1TokenPost**](#apiv1tokenpost) | **POST** /api/v1/token | Generate a new API token|

# **apiV1TokenGet**
> TokenResponse apiV1TokenGet()

Get existing API token or generate new one

### Example

```typescript
import {
    TokenApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new TokenApi(configuration);

const { status, data } = await apiInstance.apiV1TokenGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**TokenResponse**

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

# **apiV1TokenPost**
> TokenResponse apiV1TokenPost(request)

Generate a new API token

### Example

```typescript
import {
    TokenApi,
    Configuration,
    GenerateTokenRequest
} from './api';

const configuration = new Configuration();
const apiInstance = new TokenApi(configuration);

let request: GenerateTokenRequest; //Request body

const { status, data } = await apiInstance.apiV1TokenPost(
    request
);
```

### Parameters

|Name | Type | Description  | Notes|
|------------- | ------------- | ------------- | -------------|
| **request** | **GenerateTokenRequest**| Request body | |


### Return type

**TokenResponse**

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

