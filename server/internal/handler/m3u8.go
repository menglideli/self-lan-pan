package handler

// ---- HLS(m3u8) 远程下载 ----
//
// 对齐 N_m3u8DL-CLI 的核心能力子集：
//   1. 解析 master playlist（#EXT-X-STREAM-INF），按带宽自动选最高码率；
//   2. 解析 media playlist（#EXTINF / #EXT-X-BYTERANGE / #EXT-X-ENDLIST）；
//   3. 并发抓取分片（默认 8 线程，失败重试 3 次）；
//   4. AES-128-CBC 解密（#EXT-X-KEY，显式 IV 或按媒体序号推导）；
//   5. 初始化段 #EXT-X-MAP（fMP4）与按字节范围分片；
//   6. 按序合并为单个 .ts 落盘入网盘，进度按分片数上报。
//
// 刻意不引第三方 m3u8 解析库：这里只做"读"这一件事，而 IV 推导、BYTERANGE 续接、
// 引号内逗号这些细节自己实现反而更可控，也贴合本项目"单 exe、依赖尽量少"的取向。
//
// 说明：输出 .ts 原始分片流（不做转码/封装转换）。Windows 自带播放器不认 TS，
// 用 VLC / PotPlayer / mpv / 手机端播放器都可以直接播放。

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloudpan/internal/fscore"
	"cloudpan/internal/model"
)

const (
	// defaultOfflineUA 直链下载与 m3u8 分片共用：模拟浏览器，降低 CDN 防盗链 403
	defaultOfflineUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

	m3u8MaxSegs    = 20000           // 分片数上限，防恶意清单拖爆
	m3u8MaxBytes   = int64(20) << 30 // 20GB，与离线下载硬上限一致
	m3u8DefThreads = 8
	m3u8MaxThreads = 32
	m3u8MaxDepth   = 3 // master 嵌套层数上限
)

// m3u8Props HLS 任务属性（写入 Task.Props，运行中回写进度字段）
type m3u8Props struct {
	URL      string `json:"url"`
	PolicyID uint   `json:"policyId"`
	Dest     string `json:"dest"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // m3u8
	Referer  string `json:"referer,omitempty"`
	UA       string `json:"ua,omitempty"`
	Threads  int    `json:"threads,omitempty"`
	// 运行时回写展示字段
	RTName   string `json:"rtName,omitempty"` // 实际落盘名（用户填的 / YYYYMMDD_NN.mp4）
	SegDone  int    `json:"segDone,omitempty"`
	SegTotal int    `json:"segTotal,omitempty"`
	Variant  string `json:"variant,omitempty"`
	Live     bool   `json:"live,omitempty"`
	// Note 任务成功后的补充说明（如"有 3 个分片失败已跳过"）。
	// 不能塞进 Task.Msg：那个字段在任务结束时会被清空，否则界面会一直显示失效的阶段提示。
	Note string `json:"note,omitempty"`
}

// ---- playlist 模型 ----

type m3u8Key struct {
	Method string
	URI    string
	IV     []byte // 16 字节；nil = 按媒体序号推导
}

type m3u8Variant struct {
	URL        string
	Bandwidth  int64
	Resolution string
}

type m3u8Seg struct {
	URL       string
	Key       *m3u8Key
	RangeFrom int64
	RangeLen  int64 // 0 = 无 byte range
	Duration  float64
	// seq 分片媒体序号 = #EXT-X-MEDIA-SEQUENCE + 下标。规范规定：清单没给显式 IV 时，
	// AES-128 的 IV 就是它（16 字节大端）。由 runM3U8 在解析完清单后统一填。
	seq int64
}

type m3u8Playlist struct {
	IsMaster bool
	Variants []m3u8Variant
	InitSeg  string
	Segs     []m3u8Seg
	MediaSeq int64
	HasEnd   bool
}

// isM3U8URL 按路径后缀判定；没有后缀的（"…/play?sign=…" 这类）靠响应头兜底
func isM3U8URL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	return strings.HasSuffix(p, ".m3u8") || strings.HasSuffix(p, ".m3u")
}

// looksLikeM3U8Body 内容嗅探：清单必然以 #EXTM3U 开头
func looksLikeM3U8Body(b []byte, contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "x-mpegurl") {
		return true
	}
	head := b
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.HasPrefix(bytes.TrimSpace(head), []byte("#EXTM3U"))
}

// parseM3U8 解析清单。base 用于把相对 URI 解析成绝对地址（分片常写相对路径）。
func parseM3U8(body string, base *url.URL) (*m3u8Playlist, error) {
	pl := &m3u8Playlist{}
	text := strings.ReplaceAll(body, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "#EXTM3U") {
		return nil, fmt.Errorf("不是有效的 m3u8 清单（缺少 #EXTM3U 头）")
	}

	var curKey *m3u8Key
	var lastRangeEnd int64 = -1
	var pendingDur float64
	var pendingRange *[2]int64
	var pendingVar m3u8Variant
	expectVariant := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			tag, val := splitM3U8Tag(line)
			switch tag {
			case "#EXT-X-STREAM-INF":
				attrs := parseAttrList(val)
				pendingVar = m3u8Variant{
					Bandwidth:  atoi64(attrs["BANDWIDTH"]),
					Resolution: strings.TrimSpace(attrs["RESOLUTION"]),
				}
				expectVariant = true
			case "#EXT-X-KEY":
				attrs := parseAttrList(val)
				method := strings.ToUpper(strings.TrimSpace(attrs["METHOD"]))
				if method == "" || method == "NONE" {
					curKey = nil
					break
				}
				k := &m3u8Key{Method: method, URI: resolveRef(base, strings.TrimSpace(attrs["URI"]))}
				if ivs := strings.TrimSpace(attrs["IV"]); ivs != "" {
					ivs = strings.TrimPrefix(strings.TrimPrefix(ivs, "0x"), "0X")
					if b, herr := hex.DecodeString(ivs); herr == nil && len(b) == aes.BlockSize {
						k.IV = b
					}
				}
				curKey = k
			case "#EXT-X-MAP":
				if uri := parseAttrList(val)["URI"]; uri != "" {
					pl.InitSeg = resolveRef(base, uri)
				}
			case "#EXT-X-MEDIA-SEQUENCE":
				pl.MediaSeq, _ = strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			case "#EXT-X-ENDLIST":
				pl.HasEnd = true
			case "#EXTINF":
				d := val
				if i := strings.IndexByte(d, ','); i >= 0 {
					d = d[:i]
				}
				pendingDur, _ = strconv.ParseFloat(strings.TrimSpace(d), 64)
			case "#EXT-X-BYTERANGE":
				if n, off, ok := parseByteRange(val); ok {
					if off < 0 {
						off = lastRangeEnd // 无显式 offset 时紧接上一段
					}
					if off >= 0 {
						pendingRange = &[2]int64{off, n}
					}
				}
			}
			continue
		}

		u := resolveRef(base, line)
		if u == "" {
			continue
		}
		if expectVariant {
			pendingVar.URL = u
			pl.Variants = append(pl.Variants, pendingVar)
			pendingVar = m3u8Variant{}
			expectVariant = false
			continue
		}
		seg := m3u8Seg{URL: u, Key: curKey, Duration: pendingDur}
		if pendingRange != nil {
			seg.RangeFrom, seg.RangeLen = pendingRange[0], pendingRange[1]
			lastRangeEnd = pendingRange[0] + pendingRange[1]
			pendingRange = nil
		}
		pl.Segs = append(pl.Segs, seg)
	}

	// master 与 media 的区分看"有没有变体行"，不能只看有没有 #EXTINF
	pl.IsMaster = len(pl.Variants) > 0 && len(pl.Segs) == 0
	return pl, nil
}

// splitM3U8Tag "#EXT-X-KEY:METHOD=…" -> ("#EXT-X-KEY", "METHOD=…")
func splitM3U8Tag(line string) (string, string) {
	if i := strings.IndexByte(line, ':'); i >= 0 {
		return line[:i], line[i+1:]
	}
	return line, ""
}

// parseAttrList 解析 HLS 属性列表。注意 URI="a,b" 里的逗号在引号内，不能当分隔符。
func parseAttrList(s string) map[string]string {
	m := map[string]string{}
	for _, part := range splitOutsideQuotes(s, ',') {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToUpper(strings.TrimSpace(kv[0]))
		// URI 值可能带引号，去掉首尾引号
		v := strings.TrimSpace(kv[1])
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		m[k] = v
	}
	return m
}

func splitOutsideQuotes(s string, sep byte) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
			cur.WriteByte(c)
			continue
		}
		if c == sep && !inQuote {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	out = append(out, cur.String())
	return out
}

func resolveRef(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base == nil {
		return u.String()
	}
	return base.ResolveReference(u).String()
}

// parseByteRange "#EXT-X-BYTERANGE:<n>[@<o>]"；无 offset 返回 -1
func parseByteRange(v string) (n int64, off int64, ok bool) {
	v = strings.TrimSpace(v)
	off = -1
	if i := strings.IndexByte(v, '@'); i >= 0 {
		n, _ = strconv.ParseInt(strings.TrimSpace(v[:i]), 10, 64)
		off, _ = strconv.ParseInt(strings.TrimSpace(v[i+1:]), 10, 64)
	} else {
		n, _ = strconv.ParseInt(v, 10, 64)
	}
	return n, off, n > 0
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// pickVariant 选码率最高的一路；没有 BANDWIDTH 信息时退回第一个
func pickVariant(vs []m3u8Variant) m3u8Variant {
	if len(vs) == 0 {
		return m3u8Variant{}
	}
	sorted := make([]m3u8Variant, len(vs))
	copy(sorted, vs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Bandwidth > sorted[j].Bandwidth })
	return sorted[0]
}

// ---- 抓取 ----

// m3u8Fetcher 清单/分片/密钥的统一抓取器（走 ssrfHTTP：保留 SSRF 三层防护）
type m3u8Fetcher struct {
	ua      string
	referer string

	keyMu    sync.Mutex
	keyCache map[string][]byte
}

// get 发一个 GET；byteRange 非 0 时带 Range 头
func (f *m3u8Fetcher) get(rawURL string, rng *[2]int64) (*http.Response, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.ua)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if f.referer != "" {
		req.Header.Set("Referer", f.referer)
	}
	if rng != nil && rng[1] > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", rng[0], rng[0]+rng[1]-1))
	}
	return ssrfHTTP.Do(req)
}

// getBody 抓整段内容（清单、密钥用）
func (f *m3u8Fetcher) getBody(rawURL string, limit int64) ([]byte, string, error) {
	resp, err := f.get(rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, "", err
	}
	final := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String() // 跟随重定向后的真实地址，用于解析相对分片
	}
	return b, final, nil
}

// getSegment 抓一个分片；服务器忽略 Range 返回 200 时按 offset/len 自行截取
func (f *m3u8Fetcher) getSegment(seg m3u8Seg) ([]byte, error) {
	var rng *[2]int64
	if seg.RangeLen > 0 {
		rng = &[2]int64{seg.RangeFrom, seg.RangeLen}
	}
	resp, err := f.get(seg.URL, rng)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, m3u8MaxBytes))
	if err != nil {
		return nil, err
	}
	if rng != nil && resp.StatusCode == http.StatusOK {
		// 服务器不支持 Range：自行切出需要的区间
		from, n := seg.RangeFrom, seg.RangeLen
		if from > int64(len(data)) {
			return nil, fmt.Errorf("字节范围超出响应长度")
		}
		end := from + n
		if end > int64(len(data)) {
			end = int64(len(data))
		}
		data = data[from:end]
	}
	if seg.Key != nil {
		if !strings.EqualFold(seg.Key.Method, "AES-128") {
			return nil, fmt.Errorf("不支持的分片加密方式 %s（仅支持 AES-128）", seg.Key.Method)
		}
		key, kerr := f.loadKey(seg.Key.URI)
		if kerr != nil {
			return nil, fmt.Errorf("获取解密密钥失败: %w", kerr)
		}
		iv := seg.Key.IV
		if iv == nil {
			// 规范：未给 IV 时用分片媒体序号（大端）当 IV
			iv = ivFromSeq(seg.seq)
		}
		data, err = decryptAES128CBC(data, key, iv)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (f *m3u8Fetcher) loadKey(uri string) ([]byte, error) {
	if uri == "" {
		return nil, fmt.Errorf("清单声明了加密但未给出密钥地址")
	}
	f.keyMu.Lock()
	if f.keyCache == nil {
		f.keyCache = map[string][]byte{}
	}
	if k, ok := f.keyCache[uri]; ok {
		f.keyMu.Unlock()
		return k, nil
	}
	f.keyMu.Unlock()

	b, _, err := f.getBody(uri, 1<<20)
	if err != nil {
		return nil, err
	}
	if len(b) < 16 {
		return nil, fmt.Errorf("密钥长度不足 16 字节（实际 %d）", len(b))
	}
	key := b[:16]
	f.keyMu.Lock()
	f.keyCache[uri] = key
	f.keyMu.Unlock()
	return key, nil
}

// seg.seq 由调用方填充（媒体序号），用于 IV 推导 —— 见 runM3U8 的标注
func ivFromSeq(seq int64) []byte {
	iv := make([]byte, aes.BlockSize)
	binary.BigEndian.PutUint64(iv[8:], uint64(seq))
	return iv
}

// decryptAES128CBC AES-128-CBC + PKCS7 去填充（HLS 分片标准封装）
func decryptAES128CBC(data, key, iv []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("密文长度 %d 不是 16 的整数倍，密钥/IV 可能不匹配", len(data))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	if n := int(out[len(out)-1]); n > 0 && n <= aes.BlockSize && n <= len(out) {
		valid := true
		for _, b := range out[len(out)-n:] {
			if int(b) != n {
				valid = false
				break
			}
		}
		if valid {
			out = out[:len(out)-n]
		}
	}
	return out, nil
}

// ---- 任务执行 ----

// runM3U8 HLS 下载主流程。注意：分片序号 = 清单的 #EXT-X-MEDIA-SEQUENCE + 下标，
// 这个序号既用于 IV 推导也用于进度展示的准确性。
func (p *TaskPool) runM3U8(t *model.Task) error {
	var props m3u8Props
	if err := json.Unmarshal([]byte(t.Props), &props); err != nil {
		return err
	}
	return p.runM3U8Task(t, props, "", "")
}

// runM3U8Task 实际执行体。preloaded 非空表示已由直链探测阶段拿到清单正文
// （地址没带 .m3u8 后缀的情况），直接用它可以省掉一次请求，
// 也避免个别站点的一次性签名地址被请求两次而失效。
func (p *TaskPool) runM3U8Task(t *model.Task, props m3u8Props, preloaded, preloadedURL string) error {
	var policy model.Policy
	if err := model.DB.First(&policy, props.PolicyID).Error; err != nil {
		return fmt.Errorf("存储策略不存在")
	}
	d, err := p.Svc.DriverFor(&policy, userOfID(t.UserID))
	if err != nil {
		return err
	}

	ua := strings.TrimSpace(props.UA)
	if ua == "" {
		ua = defaultOfflineUA
	}
	f := &m3u8Fetcher{ua: ua, referer: strings.TrimSpace(props.Referer)}

	p.setTaskMsg(t.ID, "正在解析 m3u8 清单...")
	var body []byte
	var finalURL string
	if preloaded != "" {
		body, finalURL = []byte(preloaded), preloadedURL
	} else {
		body, finalURL, err = f.getBody(props.URL, 8<<20)
		if err != nil {
			if !SSRFAllowPrivate() {
				return fmt.Errorf("读取清单失败: %v（若该地址在内网，需在管理台开启「允许离线下载访问内网地址」）", err)
			}
			return fmt.Errorf("读取清单失败: %v", err)
		}
	}
	base, perr := url.Parse(finalURL)
	if perr != nil {
		base = nil
	}
	// 防盗链兜底：多数站点要求 Referer 与站点同源
	if f.referer == "" && base != nil {
		f.referer = base.Scheme + "://" + base.Host + "/"
	}

	var pl *m3u8Playlist
	cur := string(body)
	for depth := 0; depth < m3u8MaxDepth; depth++ {
		pl, err = parseM3U8(cur, base)
		if err != nil {
			return err
		}
		if !pl.IsMaster {
			break
		}
		v := pickVariant(pl.Variants)
		if v.URL == "" {
			return fmt.Errorf("master 清单里没有可用码流")
		}
		props.Variant = v.Resolution
		if props.Variant == "" && v.Bandwidth > 0 {
			props.Variant = fmt.Sprintf("%dkbps", v.Bandwidth/1000)
		}
		p.saveAnyProps(t.ID, props)
		p.setTaskMsg(t.ID, "已选最高码率码流"+" "+props.Variant+"，正在读取分片清单...")
		nb, nf, nerr := f.getBody(v.URL, 8<<20)
		if nerr != nil {
			return fmt.Errorf("读取码流清单失败: %v", nerr)
		}
		if nbase, e := url.Parse(nf); e == nil {
			base = nbase
		}
		cur = string(nb)
		pl = nil
	}
	if pl == nil {
		return fmt.Errorf("m3u8 嵌套层级过深（超过 %d 层）", m3u8MaxDepth)
	}
	if len(pl.Segs) == 0 {
		if len(pl.Variants) > 0 {
			return fmt.Errorf("解析到的仍是 master 清单，未找到实际分片")
		}
		return fmt.Errorf("清单里没有分片。若这是直播流，请确认链接是 m3u8（部分站点需带播放器请求头）")
	}
	if len(pl.Segs) > m3u8MaxSegs {
		return fmt.Errorf("分片数 %d 超过上限 %d，已拒绝", len(pl.Segs), m3u8MaxSegs)
	}
	// 媒体序号：IV 推导与分片定位都依赖它
	for i := range pl.Segs {
		pl.Segs[i].seq = pl.MediaSeq + int64(i)
	}
	props.SegTotal = len(pl.Segs)
	props.Live = !pl.HasEnd
	p.saveAnyProps(t.ID, props)
	if props.Live {
		// 直播/事件流的这个事实在任务结束后依然有意义 → 写进 props.Note 长期保留
		props.Note = fmt.Sprintf("该清单无 #EXT-X-ENDLIST（直播/事件流），只保存了本次抓取到的 %d 个分片", len(pl.Segs))
		p.saveAnyProps(t.ID, props)
		p.setTaskMsg(t.ID, props.Note)
	}

	// 输出文件名：用户填了就用他的，没填就用「YYYYMMDD_NN」；没有扩展名一律补 .mp4
	name, err := resolveOutName(d, props.Dest, props.Name, mediaDefaultExt)
	if err != nil {
		return err
	}
	// 与既有文件同名时自动改名为 name_1.mp4，绝不覆盖用户盘里的东西
	name, err = uniqueFileName(d, props.Dest, name)
	if err != nil {
		return err
	}
	props.RTName = name
	p.saveAnyProps(t.ID, props)
	target, err := fscore.Join(props.Dest, name)
	if err != nil {
		return err
	}

	// 分片落临时目录，全部完成后按序合并
	tmpDir, err := os.MkdirTemp(p.Zips, "m3u8-")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	threads := props.Threads
	if threads <= 0 {
		threads = m3u8DefThreads
	}
	if threads > m3u8MaxThreads {
		threads = m3u8MaxThreads
	}
	if threads > len(pl.Segs) {
		threads = len(pl.Segs)
	}

	type segResult struct {
		file string
		size int64
		err  error
	}
	results := make([]segResult, len(pl.Segs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done, softErr := 0, 0
	var totalBytes int64
	var firstErr error
	var abort atomic.Bool // 大面积失败时止损：投递停止，worker 快速跳过剩余项
	step := len(pl.Segs) / 100
	if step < 1 {
		step = 1
	}
	// 前若干分片全挂就说明链接/请求头不对，早点退出比跑完 20000 个分片好
	const earlyFailProbe = 16

	for w := 0; w < threads; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if p.cancelled(t.ID) || abort.Load() {
					results[idx] = segResult{err: fmt.Errorf("已中止")}
					continue
				}
				seg := pl.Segs[idx]
				var data []byte
				var derr error
				for attempt := 0; attempt < 3; attempt++ {
					data, derr = f.getSegment(seg)
					if derr == nil {
						break
					}
					if p.cancelled(t.ID) {
						break
					}
					time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
				}
				res := segResult{err: derr}
				if derr == nil {
					fp := filepath.Join(tmpDir, fmt.Sprintf("seg_%06d.part", idx))
					if werr := os.WriteFile(fp, data, 0o644); werr != nil {
						res.err = werr
					} else {
						res.file = fp
						res.size = int64(len(data))
					}
				}
				results[idx] = res

				mu.Lock()
				if res.err != nil {
					softErr++
					if firstErr == nil {
						firstErr = fmt.Errorf("分片 %d 下载失败: %w", idx, res.err)
					}
				} else {
					done++
					totalBytes += res.size
				}
				cur, failed := done, softErr
				pct := done * 100 / len(pl.Segs)
				if pct < 100 {
					p.setProgress(t.ID, pct)
				}
				// 开头就全挂 → 止损（避免坏链接把 20000 个分片都重试一遍）
				if cur == 0 && failed >= earlyFailProbe {
					abort.Store(true)
				}
				mu.Unlock()

				if cur%step == 0 || cur == len(pl.Segs) {
					props.SegDone = cur
					props.SegTotal = len(pl.Segs)
					p.saveAnyProps(t.ID, props)
					p.setTaskMsg(t.ID, fmt.Sprintf("正在下载分片 %d/%d", cur, len(pl.Segs)))
				}
			}
		}()
	}
	for i := range pl.Segs {
		if p.cancelled(t.ID) || abort.Load() {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if p.cancelled(t.ID) {
		return fmt.Errorf("已取消")
	}
	if done == 0 {
		if firstErr != nil {
			return fmt.Errorf("全部分片下载失败：%w", firstErr)
		}
		return fmt.Errorf("没有下载到任何分片")
	}
	if totalBytes > m3u8MaxBytes {
		return fmt.Errorf("下载量 %dMB 超过 20GB 上限，已中止", totalBytes>>20)
	}
	// 配额预检（与离线下载同一套记账）
	if limit, limited := effectiveQuotaBytes(userOfID(t.UserID), model.AdminPerms()); limited {
		var u model.User
		if model.DB.First(&u, t.UserID).Error == nil && u.UsedBytes+totalBytes > limit {
			return fmt.Errorf("超出配额（已用 %dMB / 上限 %dMB），未保存", u.UsedBytes>>20, limit>>20)
		}
	}

	p.setTaskMsg(t.ID, "正在合并分片...")
	outPath := filepath.Join(tmpDir, "out.ts")
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	var written int64
	// 初始化段（fMP4 的 #EXT-X-MAP）必须排在最前
	if pl.InitSeg != "" {
		initData, ierr := f.getSegment(m3u8Seg{URL: pl.InitSeg, seq: 0})
		if ierr != nil {
			out.Close()
			return fmt.Errorf("初始化段下载失败: %w", ierr)
		}
		if _, werr := out.Write(initData); werr != nil {
			out.Close()
			return werr
		}
		written += int64(len(initData))
	}
	for _, r := range results {
		if r.err != nil || r.file == "" {
			continue
		}
		src, oerr := os.Open(r.file)
		if oerr != nil {
			out.Close()
			return oerr
		}
		n, cerr := io.Copy(out, src)
		src.Close()
		if cerr != nil {
			out.Close()
			return cerr
		}
		written += n
		_ = os.Remove(r.file) // 边合并边清理，避免临时目录堆满
	}
	if err := out.Close(); err != nil {
		return err
	}

	fh, err := os.Open(outPath)
	if err != nil {
		return err
	}
	defer fh.Close()
	if err := d.CreateFile(target, fh); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	addQuota(t.UserID, written)
	// 有分片失败时的说明写进 props.Note（不是 Task.Msg —— 那个字段在 run() 成功分支里会被清空）
	if softErr > 0 {
		note := fmt.Sprintf("有 %d/%d 个分片下载失败已跳过，产物可能缺段", softErr, len(pl.Segs))
		if props.Note != "" {
			props.Note += "；" + note
		} else {
			props.Note = note
		}
		props.SegDone = done
		props.SegTotal = len(pl.Segs)
		p.saveAnyProps(t.ID, props)
	}
	return nil
}

// saveAnyProps 把任意 props 结构写回 Task.Props（btProps/m3u8Props 共用同一列）
func (p *TaskPool) saveAnyProps(id uint, v interface{}) {
	b, _ := json.Marshal(v)
	model.DB.Model(&model.Task{}).Where("id = ?", id).UpdateColumn("props", string(b))
}
