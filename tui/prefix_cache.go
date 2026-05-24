package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"deepx/agent"
	"deepx/mcp"
	"deepx/tools"

	tea "charm.land/bubbletea/v2"
)

// 空闲压缩:会话静默超过 idleCompactAfter 即跑一次缓存友好压缩;idleCheckInterval 是轮询粒度。
const (
	idleCompactAfter  = time.Hour
	idleCheckInterval = time.Minute
)

// idleCheckMsg 由自循环定时器投递,触发一次空闲检查。
type idleCheckMsg struct{}

// idleCheckCmd 调度下一次空闲检查(自循环)。
func idleCheckCmd() tea.Cmd {
	return tea.Tick(idleCheckInterval, func(time.Time) tea.Msg { return idleCheckMsg{} })
}

// shouldIdleCompact 判断是否到了该跑空闲压缩的条件:
// 有 session/Pro、未在流式、本空闲期还没压过、静默已达阈值、且历史够大(复用重启下限)。
func (m *model) shouldIdleCompact() bool {
	if m.session == nil || m.models.Pro.Model == "" || m.streaming || m.idleCompacted || m.compacting {
		return false
	}
	if m.lastActivityAt.IsZero() || time.Since(m.lastActivityAt) < idleCompactAfter {
		return false
	}
	ctxWin := m.models.Pro.ContextWindow
	if ctxWin <= 0 {
		ctxWin = 65536
	}
	return m.lastPromptTokens() >= ctxWin*restartCompactFloorPct/100
}

// idleCompactionCmd 跑一次缓存友好压缩(复刻当前快照前缀,趁缓存可能仍热命中)。
func (m *model) idleCompactionCmd() tea.Cmd {
	snapshot := append([]agent.ChatMessage(nil), m.history...)
	_, lastModel, lastSys, lastTools := m.session.LoadPrefixSnapshot()
	entry := m.entryForModel(lastModel) // 用缓存那段历史的同一模型才命中
	ctxWin := m.models.Pro.ContextWindow // 尺寸口径统一用 Pro 的窗口
	if ctxWin <= 0 {
		ctxWin = 65536
	}
	return func() tea.Msg {
		summary, cutIdx, compressedTurns, err := runCompression(lastSys, lastTools, snapshot, entry, ctxWin)
		return compressionResultMsg{
			summary:         summary,
			cutIdx:          cutIdx,
			compressedTurns: compressedTurns,
			err:             err,
		}
	}
}

// restartCompactFloorPct:重启检测到前缀变化时,历史 ≥ ctxWin 的这个百分比才触发自动压缩。
// 低于此量,冷的首请求本来就便宜,直接扛着、让缓存在新前缀上自然回暖即可。暂定 25%。
const restartCompactFloorPct = 25

// prefixSignature 计算前缀变化检测用的签名:直接对"会进缓存前缀的实际内容"取指纹:
//
//	hash(核心系统提示词文本 + 内置工具 catalog JSON + 排序后的 mcp.json 配置)
//
// - 核心系统提示词 = BuildSystemPrompt(workspace, skill, "") —— 含提示词正文 + workspace + skill 目录,
//   不含会话摘要(摘要变化是压缩的正常结果,不应触发"重启压缩";两侧都不含,天然可比)。
// - 内置工具 catalog:遍历 tools.Tools 取 ToOpenAISpec —— 工具增删/改 schema 都会变。
// - mcp 配置:取自 mcp.json(server 身份),非实时连上的工具,避免异步上线/连接失败抖动。
//
// 这样 prompt 文本 / 工具 / skill / mcp 任一改动都能检测到,dev(go run)和发布版均生效,
// 不再依赖 version 代理(之前手改提示词、go run 时 version 恒为 "dev",签名不变 → 漏检)。
func (m *model) prefixSignature() string {
	core := agent.BuildSystemPrompt(m.workspace, m.skillCatalog, "")

	specs := make([]tools.OpenAIToolSpec, 0, len(tools.Tools))
	for _, t := range tools.Tools {
		specs = append(specs, t.ToOpenAISpec())
	}
	toolsJSON := agent.MarshalToolSpecs(specs)

	servers, _ := mcp.LoadConfig()
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	mcpJSON, _ := json.Marshal(servers)

	h := sha256.New()
	h.Write([]byte(core))
	h.Write([]byte{0})
	h.Write([]byte(toolsJSON))
	h.Write([]byte{0})
	h.Write(mcpJSON)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// onPrefixSnapshot 持久化本轮"实际发送"的前缀(供压缩复刻热缓存)+ 当前稳定签名(供重启检测)。
func (m *model) onPrefixSnapshot(msg agent.PrefixSnapshotMsg) {
	if m.session == nil {
		return
	}
	sig := m.prefixSignature()
	m.session.SavePrefixSnapshot(sig, msg.Model, msg.SystemPrompt, msg.ToolSpecsJSON)
}

// lastPromptTokens 返回"下一次请求 prompt 大约多大"的 token 数,用于压缩触发判断。
// 优先用 API 上次返回的真实 prompt_tokens(精确);若没有(后端没返回 usage)则退回本地估算。
func (m *model) lastPromptTokens() int {
	if m.lastUsage != nil && m.lastUsage.PromptTokens > 0 {
		return m.lastUsage.PromptTokens
	}
	return m.estimatePromptTokens()
}

// estimatePromptTokens 本地估算 prompt token 数(API 没返回 usage 时的兜底)。
//
// 算法:把真实 prompt 的三块字符数全加起来,再按 ~3 字符/token 换算。
//
//	prompt ≈ ( 系统提示词文本                                  // BuildSystemPrompt(workspace, skill, summary)
//	         + 工具定义 JSON(内置全集 + 当前 MCP)             // 各 tool 的 ToOpenAISpec 序列化
//	         + 历史每条消息(Content + ReasoningContent
//	                        + ContentParts.Text + ToolCalls 的 Name/Arguments) )  // m.history 全字段
//	         / 3
//
// 这三块正好对应请求体的 system 消息 + tools 字段 + messages 数组。/3 是经验比例(中文~1.5、
// 英文~4,混合取中),只为给阈值一个量级,±20% 可接受。它会比真值略有出入,但永远有值、不依赖 API。
func (m *model) estimatePromptTokens() int {
	chars := len([]rune(agent.BuildSystemPrompt(m.workspace, m.skillCatalog, m.summary)))

	specs := make([]tools.OpenAIToolSpec, 0, len(tools.Tools))
	for _, t := range tools.Tools {
		specs = append(specs, t.ToOpenAISpec())
	}
	for _, t := range tools.MCPTools() {
		specs = append(specs, t.ToOpenAISpec())
	}
	chars += len([]rune(agent.MarshalToolSpecs(specs)))

	for _, msg := range m.history {
		chars += len([]rune(msg.Content)) + len([]rune(msg.ReasoningContent))
		for _, p := range msg.ContentParts {
			chars += len([]rune(p.Text))
		}
		for _, tc := range msg.ToolCalls {
			chars += len([]rune(tc.Function.Name)) + len([]rune(tc.Function.Arguments))
		}
	}
	return chars / 3
}

// entryForModel 按 model ID 取对应的 ModelEntry:命中 flash 则用 flash,否则退 pro。
// 缓存按模型分,压缩必须用"缓存那段历史的同一模型"才命中(见 DeepSeek 缓存讨论)。
func (m *model) entryForModel(id string) agent.ModelEntry {
	if id != "" && m.models.Flash.Model == id {
		return m.models.Flash
	}
	return m.models.Pro
}

// detectRestartCompaction 在启动加载历史后调用:若签名相对上次会话变了、且历史够大,
// 暂存上次的前缀快照(oldSys/oldTools)并返回 true,表示需要在首请求前跑一次缓存友好压缩。
func (m *model) detectRestartCompaction() bool {
	if m.session == nil || m.models.Pro.Model == "" {
		return false
	}
	persistedSig, oldModel, oldSys, oldTools := m.session.LoadPrefixSnapshot()
	if persistedSig == "" || oldSys == "" {
		return false // 首次运行 / 无快照,没有可失效的缓存
	}
	if m.prefixSignature() == persistedSig {
		return false // prompt / 工具 / mcp 均未变,缓存仍有效,不压
	}
	ctxWin := m.models.Pro.ContextWindow
	if ctxWin <= 0 {
		ctxWin = 65536
	}
	if m.lastPromptTokens() < ctxWin*restartCompactFloorPct/100 {
		return false // 历史太小,冷首请求本来就便宜,不值得压
	}
	m.pendingCompactModel = oldModel
	m.pendingCompactSys = oldSys
	m.pendingCompactTools = oldTools
	m.compacting = true // Init 会 fire restartCompactionCmd;占住锁防与首轮 70% 触发并发
	return true
}

// restartCompactionCmd 返回一个在首请求前执行的缓存友好压缩 Cmd(复刻旧前缀命中热缓存)。
func (m *model) restartCompactionCmd() tea.Cmd {
	snapshot := append([]agent.ChatMessage(nil), m.history...)
	oldSys, oldTools := m.pendingCompactSys, m.pendingCompactTools
	entry := m.entryForModel(m.pendingCompactModel) // 用缓存那段历史的同一模型才命中
	ctxWin := m.models.Pro.ContextWindow
	if ctxWin <= 0 {
		ctxWin = 65536
	}
	return func() tea.Msg {
		summary, cutIdx, compressedTurns, err := runCompression(oldSys, oldTools, snapshot, entry, ctxWin)
		return compressionResultMsg{
			summary:         summary,
			cutIdx:          cutIdx,
			compressedTurns: compressedTurns,
			err:             err,
		}
	}
}
