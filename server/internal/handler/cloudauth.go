package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cloudpan/internal/driver"
	"cloudpan/internal/dto"
	"cloudpan/internal/middleware"
	"cloudpan/internal/model"
)

func parseUintQuery(c *gin.Context, key string) uint {
	v, _ := strconv.ParseUint(c.Query(key), 10, 32)
	return uint(v)
}

// publicBaseOf 站点对外地址（云盘 OAuth 回调等第三方回拉用），优先级：
// 1) 环境变量 CP_PUBLIC_URL 显式设置（运维意图）
// 2) 站点设置 public_url（管理控制台填写，需第三方可达）
// 3) 当前请求的 Host 推导（浏览器能到达的地址）
// 注意：Cfg.PublicURL 未显式设置时只是 localhost 占位值，不代表运维意图，不作回退。
func publicBaseOf(site *SiteHandler, c *gin.Context) string {
	if site.Cfg.PublicURLOverridden {
		if b := strings.TrimRight(site.Cfg.PublicURL, "/"); b != "" {
			return b
		}
	}
	if b := strings.TrimRight(GetSiteSettings()["public_url"], "/"); b != "" {
		return b
	}
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// ---- 云盘授权 ----

// CloudAuth 云盘 OAuth 授权辅助
type CloudAuth struct{ Site *SiteHandler }

// AuthURL 返回指定策略类型的授权页地址。
// aliyun/baidu 走扫码绑定：授权页 redirect_uri 指向本系统 /api/cloud/callback（带签名 state），
// 用户手机扫码→厂商授权→厂商重定向回回调接口自动完成 token 交换，全程无需人工复制粘贴。
// 管理端前端把返回的 url 渲染成二维码（手机扫码）或提供链接（电脑浏览器打开均可）。
func (h *CloudAuth) AuthURL(c *gin.Context) {
	policyID := parseUintQuery(c, "policyId")
	var p model.Policy
	if err := model.DB.First(&p, policyID).Error; err != nil {
		dto.Fail(c, 404, "存储策略不存在")
		return
	}
	o := p.Opts()
	switch p.Type {
	case "aliyun", "baidu":
		// ?mode=code → 授权码粘贴回退（手机完全不可达本站时用：oob 地址页面上直接显示授权码）
		if c.Query("mode") == "code" {
			var u string
			if p.Type == "aliyun" {
				u = driver.AliyunAuthURL(o["client_id"], "oob", "")
			} else {
				u = driver.BaiduAuthURL(o["client_id"], "oob", "")
			}
			dto.OK(c, gin.H{"url": u, "mode": "code"})
			return
		}
		redirect := publicBaseOf(h.Site, c) + "/api/cloud/callback"
		st := h.signState(cloudAuthState{PolicyID: p.ID, Type: p.Type, Redirect: redirect})
		var u string
		if p.Type == "aliyun" {
			u = driver.AliyunAuthURL(o["client_id"], redirect, st)
		} else {
			u = driver.BaiduAuthURL(o["client_id"], redirect, st)
		}
		dto.OK(c, gin.H{"url": u, "mode": "qr", "redirect": redirect})
	case "pan123":
		dto.OK(c, gin.H{"url": "", "mode": "keys"}) // 123 直接用 clientID/secret，无需跳转
	case "tianyi":
		dto.OK(c, gin.H{"url": "", "mode": "cookie"})
	default:
		dto.OK(c, gin.H{"url": "", "mode": "none"})
	}
}

// cloudAuthState 云盘授权回调 state（HMAC 签名：防伪造回调、防跨策略注入 code、防重放）
type cloudAuthState struct {
	PolicyID uint   `json:"p"`
	Type     string `json:"t"`
	Redirect string `json:"r"` // 授权页所用 redirect_uri，换码时必须一致
	Exp      int64  `json:"exp"`
}

func (h *CloudAuth) signState(st cloudAuthState) string {
	st.Exp = time.Now().Add(30 * time.Minute).Unix()
	b, _ := json.Marshal(st)
	mac := hmac.New(sha256.New, h.Site.Cfg.Secret)
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + hex.EncodeToString(mac.Sum(nil))
}

func (h *CloudAuth) verifyState(tok string, out *cloudAuthState) error {
	dot := strings.IndexByte(tok, '.')
	if dot <= 0 || dot == len(tok)-1 {
		return fmt.Errorf("state 非法")
	}
	b, err := base64.RawURLEncoding.DecodeString(tok[:dot])
	if err != nil {
		return fmt.Errorf("state 非法")
	}
	mac := hmac.New(sha256.New, h.Site.Cfg.Secret)
	mac.Write(b)
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(tok[dot+1:])) {
		return fmt.Errorf("state 签名不匹配")
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("state 解析失败")
	}
	if time.Now().Unix() > out.Exp {
		return fmt.Errorf("授权链接已过期，请重新发起")
	}
	return nil
}

// Callback 云盘 OAuth 回调（公开路由：厂商把用户浏览器重定向到这里，此时浏览器未必登录本系统）。
// 校验 state → 授权码换 token → 写回策略 options → 渲染结果页。
func (h *CloudAuth) Callback(c *gin.Context) {
	var msg, cls, icon string
	fail := func(m string) {
		msg, cls, icon = m, "err", "✕"
	}
	st := cloudAuthState{}
	if err := h.verifyState(c.Query("state"), &st); err != nil {
		fail("授权状态校验失败：" + err.Error())
	} else if code := c.Query("code"); code == "" {
		fail("未收到授权码（用户可能取消了授权，或厂商应用未配置回调地址）")
	} else if c.Query("error") != "" {
		fail("厂商返回错误：" + c.Query("error"))
	} else {
		var p model.Policy
		if err := model.DB.First(&p, st.PolicyID).Error; err != nil {
			fail("存储策略不存在（可能已被删除）")
		} else if p.Type != st.Type {
			fail("策略类型不匹配")
		} else {
			o := p.Opts()
			switch st.Type {
			case "aliyun":
				refresh, _, err := driver.AliyunExchangeCode(o["client_id"], o["client_secret"], code, st.Redirect)
				if err != nil {
					fail("授权码换取 token 失败：" + err.Error())
				} else {
					o["refresh_token"] = refresh
				}
			case "baidu":
				access, refresh, err := driver.BaiduExchangeCode(o["client_id"], o["client_secret"], code, st.Redirect)
				if err != nil {
					fail("授权码换取 token 失败：" + err.Error())
				} else {
					o["access_token"] = access
					if refresh != "" {
						o["refresh_token"] = refresh
					}
				}
			default:
				fail("不支持的授权类型")
			}
			if msg == "" {
				b, _ := json.Marshal(o)
				model.DB.Model(&p).UpdateColumn("options", string(b))
				h.Site.Fs.Invalidate(p.ID)
				model.DB.Create(&model.AuditLog{UserID: 0, Username: "cloud-callback", Action: "cloud-bind",
					Detail: "云盘授权成功 " + p.Name + " (" + p.Type + ")"})
				msg, cls, icon = "云盘绑定成功，可以关闭此窗口返回 CloudPan 管理台。", "ok", "✓"
			}
		}
	}
	// 占位符替换（不走 fmt 格式化：模板 CSS 含 % 号，c.String 的 format 语义会错位）
	out := strings.NewReplacer("__CLS__", cls, "__ICON__", icon, "__MSG__", html.EscapeString(msg)).Replace(callbackHTML)
	c.Data(200, "text/html; charset=utf-8", []byte(out))
}

// callbackHTML 回调结果页（手机端打开，移动端友好）
const callbackHTML = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CloudPan · 云盘绑定</title><style>
body{font-family:system-ui,-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#f4f6fb}
.card{background:#fff;border-radius:18px;box-shadow:0 10px 34px rgba(30,50,90,.14);padding:44px 36px;text-align:center;max-width:380px;margin:16px}
.icon{width:68px;height:68px;border-radius:50%;margin:0 auto 18px;display:flex;align-items:center;justify-content:center;font-size:34px}
.ok .icon{background:#e7f7ee;color:#18a058}.err .icon{background:#fdecec;color:#d4380d}
h1{font-size:20px;margin:0 0 10px;color:#222}p{font-size:14px;color:#667;margin:0;line-height:1.8;word-break:break-all}
</style></head><body><div class="card __CLS__"><div class="icon">__ICON__</div><h1>CloudPan 云盘绑定</h1><p>__MSG__</p></div></body></html>`

// Exchange 用授权码换取 token 并写回策略 options
func (h *CloudAuth) Exchange(c *gin.Context) {
	var in struct {
		PolicyID uint   `json:"policyId" binding:"required"`
		Type     string `json:"type" binding:"required"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.Fail(c, 400, "参数错误")
		return
	}
	var p model.Policy
	if err := model.DB.First(&p, in.PolicyID).Error; err != nil {
		dto.Fail(c, 404, "存储策略不存在")
		return
	}
	o := p.Opts()
	switch in.Type {
	case "aliyun":
		refresh, _, err := driver.AliyunExchangeCode(o["client_id"], o["client_secret"], in.Code, "oob")
		if err != nil {
			dto.Fail(c, 400, err.Error())
			return
		}
		o["refresh_token"] = refresh
	case "baidu":
		acc, refresh, err := driver.BaiduExchangeCode(o["client_id"], o["client_secret"], in.Code, "oob")
		if err != nil {
			dto.Fail(c, 400, err.Error())
			return
		}
		o["access_token"] = acc
		if refresh != "" {
			o["refresh_token"] = refresh
		}
	default:
		dto.Fail(c, 400, "不支持的授权类型")
		return
	}
	b, _ := json.Marshal(o)
	model.DB.Model(&p).UpdateColumn("options", string(b))
	h.Site.Fs.Invalidate(p.ID)
	middleware.Audit(c, "admin", "云盘授权成功 "+p.Name)
	dto.OK(c, gin.H{"ok": true})
}

// Status 授权连通性探测
func (h *CloudAuth) Status(c *gin.Context) {
	policyID := parseUintQuery(c, "policyId")
	var p model.Policy
	if err := model.DB.First(&p, policyID).Error; err != nil {
		dto.Fail(c, 404, "存储策略不存在")
		return
	}
	if p.Type == "local" {
		dto.OK(c, gin.H{"ok": true, "msg": "本地存储"})
		return
	}
	d, err := h.Site.Fs.DriverFor(&p, nil) // 云盘连通性探测（本地策略已提前返回）
	if err != nil {
		dto.OK(c, gin.H{"ok": false, "msg": err.Error()})
		return
	}
	used, total, err := d.Quota()
	if err != nil {
		dto.OK(c, gin.H{"ok": false, "msg": err.Error()})
		return
	}
	dto.OK(c, gin.H{"ok": true, "used": used, "total": total})
}
