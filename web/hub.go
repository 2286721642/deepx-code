package web

import (
	"strconv"
	"sync"
)

// Message 是聊天窗口里的一条消息(只在左栏渲染)。role: "user" | "assistant"。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCallView 是右栏实时工具调用列表的一项。
type ToolCallView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Args   string `json:"args"`
	Status string `json:"status"` // running | done | failed
	Output string `json:"output,omitempty"`
}

// ModelsInfo 右栏顶部展示的模型信息。
type ModelsInfo struct {
	Flash      string `json:"flash"`
	Pro        string `json:"pro"`
	ActiveRole string `json:"activeRole"` // "flash" | "pro"
}

// ReviewInfo 待人工确认的工具调用(review 模式)。nil = 当前没有待确认。
type ReviewInfo struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

// Snapshot 是 web dashboard 的完整状态快照,新连接的浏览器先收到它,再收实时增量。
type Snapshot struct {
	Messages      []Message      `json:"messages"`
	Plan          []PlanNode     `json:"plan"`
	ToolCalls     []ToolCallView `json:"toolCalls"`
	Usage         *Usage         `json:"usage"`
	Streaming     bool           `json:"streaming"`
	Models        ModelsInfo     `json:"models"`
	Workspace     string         `json:"workspace"`
	Lang          string         `json:"lang"` // "zh" | "en",跟 TUI 同步
	ReviewPending *ReviewInfo    `json:"reviewPending"`
}

const clientBufferSize = 512

// Hub 是 SSE 广播中心:维护一份服务端会话快照(供新连接初始化),
// 并把实时增量事件 fan-out 给所有已连接的浏览器。
//
// 所有有状态逻辑(开/合 assistant 气泡、工具调用配对、plan 更新)集中在 apply 里,
// 浏览器端用同样的 reducer 处理增量,保证两边一致。
type Hub struct {
	mu      sync.Mutex
	clients map[chan Event]struct{}
	snap    Snapshot

	openAssistant int // 当前正在流式的 assistant 消息下标;-1 表示没有
	toolSeq       int // 工具调用 ID 自增
}

// NewHub 创建 Hub。flashModel/proModel/workspace/lang 是初始展示信息(lang 后续可由 lang 事件更新)。
func NewHub(flashModel, proModel, workspace, lang string) *Hub {
	return &Hub{
		clients: make(map[chan Event]struct{}),
		snap: Snapshot{
			Messages:  []Message{},
			Plan:      []PlanNode{},
			ToolCalls: []ToolCallView{},
			Models:    ModelsInfo{Flash: flashModel, Pro: proModel, ActiveRole: "flash"},
			Workspace: workspace,
			Lang:      lang,
		},
		openAssistant: -1,
	}
}

// Broadcast 把一个事件应用到快照并 fan-out 给所有客户端。
// apply 可能会丰富事件(如给工具调用分配 ID),fan-out 的是丰富后的版本,确保前后端一致。
func (h *Hub) Broadcast(ev Event) {
	if h == nil {
		return
	}
	h.mu.Lock()
	enriched := h.apply(ev)
	for ch := range h.clients {
		select {
		case ch <- enriched:
		default:
			// 客户端缓冲满(慢消费者)→ 关闭它,浏览器 EventSource 会自动重连并重新拉快照。
			close(ch)
			delete(h.clients, ch)
		}
	}
	h.mu.Unlock()
}

// Subscribe 注册一个新客户端,返回其事件 channel、当前快照、以及注销函数。
// 调用方应先把 snapshot 发给浏览器,再从 channel 持续读增量。
func (h *Hub) Subscribe() (<-chan Event, Snapshot, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan Event, clientBufferSize)
	h.clients[ch] = struct{}{}
	snap := h.copySnapshotLocked()
	unsub := func() {
		h.mu.Lock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, snap, unsub
}

// SnapshotCopy 返回当前快照的拷贝(给 GET /api/state)。
func (h *Hub) SnapshotCopy() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.copySnapshotLocked()
}

func (h *Hub) copySnapshotLocked() Snapshot {
	s := h.snap
	s.Messages = append([]Message(nil), h.snap.Messages...)
	s.Plan = append([]PlanNode(nil), h.snap.Plan...)
	s.ToolCalls = append([]ToolCallView(nil), h.snap.ToolCalls...)
	if h.snap.Usage != nil {
		u := *h.snap.Usage
		s.Usage = &u
	}
	if h.snap.ReviewPending != nil {
		r := *h.snap.ReviewPending
		s.ReviewPending = &r
	}
	return s
}

// apply 把事件更新到快照,返回(可能被丰富过的)事件用于 fan-out。必须在持锁状态下调用。
func (h *Hub) apply(ev Event) Event {
	switch ev.Kind {
	case "user_message":
		// 新回合开始:落一条 user 消息,清空上一轮的 plan / 工具列表,重置 assistant 气泡。
		h.snap.Messages = append(h.snap.Messages, Message{Role: "user", Content: ev.Text})
		h.snap.Plan = []PlanNode{}
		h.snap.ToolCalls = []ToolCallView{}
		h.snap.Usage = nil
		h.snap.ReviewPending = nil
		h.snap.Streaming = true
		h.openAssistant = -1

	case "token":
		if h.openAssistant < 0 {
			h.snap.Messages = append(h.snap.Messages, Message{Role: "assistant"})
			h.openAssistant = len(h.snap.Messages) - 1
		}
		h.snap.Messages[h.openAssistant].Content += ev.Text

	case "reasoning_token":
		// 思考过程不进聊天气泡;前端可用它显示 "thinking…"。快照不存。

	case "tool_call":
		h.toolSeq++
		id := strconv.Itoa(h.toolSeq)
		ev.ID = id
		h.snap.ToolCalls = append(h.snap.ToolCalls, ToolCallView{
			ID: id, Name: ev.Name, Args: ev.Args, Status: "running",
		})

	case "tool_result":
		// 配对到最近一个同名 running 工具。
		for i := len(h.snap.ToolCalls) - 1; i >= 0; i-- {
			tc := &h.snap.ToolCalls[i]
			if tc.Name == ev.Name && tc.Status == "running" {
				if ev.Success != nil && *ev.Success {
					tc.Status = "done"
				} else {
					tc.Status = "failed"
				}
				tc.Output = ev.Output
				ev.ID = tc.ID
				break
			}
		}

	case "model_switch":
		if ev.Role != "" {
			h.snap.Models.ActiveRole = ev.Role
		}

	case "plan":
		h.snap.Plan = append([]PlanNode(nil), ev.Plan...)

	case "plan_status":
		for i := range h.snap.Plan {
			if h.snap.Plan[i].ID == ev.ID {
				if ev.Status != "" {
					h.snap.Plan[i].Status = ev.Status
				}
				if ev.Summary != "" {
					h.snap.Plan[i].Summary = ev.Summary
				}
				break
			}
		}

	case "usage":
		h.snap.Usage = ev.Usage

	case "done":
		h.snap.Streaming = false
		h.openAssistant = -1

	case "error":
		h.snap.Streaming = false
		h.openAssistant = -1

	case "review_request":
		h.snap.ReviewPending = &ReviewInfo{Name: ev.Name, Args: ev.Args}

	case "review_resolved":
		h.snap.ReviewPending = nil

	case "lang":
		if ev.Text != "" {
			h.snap.Lang = ev.Text
		}
	}
	return ev
}
