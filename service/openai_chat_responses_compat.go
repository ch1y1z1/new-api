package service

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/openaicompat"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	return openaicompat.ChatCompletionsRequestToResponsesRequest(req)
}

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	return openaicompat.ResponsesResponseToChatCompletionsResponse(resp, id)
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	return openaicompat.ExtractOutputTextFromResponses(resp)
}

func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, responseId string, instructions json.RawMessage, metadata json.RawMessage) (*dto.OpenAIResponsesResponse, error) {
	return openaicompat.ChatCompletionsResponseToResponsesResponse(resp, responseId, instructions, metadata)
}

func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	return openaicompat.ResponsesRequestToChatCompletionsRequest(req)
}

func ApplyWebSearchInterception(chatReq *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) error {
	return openaicompat.ApplyWebSearchInterception(chatReq, info)
}
