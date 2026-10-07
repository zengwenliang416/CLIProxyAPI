package openai

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIDecisionsAlt = "decisions"

func writeDecisionsInvalidRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
		Error: handlers.ErrorDetail{
			Message: message,
			Type:    "invalid_request_error",
		},
	})
}

// Decisions handles POST /v1/decisions.
// The body is forwarded as a Decisions request. Question types are validated
// by the upstream API.
func (h *OpenAIResponsesAPIHandler) Decisions(c *gin.Context) {
	rawJSON, err := h.ReadRequestBody(c)
	if err != nil {
		handlers.WriteRequestBodyError(c, err)
		return
	}
	defer handlers.ReleaseRequestBody(c)

	if !gjson.ValidBytes(rawJSON) || !gjson.ParseBytes(rawJSON).IsObject() {
		writeDecisionsInvalidRequest(c, "Invalid JSON body")
		return
	}
	if gjson.GetBytes(rawJSON, "stream").Type == gjson.True {
		writeDecisionsInvalidRequest(c, "Streaming is not supported for the Decisions API")
		return
	}
	model := gjson.GetBytes(rawJSON, "model")
	if model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		writeDecisionsInvalidRequest(c, "Missing required parameter: 'model'")
		return
	}
	input := gjson.GetBytes(rawJSON, "input")
	if !input.Exists() || input.Type == gjson.Null {
		writeDecisionsInvalidRequest(c, "Missing required parameter: 'input'")
		return
	}
	questions := gjson.GetBytes(rawJSON, "questions")
	if !questions.IsArray() || len(questions.Array()) == 0 {
		writeDecisionsInvalidRequest(c, "Missing required parameter: 'questions'")
		return
	}
	if gjson.GetBytes(rawJSON, "stream").Exists() {
		if updated, errDelete := sjson.DeleteBytes(rawJSON, "stream"); errDelete == nil {
			rawJSON = updated
		}
	}

	c.Header("Content-Type", "application/json")
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	stopKeepAlive := h.StartNonStreamingKeepAlive(c, cliCtx)
	resp, upstreamHeaders, errMsg := h.ExecuteWithAuthManager(cliCtx, h.HandlerType(), model.String(), rawJSON, openAIDecisionsAlt)
	stopKeepAlive()
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		cliCancel(errMsg.Error)
		return
	}
	handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
	_, _ = c.Writer.Write(resp)
	cliCancel()
}
