package response

import (
	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func Success(c *gin.Context, httpStatus int, data interface{}) {
	c.JSON(httpStatus, Envelope{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

func Error(c *gin.Context, httpStatus int, bizCode int, message string) {
	c.JSON(httpStatus, Envelope{
		Code:    bizCode,
		Message: message,
		Data:    nil,
	})
}
