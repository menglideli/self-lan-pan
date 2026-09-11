package handler

import (
	"net"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"cloudpan/internal/dto"
)

// ---- 本机访问地址枚举 ----
//
// 为什么要有这个端点：以前前端用 location.origin 拼「手机/别的电脑该填什么地址」，
// 等于假设「你用什么地址打开页面，别的设备就用什么地址」。多网卡的机器上这是错的 ——
// 用户常常用 A 网卡的地址打开网页，却要把链接发给连在 B 网段（甚至 B 网卡）上的手机。
// 这里把本机所有可用地址一次列全，由用户自己挑要复制哪一条。

// LanAddress 一条可供外部设备访问的入口地址
type LanAddress struct {
	Iface string `json:"iface"` // 网卡名，如 WLAN / 以太网 / VMware Network Adapter VMnet8
	IP    string `json:"ip"`
	URL   string `json:"url"`  // http://<ip>:<port>
	Kind  string `json:"kind"` // lan(常规局域网) | virtual(虚拟网卡/组网) | other(其他)
}

// Addresses 列出本机全部可访问地址（需登录；单用户部署下即管理员本人）
func (h *SiteHandler) Addresses(c *gin.Context) {
	dto.OK(c, LocalAddresses(h.Cfg.Port))
}

// virtualIfaceHints 虚拟网卡/覆盖网络的名字特征。命中只降级排序并如实标注，
// 不过滤掉 —— 用 VMware 网段或 ZeroTier/Tailscale 组网做互通是很常见的正当用法。
var virtualIfaceHints = []string{
	"vmware", "virtualbox", "vbox", "hyper-v", "vethernet", "wsl",
	"docker", "veth", "tap", "tun", "loopback", "npcap", "bluetooth",
	"tailscale", "zerotier", "radmin", "hamachi", "openvpn", "wireguard",
}

// LocalAddresses 枚举本机 IPv4 访问地址，按「常规局域网 → 其他 → 虚拟网卡」排序。
// 启动日志与 /api/system/addresses 共用同一份实现，避免两处规则不一致。
func LocalAddresses(port string) []LanAddress {
	out := []LanAddress{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, aerr := ifi.Addrs()
		if aerr != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			// 只要可用的单播地址：排除 0.0.0.0 / 组播 / 广播
			if !ip4.IsGlobalUnicast() {
				continue
			}
			// 169.254.x.x（APIPA）表示该网卡没拿到地址，给出去也没用
			if ip4.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, LanAddress{
				Iface: ifi.Name,
				IP:    ip4.String(),
				URL:   "http://" + ip4.String() + ":" + port,
				Kind:  classifyIface(ifi.Name, ip4),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if rank(out[i].Kind) != rank(out[j].Kind) {
			return rank(out[i].Kind) < rank(out[j].Kind)
		}
		a, b := net.ParseIP(out[i].IP).To4(), net.ParseIP(out[j].IP).To4()
		for k := 0; k < 4; k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return out[i].Iface < out[j].Iface
	})
	return out
}

func rank(kind string) int {
	switch kind {
	case "lan":
		return 0
	case "other":
		return 1
	default:
		return 2
	}
}

// classifyIface 判定地址类型：虚拟网卡优先，其次是私有网段的常规局域网
func classifyIface(name string, ip net.IP) string {
	low := strings.ToLower(name)
	for _, h := range virtualIfaceHints {
		if strings.Contains(low, h) {
			return "virtual"
		}
	}
	if isPrivateV4(ip) {
		return "lan"
	}
	return "other"
}

// isPrivateV4 RFC1918 私有地址（含 CGNAT 100.64/10，运营商大内网也会用到）
func isPrivateV4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 10 ||
		(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
		(ip4[0] == 192 && ip4[1] == 168) ||
		(ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127)
}
