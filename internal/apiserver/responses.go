package apiserver

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"monica-proxy/internal/monica"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
)

func handleCreateResponse(c echo.Context) error {
	var req types.CreateResponseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{"message": "invalid request payload"},
		})
	}

	if !types.IsModelSupported(req.Model) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{"message": "model not supported"},
		})
	}

	stateful := types.IsStatefulRequest(req)

	var prev *types.MonicaSession
	if stateful && req.PreviousResponseID != "" {
		session, ok := types.DefaultSessionStore.Get(req.PreviousResponseID)
		if !ok {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error": map[string]string{
					"message": "previous_response_id not found or expired: " + req.PreviousResponseID,
				},
			})
		}
		prev = session
	}

	build, err := types.ResponsesToMonica(req, prev)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": map[string]string{"message": err.Error()},
		})
	}

	respID := monica.ResponseID()
	createdAt := monica.ResponseCreatedAt()
	modelInfo := types.GetModelInfo(req.Model)

	// 有状态：立即写入 resp_id ↔ conversation_id 映射，不等待流结束
	if stateful {
		putMonicaSessionInitial(respID, req, build, modelInfo)
	}

	stream, err := monica.SendMonicaRequest(c.Request().Context(), build.Request)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]string{"message": err.Error()},
		})
	}
	defer stream.RawBody().Close()

	if req.Stream {
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		c.Response().WriteHeader(http.StatusOK)

		result, err := monica.StreamMonicaToResponsesSSE(
			c.Request().Context(),
			c.Response().Writer,
			stream.RawBody(),
			respID,
			req.Model,
			createdAt,
		)
		if err != nil {
			return err
		}
		finalizeMonicaSession(respID, req, build, result, stateful)
		return nil
	}

	result, err := monica.CollectMonicaStream(c.Request().Context(), stream.RawBody())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]string{"message": err.Error()},
		})
	}

	inTok := utils.CalculateTokens(build.Request.Data.Items[len(build.Request.Data.Items)-1].Data.Content)
	outTok := utils.CalculateTokens(result.Content)
	usage := &types.ResponseUsage{
		InputTokens:  inTok,
		OutputTokens: outTok,
		TotalTokens:  inTok + outTok,
	}
	resp := types.NewResponseObject(respID, createdAt, req.Model, "completed", result.Content, usage)
	finalizeMonicaSession(respID, req, build, result, stateful)
	return c.JSON(http.StatusOK, resp)
}

// putMonicaSessionInitial 请求开始时写入会话骨架（含 conversation_id），便于上一轮未结束即可续聊。
func putMonicaSessionInitial(
	respID string,
	req types.CreateResponseRequest,
	build *types.MonicaBuildResult,
	modelInfo types.OpenAIModel,
) {
	questionItem := build.Request.Data.Items[len(build.Request.Data.Items)-1]
	replyPlaceholder := types.BuildReplyItemFromAssistant(
		build.ConversationID,
		build.PreReplyItemID,
		build.QuestionItemID,
		req.Model,
		"",
		types.MonicaReplyMeta{},
	)

	session := &types.MonicaSession{
		ConversationID:   build.ConversationID,
		Model:            req.Model,
		BotUID:           modelInfo.BotUid,
		Origin:           modelInfo.Origin,
		OriginPageTitle:  modelInfo.OriginPageTitle,
		Instructions:     resolveSessionInstructions(req, build),
		LastQuestionItem: questionItem,
		LastReplyItem:    replyPlaceholder,
		CreatedAt:        time.Now(),
	}
	types.DefaultSessionStore.Put(respID, session)
}

// finalizeMonicaSession 流式/非流式结束后补全助手回复内容。
func finalizeMonicaSession(
	respID string,
	req types.CreateResponseRequest,
	build *types.MonicaBuildResult,
	result *monica.MonicaStreamResult,
	stateful bool,
) {
	if !stateful || result == nil {
		return
	}

	replyItem := types.BuildReplyItemFromAssistant(
		build.ConversationID,
		build.PreReplyItemID,
		build.QuestionItemID,
		req.Model,
		result.Content,
		result.Meta,
	)

	session, ok := types.DefaultSessionStore.Get(respID)
	if !ok {
		modelInfo := types.GetModelInfo(req.Model)
		putMonicaSessionInitial(respID, req, build, modelInfo)
		session, _ = types.DefaultSessionStore.Get(respID)
	}
	if session == nil {
		return
	}

	session.LastReplyItem = replyItem
	types.DefaultSessionStore.Put(respID, session)
}

func resolveSessionInstructions(req types.CreateResponseRequest, build *types.MonicaBuildResult) string {
	instructions := build.Instructions
	if instructions == "" {
		instructions = req.Instructions
	}
	if instructions == "" && req.PreviousResponseID != "" {
		if prev, ok := types.DefaultSessionStore.Get(req.PreviousResponseID); ok {
			instructions = prev.Instructions
		}
	}
	return instructions
}
