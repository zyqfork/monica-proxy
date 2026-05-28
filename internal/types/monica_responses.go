package types

import (
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"monica-proxy/internal/config"
)

const monicaTaskTypeCustomBot = "chat_with_custom_bot"

// MonicaBuildResult 构建 Monica 请求及本轮 ID 信息。
type MonicaBuildResult struct {
	Request          *MonicaRequest
	ConversationID   string
	QuestionItemID   string
	PreReplyItemID   string
	IsFirstTurn      bool
	Instructions     string
}

// ResponsesToMonica 将 Responses API 请求转为 Monica 请求。
// 首轮：welcome + user question；续轮：上轮 reply + 新 question（复用 conversation_id）。
func ResponsesToMonica(req CreateResponseRequest, prev *MonicaSession) (*MonicaBuildResult, error) {
	if !IsModelSupported(req.Model) {
		return nil, fmt.Errorf("model not supported: %s", req.Model)
	}

	parsed, err := ParseResponseInput(req.Input)
	if err != nil {
		return nil, err
	}
	userText := parsed.UserText
	if userText == "" {
		return nil, fmt.Errorf("empty user input")
	}

	modelInfo := modelMap[req.Model]
	instructions := req.Instructions
	if parsed.Instructions != "" {
		if instructions != "" {
			instructions = instructions + "\n" + parsed.Instructions
		} else {
			instructions = parsed.Instructions
		}
	}
	if prev != nil && instructions == "" {
		instructions = prev.Instructions
	}
	if instructions != "" {
		userText = instructions + "\n" + userText
	}

	questionID := fmt.Sprintf("msg:%s", uuid.New().String())
	preReplyID := fmt.Sprintf("msg:%s", uuid.New().String())

	if prev == nil {
		return buildFirstMonicaTurn(req.Model, modelInfo, userText, questionID, preReplyID, instructions)
	}
	return buildContinueMonicaTurn(req.Model, modelInfo, prev, userText, questionID, preReplyID, instructions)
}

func buildFirstMonicaTurn(model string, modelInfo OpenAIModel, userText, questionID, preReplyID, instructions string) (*MonicaBuildResult, error) {
	conversationID := fmt.Sprintf("conv:%s", uuid.New().String())

	welcomeID := fmt.Sprintf("msg:%s", uuid.New().String())
	welcomeItem := Item{
		ItemID:         welcomeID,
		ConversationID: conversationID,
		ItemType:       "reply",
		Summary:        "__RENDER_BOT_WELCOME_MSG__",
		Data:           ItemContent{Type: "text", Content: "__RENDER_BOT_WELCOME_MSG__"},
	}

	questionItem := Item{
		ConversationID: conversationID,
		ItemID:         questionID,
		ItemType:       "question",
		Summary:        truncateSummary(userText),
		ParentItemID:   welcomeID,
		Data: ItemContent{
			Type:        "text",
			Content:     userText,
			QuoteContent: "",
			ChatModel:   chatModelForAPI(model),
			MaxToken:    0,
			IsIncognito: config.MonicaConfig.IsIncognito,
		},
	}

	mReq := &MonicaRequest{
		TaskUID: fmt.Sprintf("task:%s", uuid.New().String()),
		BotUID:  modelInfo.BotUid,
		Data: DataField{
			ConversationID:      conversationID,
			Items:               []Item{welcomeItem, questionItem},
			PreGeneratedReplyId: preReplyID,
			PreParentItemID:     questionID,
			Origin:              modelInfo.Origin,
			OriginPageTitle:     modelInfo.OriginPageTitle,
			TriggerBy:           "auto",
			UseModel:            model,
			KnowledgeSource:     "academic",
			IsIncognito:         config.MonicaConfig.IsIncognito,
			UseNewMemory:        true,
			UseMemorySuggestion: true,
		},
		Language:       "auto",
		Locale:         "zh_CN",
		TaskType:       monicaTaskTypeCustomBot,
		ToolData:       defaultMonicaToolData(),
		AIRespLanguage: "Chinese (Simplified)",
	}

	return &MonicaBuildResult{
		Request:        mReq,
		ConversationID: conversationID,
		QuestionItemID: questionID,
		PreReplyItemID: preReplyID,
		IsFirstTurn:    true,
		Instructions:   instructions,
	}, nil
}

func buildContinueMonicaTurn(model string, modelInfo OpenAIModel, prev *MonicaSession, userText, questionID, preReplyID, instructions string) (*MonicaBuildResult, error) {
	conversationID := prev.ConversationID
	lastReply := prev.LastReplyItem
	lastReply.ConversationID = conversationID

	questionItem := Item{
		ConversationID: conversationID,
		ItemID:         questionID,
		ItemType:       "question",
		Summary:        truncateSummary(userText),
		ParentItemID:   lastReply.ItemID,
		Data: ItemContent{
			Type:        "text",
			Content:     userText,
			QuoteContent: "",
			ChatModel:   chatModelForAPI(model),
			MaxToken:    0,
			IsIncognito: config.MonicaConfig.IsIncognito,
		},
	}

	origin := modelInfo.Origin + "?convId=" + url.QueryEscape(conversationID)

	mReq := &MonicaRequest{
		TaskUID: fmt.Sprintf("task:%s", uuid.New().String()),
		BotUID:  prev.BotUID,
		Data: DataField{
			ConversationID:      conversationID,
			Items:               []Item{lastReply, questionItem},
			PreGeneratedReplyId: preReplyID,
			PreParentItemID:     questionID,
			Origin:              origin,
			OriginPageTitle:     modelInfo.OriginPageTitle,
			TriggerBy:           "auto",
			UseModel:            model,
			KnowledgeSource:     "academic",
			IsIncognito:         config.MonicaConfig.IsIncognito,
			UseNewMemory:        true,
			UseMemorySuggestion: true,
		},
		Language:       "auto",
		Locale:         "zh_CN",
		TaskType:       monicaTaskTypeCustomBot,
		ToolData:       defaultMonicaToolData(),
		AIRespLanguage: "Chinese (Simplified)",
	}

	return &MonicaBuildResult{
		Request:        mReq,
		ConversationID: conversationID,
		QuestionItemID: questionID,
		PreReplyItemID: preReplyID,
		IsFirstTurn:    false,
		Instructions:   instructions,
	}, nil
}

// BuildReplyItemFromAssistant 根据助手回复构建 Monica reply item，供下一轮续聊。
func BuildReplyItemFromAssistant(
	conversationID, replyItemID, questionItemID, model, content string,
	meta MonicaReplyMeta,
) Item {
	agentStatus := meta.AgentStatus
	if agentStatus == nil {
		agentStatus = []AgentStatusEntry{}
	}
	artifacts := meta.Artifacts
	if artifacts == nil {
		artifacts = []any{}
	}
	followSuggestions := meta.FollowSuggestions
	if followSuggestions == nil {
		followSuggestions = []FollowSuggestion{}
	}
	return Item{
		ConversationID: conversationID,
		ItemID:         replyItemID,
		ItemType:       "reply",
		Summary:        truncateSummary(content),
		ParentItemID:   questionItemID,
		Data: ItemContent{
			Type:                   "text",
			Content:                content,
			QuestionID:             questionItemID,
			FromTaskType:           monicaTaskTypeCustomBot,
			ManualWebSearchEnabled: false,
			UseModel:               model,
			AgentStatus:            agentStatus,
			Artifacts:              artifacts,
			FollowSuggestions:      followSuggestions,
		},
	}
}

// MonicaReplyMeta 从 Monica SSE 收集的 reply 元数据。
type MonicaReplyMeta struct {
	AgentStatus       []AgentStatusEntry
	Artifacts         []any
	FollowSuggestions []FollowSuggestion
}

func truncateSummary(s string) string {
	runes := []rune(s)
	if len(runes) <= 200 {
		return s
	}
	return string(runes[:200])
}
