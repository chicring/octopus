package model

import (
	"context"
	"net/http"
)

type Inbound interface {
	// 入站请求转为内部通用格式
	TransformRequest(ctx context.Context, body []byte) (*InternalLLMRequest, error)

	// 将出站内部通用响应转为入站对应的响应格式
	TransformResponse(ctx context.Context, response *InternalLLMResponse) ([]byte, error)

	// 将出站内部通用流式响应转为入站对应的流式响应格式
	TransformStream(ctx context.Context, stream *InternalLLMResponse) ([]byte, error)

	// 获取完整的内部响应，用于日志记录、数据统计等
	// 流式场景：将储存的流式响应聚合为完整的响应
	// 非流式场景：返回储存的完整响应
	GetInternalResponse(ctx context.Context) (*InternalLLMResponse, error)

	// Reset 清除内部累积的响应状态（streamChunks/storedResponse）
	// 在每次重试尝试前调用，避免前次尝试的残留数据污染后续的响应聚合
	Reset()
}

type Outbound interface {
	// 将入站内部通用请求转为出站对应的请求格式
	TransformRequest(ctx context.Context, request *InternalLLMRequest, baseUrl, key string) (*http.Request, error)

	// 将出站响应转为内部通用响应格式
	TransformResponse(ctx context.Context, response *http.Response) (*InternalLLMResponse, error)

	// 将出站流式转为内部通用流式响应格式
	TransformStream(ctx context.Context, eventData []byte) (*InternalLLMResponse, error)
}

/*
请求流程
非流式

client		-> inbound.TransformRequest(ctx, body)
			-> outbound.TransformRequest(ctx, request)
 			-> http.Do(request)
 			-> outbound.TransformResponse(ctx, response)
			-> inbound.TransformResponse(ctx, response)
															-> client

流式
client		-> inbound.TransformRequest(ctx, body)
        	-> outbound.TransformStream(ctx, chunk)
        	-> http.Do(request)
        	-> outbound.TransformStream(ctx, chunk)
        	-> inbound.TransformStream(ctx, chunk)
															-> client
*/
