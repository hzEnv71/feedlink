package handlers

import (
	"feed/mq"
	"feed/utils"

	"github.com/gin-gonic/gin"
)

// OpsHandler 提供运维观测接口（只读）。
type OpsHandler struct{}

func NewOpsHandler() *OpsHandler { return &OpsHandler{} }

// MQMetrics 查看 MQ 熔断/降级指标快照。
// GET /api/ops/mq/metrics
func (h *OpsHandler) MQMetrics(c *gin.Context) {
	utils.Success(c, mq.SnapshotMetrics())
}
