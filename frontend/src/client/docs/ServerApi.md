# ServerApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1ServerRestartPost**](#apiv1serverrestartpost) | **POST** /api/v1/server/restart | Restart the server|
|[**apiV1ServerStartPost**](#apiv1serverstartpost) | **POST** /api/v1/server/start | Start the server|
|[**apiV1ServerStopPost**](#apiv1serverstoppost) | **POST** /api/v1/server/stop | Stop the server gracefully|
|[**apiV1StatusGet**](#apiv1statusget) | **GET** /api/v1/status | Get server status and statistics|

# **apiV1ServerRestartPost**
> ServerActionResponse apiV1ServerRestartPost()

Restart the server

### Example

```typescript
import {
    ServerApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ServerApi(configuration);

const { status, data } = await apiInstance.apiV1ServerRestartPost();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**ServerActionResponse**

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

# **apiV1ServerStartPost**
> ServerActionResponse apiV1ServerStartPost()

Start the server

### Example

```typescript
import {
    ServerApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ServerApi(configuration);

const { status, data } = await apiInstance.apiV1ServerStartPost();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**ServerActionResponse**

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

# **apiV1ServerStopPost**
> ServerActionResponse apiV1ServerStopPost()

Stop the server gracefully

### Example

```typescript
import {
    ServerApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ServerApi(configuration);

const { status, data } = await apiInstance.apiV1ServerStopPost();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**ServerActionResponse**

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

# **apiV1StatusGet**
> StatusResponse apiV1StatusGet()

Get server status and statistics

### Example

```typescript
import {
    ServerApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new ServerApi(configuration);

const { status, data } = await apiInstance.apiV1StatusGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**StatusResponse**

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

