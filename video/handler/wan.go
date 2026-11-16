package handler

import (
	"net/http"

	"github.com/QuantumNous/new-api/video/dto"
	"github.com/QuantumNous/new-api/video/service"
	"github.com/gin-gonic/gin"
)

type Wan struct {
	wanService *service.Wan
}

func NewWan() *Wan {
	return &Wan{wanService: &service.Wan{}}
}

func (w *Wan) CreateTask(ctx *gin.Context) {
	var req dto.WanCreateTaskRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := w.wanService.CreateTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}
func (w *Wan) GetTask(ctx *gin.Context) {
	var req dto.WanGetTaskRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := w.wanService.GetTask(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}
