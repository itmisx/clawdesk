package agent

// events
const (
	LLMStart    = "llm:start"
	LLMToken    = "llm:token"
	LLMDone     = "llm:done"
	LLMError    = "llm:error"
	LLMThinking = "llm:thinking"
	LLMToolCall = "llm:toolcall" // 工具调用通知
	LLMTrace    = "llm:trace"    // 执行追踪完成（前端可查看）
)
