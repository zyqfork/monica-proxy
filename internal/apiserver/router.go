package apiserver

import (
	"encoding/json"
	"log"
	"monica-proxy/internal/middleware"
	"monica-proxy/internal/monica"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sashabaranov/go-openai"
)

// RegisterRoutes 注册 Echo 路由
func RegisterRoutes(e *echo.Echo) {
	// 添加Bearer Token认证中间件
	e.Use(middleware.BearerAuth())

	// ChatGPT 风格的请求转发到 /v1/chat/completions
	e.POST("/v1/chat/completions", handleChatCompletion)
	// 获取支持的模型列表
	e.GET("/v1/models", handleListModels)
}

func handleChatCompletion(c echo.Context) error {
	var req openai.ChatCompletionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request payload",
		})
	}

	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "No messages found",
		})
	}

	monicaReq, err := types.ChatGPTToMonica(req)
	// 将monicaReq转换为JSON格式并打印
	jsonBytes, err := json.MarshalIndent(monicaReq, "", "    ")
	if err != nil {
		log.Printf("JSON marshal error: %v", err)
	} else {
		log.Printf("monicaReq: \n%s", string(jsonBytes))
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
	}

	stream, err := monica.SendMonicaRequest(c.Request().Context(), monicaReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
	}
	defer stream.RawBody().Close()

	// 根据请求的 stream 参数决定使用哪种处理方式
	fingerprint := utils.RandStringUsingMathRand(10)
	if req.Stream {
		// 流式处理
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Transfer-Encoding", "chunked")
		c.Response().WriteHeader(http.StatusOK)

		return monica.StreamMonicaSSEToClient(c.Request().Context(), req, c.Response().Writer, stream.RawBody(), fingerprint)
	} else {
		// 非流式处理
		response, err := monica.ProcessMonicaResponse(c.Request().Context(), req, stream.RawBody(), fingerprint)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
		}
		return c.JSON(http.StatusOK, response)
	}
}

// handleListModels 返回支持的模型列表
func handleListModels(c echo.Context) error {
	models := types.GetSupportedModels()
	return c.JSON(http.StatusOK, models)
}
