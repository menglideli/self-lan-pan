package handler

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloudpan/internal/model"
)

// ---- 离线下载缓存目录的统一治理 ----
//
// 背景（2026-09-13 实测定位）：三种下载链路各自把中间数据放在不同地方，
// 每处都写了 defer Remove/RemoveAll —— 但它只在"执行体正常返回"时生效。
// 一旦进程被强杀 / 断电 / 崩溃，这些 defer 全部不执行，残留会一直堆着。
// 实测证据：手工放进 bt_tmp/99999 与 %TEMP%/cp_offline_88888.tmp 的残留，
// 完整重启服务后依然存在（见 docs/单用户私有化改造计划.md §7.11）。
//
// 三处缓存：
//   1. HTTP 直链  : <os.TempDir()>/cp_offline_<taskID>.tmp          单文件
//   2. m3u8 (HLS) : <data>/ziptmp/m3u8-<随机>/                       目录（分片）
//   3. BT / 磁力  : <data>/bt_tmp/<taskID>/                          目录（数据 + bolt.db）
//
// 治理原则（与"绝不自动删用户数据"不冲突）：
//   - 只清理「本程序自己造的缓存命名规则」匹配到的路径，绝不泛删目录内容；
//   - bt_tmp / ziptmp 下凡是「不属于当前活跃任务」的条目一律清掉；
//   - 启动时清一次（兜住上次异常退出），运行中每 6 小时清一次（兜住运行期异常）；
//   - 删除任务时顺手清该任务的缓存（用户点了删除就该干净）。

// offlineTmpPath HTTP 直链下载的临时文件路径（与 runOffline 共用同一命名规则）
func offlineTmpPath(taskID uint) string {
	return filepath.Join(os.TempDir(), "cp_offline_"+fmt.Sprint(taskID)+".tmp")
}

// removeOfflineTmp 删除直链下载的临时文件（不存在则忽略）
func removeOfflineTmp(taskID uint) {
	_ = os.Remove(offlineTmpPath(taskID))
}

// btTaskDir BT 任务的缓存目录（与 runBT 共用同一命名规则）
func (p *TaskPool) btTaskDir(taskID uint) string {
	return filepath.Join(p.BtDir, fmt.Sprint(taskID))
}

// removeBTCache 删除某个 BT 任务的缓存目录
func (p *TaskPool) removeBTCache(taskID uint) {
	if p.BtDir == "" {
		return
	}
	_ = os.RemoveAll(p.btTaskDir(taskID))
}

// m3u8TmpDirOf m3u8 任务的临时目录（与 runM3U8Task 共用同一命名规则）
func m3u8TmpDirOf(zips string, taskID uint) string {
	return filepath.Join(zips, "m3u8-"+fmt.Sprint(taskID))
}

// removeM3U8Cache 删除某个 m3u8 任务的临时目录
func (p *TaskPool) removeM3U8Cache(taskID uint) {
	if p.Zips == "" {
		return
	}
	_ = os.RemoveAll(m3u8TmpDirOf(p.Zips, taskID))
}

// purgeTaskCache 清理单个任务的全部磁盘缓存（取消 / 删除 / 失败后调用）
func (p *TaskPool) purgeTaskCache(taskID uint) {
	removeOfflineTmp(taskID)
	p.removeBTCache(taskID)
	p.removeM3U8Cache(taskID)
}

// activeTaskIDs 当前"尚未结束"的任务 ID 集合（queued / processing）。
// 清扫时要放过这些 ID 对应的缓存 —— 它们正在被写。
func activeTaskIDs() map[uint]bool {
	out := map[uint]bool{}
	var ids []uint
	model.DB.Model(&model.Task{}).Where("status IN ?", []string{"queued", "processing"}).Pluck("id", &ids)
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// sweepTaskCaches 清理离线下载的磁盘缓存。
//
// 三类残留都会被清掉：
//   - bt_tmp/ 下不属于活跃任务的目录（含历史崩溃残留）
//   - ziptmp/ 下不属于活跃任务的 m3u8-* 目录
//   - os.TempDir()/ 下不属于活跃任务的 cp_offline_*.tmp
//
// 注意只删匹配到本程序命名规则的条目：bt_tmp 下非纯数字命名的目录不动，
// ziptmp 下非 "m3u8-" 前缀的条目不动（那里还有 mk_ 临时压缩包，由各自 defer 管）。
func (p *TaskPool) sweepTaskCaches() {
	active := activeTaskIDs()
	now := time.Now()
	// freshEnough 判断"这个缓存是不是刚被写过"。
	// 用目录树里最新的 mtime，而不是目录自身的 mtime —— 目录 mtime 在子目录被
	// 创建/删除时才更新，往里写文件往往不改它，拿它做"是否活跃"的判据会误删。
	freshEnough := func(path string) bool {
		return now.Sub(newestMTime(path)) < time.Minute
	}

	// 1) BT 缓存
	if p.BtDir != "" {
		if entries, err := os.ReadDir(p.BtDir); err == nil {
			for _, e := range entries {
				name := e.Name()
				// 只处理纯数字目录名（= 任务 ID 的十进制形式）
				id, ok := parseTaskIDName(name)
				if !ok || active[id] {
					continue
				}
				path := filepath.Join(p.BtDir, name)
				if freshEnough(path) {
					continue
				}
				_ = os.RemoveAll(path)
				log.Printf("[CloudPan] 清理 BT 缓存残留: %s", path)
			}
		}
	}

	// 2) m3u8 缓存（ziptmp/m3u8-<taskID>）
	if p.Zips != "" {
		if entries, err := os.ReadDir(p.Zips); err == nil {
			for _, e := range entries {
				if !strings.HasPrefix(e.Name(), "m3u8-") {
					continue
				}
				id, ok := parseTaskIDName(strings.TrimPrefix(e.Name(), "m3u8-"))
				if !ok || active[id] {
					continue
				}
				path := filepath.Join(p.Zips, e.Name())
				if freshEnough(path) {
					continue
				}
				_ = os.RemoveAll(path)
				log.Printf("[CloudPan] 清理 m3u8 缓存残留: %s", path)
			}
		}
	}

	// 3) HTTP 直链缓存（os.TempDir()/cp_offline_*.tmp）
	if entries, err := os.ReadDir(os.TempDir()); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "cp_offline_") || !strings.HasSuffix(name, ".tmp") {
				continue
			}
			id, ok := parseTaskIDName(strings.TrimSuffix(strings.TrimPrefix(name, "cp_offline_"), ".tmp"))
			if !ok || active[id] {
				continue
			}
			path := filepath.Join(os.TempDir(), name)
			if freshEnough(path) {
				continue
			}
			_ = os.Remove(path)
			log.Printf("[CloudPan] 清理离线下载临时文件残留: %s", path)
		}
	}
}

// parseTaskIDName 把"纯十进制数字"的名字解析成任务 ID。
// 刻意要求 fmt.Sprint(id) == name：这样 "007"、" 12"、"-3" 这类都会被拒掉，
// 避免误伤名字里恰好含数字的其他条目。
func parseTaskIDName(name string) (uint, bool) {
	var id uint
	if _, err := fmt.Sscanf(name, "%d", &id); err != nil {
		return 0, false
	}
	if fmt.Sprint(id) != name {
		return 0, false
	}
	return id, true
}

// newestMTime 返回路径（文件或目录）树内最新的修改时间。
// 路径不存在时返回零值时间（于是 freshEnough 判定为"不新鲜"，允许清理）。
func newestMTime(path string) time.Time {
	var newest time.Time
	fi, err := os.Stat(path)
	if err != nil {
		return newest
	}
	newest = fi.ModTime()
	if !fi.IsDir() {
		return newest
	}
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, werr error) error {
		if werr != nil || info == nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

