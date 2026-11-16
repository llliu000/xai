package handler

import (
	"net/http"

	"github.com/QuantumNous/new-api/video/dto"
	"github.com/QuantumNous/new-api/video/service"
	"github.com/gin-gonic/gin"
)

type Ark struct {
	arkService *service.Ark
}

func NewArk() *Ark {
	return &Ark{arkService: &service.Ark{}}
}

func (s *Ark) GetTask(ctx *gin.Context) {
	var req dto.ArkGetTaskRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := s.arkService.GetTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (s *Ark) CreateTask(ctx *gin.Context) {
	var req dto.ArkCreateTaskRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := s.arkService.CreateTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}
