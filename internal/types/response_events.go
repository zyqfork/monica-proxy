package types

import "github.com/openai/openai-go/v3/shared/constant"

// Responses API 流式事件名（与官方 openai-go SDK 对齐）。
const (
	ResponseEventCreated            = constant.ResponseCreated("response.created")
	ResponseEventInProgress         = constant.ResponseInProgress("response.in_progress")
	ResponseEventOutputItemAdded    = constant.ResponseOutputItemAdded("response.output_item.added")
	ResponseEventContentPartAdded   = constant.ResponseContentPartAdded("response.content_part.added")
	ResponseEventOutputTextDelta    = constant.ResponseOutputTextDelta("response.output_text.delta")
	ResponseEventOutputTextDone     = constant.ResponseOutputTextDone("response.output_text.done")
	ResponseEventContentPartDone     = constant.ResponseContentPartDone("response.content_part.done")
	ResponseEventOutputItemDone     = constant.ResponseOutputItemDone("response.output_item.done")
	ResponseEventCompleted          = constant.ResponseCompleted("response.completed")
)
