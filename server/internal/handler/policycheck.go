package handler

import (
	"fmt"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"

	"cloudpan/internal/dto"
	"cloudpan/internal/fscore"
	"cloudpan/internal/model"
)

// ---- 存储策略连通性探测（管理台「测通」）----
//
// 历史：本文件原为 cloudauth.go，承载云盘 OAuth 授权（授权页 → 回调 → 授权码换 token）。
// 单用户私有部署只保留「挂载本机磁盘目录」后，云盘挂载与其授权流程整体移除，
// 这里只剩「测通」这一路。

func parseUintQuery(c *gin.Context, key string) uint {
	v, _ := strconv.ParseUint(c.Query(key), 10, 32)
	return uint(v)
}

// PolicyCheck 存储策略连通性探测
type PolicyCheck struct{}

// Status 探测策略是否可用（管理台「测通」按钮）。
//
// 本地挂载的"连通"= 根目录真的读得出来。早先本地策略直接返回 ok，
// 于是根目录被拔盘 / 改名 / 权限变更之后管理台依然显示"连通正常"——测了个寂寞。
func (h *PolicyCheck) Status(c *gin.Context) {
	policyID := parseUintQuery(c, "policyId")
	var p model.Policy
	if err := model.DB.First(&p, policyID).Error; err != nil {
		dto.Fail(c, 404, "存储策略不存在")
		return
	}
	// 先 Stat 再 NewLocal：fscore.NewLocal 内部会 MkdirAll，
	// 直接调它会把"已经不存在的挂载目录"悄悄重建出来，然后报告"连通正常"。
	st, err := os.Stat(p.RootPath)
	if err != nil {
		dto.OK(c, gin.H{"ok": false, "msg": "挂载目录不存在或不可访问：" + err.Error()})
		return
	}
	if !st.IsDir() {
		dto.OK(c, gin.H{"ok": false, "msg": "挂载路径不是文件夹"})
		return
	}
	d, err := fscore.NewLocal(p.RootPath)
	if err != nil {
		dto.OK(c, gin.H{"ok": false, "msg": err.Error()})
		return
	}
	ents, err := d.List("/")
	if err != nil {
		dto.OK(c, gin.H{"ok": false, "msg": "目录不可读：" + err.Error()})
		return
	}
	dto.OK(c, gin.H{"ok": true, "msg": fmt.Sprintf("目录可读（%d 项）", len(ents))})
}
