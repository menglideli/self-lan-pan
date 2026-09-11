package handler

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cloudpan/internal/dto"
	"cloudpan/internal/model"
)

// BrowseDirs 列出本机目录，给「挂载文件夹」的目录选择器当数据源。
//
// 路由挂在 /api/admin 下（middleware.AdminOnly），非管理员不可访问。
// 单用户内网私有部署：按用户明确要求**不设根目录白名单**，整机可翻。
//   - 不传 path：返回根列表（Windows = 各就绪盘符；其他系统 = /）
//   - 传 path：必须是已存在的绝对路径目录，返回其下子目录（不返回文件——挂载只针对目录）
func (h *AdminHandler) BrowseDirs(c *gin.Context) {
	p := strings.TrimSpace(c.Query("path"))
	roots := localRoots()

	if p != "" {
		// 只接受绝对路径：拒绝相对路径，避免把服务进程的当前工作目录静默当作浏览根
		if !filepath.IsAbs(p) && !(runtime.GOOS == "windows" && filepath.IsAbs(filepath.FromSlash(p))) {
			dto.Fail(c, 400, "必须是绝对路径")
			return
		}
		st, err := os.Stat(p)
		if err != nil {
			dto.Fail(c, 400, "目录不存在或无法访问："+err.Error())
			return
		}
		if !st.IsDir() {
			dto.Fail(c, 400, "所选路径不是文件夹")
			return
		}
	}

	type dirEnt struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	dirs := make([]dirEnt, 0, 32)
	if p != "" {
		ents, err := os.ReadDir(p)
		if err != nil {
			dto.Fail(c, 400, "读取目录失败："+err.Error())
			return
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue // 符号链接（含指向目录的）在此为 false，一并跳过：挂载根不应是链接
			}
			name := e.Name()
			// 跳过 Windows 系统占用目录：对用户没有挂载意义，且读取常被拒绝
			if name == "$RECYCLE.BIN" || name == "System Volume Information" {
				continue
			}
			dirs = append(dirs, dirEnt{Name: name, Path: filepath.Join(p, name)})
		}
		sort.Slice(dirs, func(i, j int) bool {
			return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
		})
	}

	// 上级目录：已在顶层（如 C:\ 的 Dir 仍是 C:\）时不再提供「上一级」
	parent := ""
	if p != "" {
		if par := filepath.Dir(p); par != p {
			parent = par
		}
	}

	dto.OK(c, gin.H{
		"path":   p,
		"parent": parent,
		"dirs":   dirs,
		"roots":  roots,
	})
}

// localRoots 列出可浏览的顶层位置：Windows 为各就绪盘符（C:\ D:\ …），其他系统为 /
func localRoots() []string {
	if runtime.GOOS != "windows" {
		return []string{"/"}
	}
	out := make([]string, 0, 8)
	for ch := 'A'; ch <= 'Z'; ch++ {
		root := string(ch) + ":\\"
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			out = append(out, root)
		}
	}
	return out
}

// genPolicyLetter 为挂载自动生成唯一的「盘符」标识。
//
// 单用户私有部署下前端不再让用户填盘符，但该字段仍有两处硬用途：
// ① WebDAV 的路径段（/dav/<letter>/，见 webdav.go resolve）；
// ② 列表排序键，且带唯一索引（见 model.Policy.Letter）。
// 故必须自动生成且保证唯一：优先取名称里的 ASCII 字母数字（大写，最多 4 位），
// 纯中文名等取不到字符时退化为 M1、M2…（M = Mount）。
func genPolicyLetter(name string) string {
	base := ""
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			base += string(r)
			if len(base) >= 4 {
				break
			}
		}
	}
	if base == "" {
		base = "M"
	}
	cand := base
	for i := 1; ; i++ {
		var n int64
		model.DB.Model(&model.Policy{}).Where("letter = ?", cand).Count(&n)
		if n == 0 {
			return cand
		}
		suf := strconv.Itoa(i)
		trim := 4 - len(suf)
		if trim < 1 {
			trim = 1
		}
		b := base
		if len(b) > trim {
			b = b[:trim]
		}
		cand = b + suf
	}
}
