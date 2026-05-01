package relay

import "strings"

// ClientInfo 客户端识别结果
type ClientInfo struct {
	Name string // 标准化客户端名称，如 "claude-code", "cline"
}

// clientRule 客户端识别规则
type clientRule struct {
	patterns []string // UA 中包含的模式（小写匹配，使用分隔符边界匹配避免误匹配短词）
	name     string   // 标准化客户端名称
}

// 已确认 UA 格式的客户端排在前面，关键字匹配的排在后面
// 优先级：先匹配专用客户端，再匹配通用 SDK
// 规则说明：
//   - 含分隔符（-、/、空格）的模式用 Contains 匹配，因为分隔符天然形成边界
//   - 纯字母短词（如 amp、crush、zed）用 wordBoundaryContains 匹配，避免误匹配
var clientRules = []clientRule{
	// === 已确认 UA 格式的客户端 ===
	// Claude Code: UA = "Claude-Code" 或 "claude-code/x.x.x"
	{patterns: []string{"claude-code"}, name: "claude-code"},
	// Roo Code: UA = "RooCode/x.x.x" 或 "roo-code"
	{patterns: []string{"roocode", "roo-code"}, name: "roo-code"},
	// Cline (VSCode 扩展 saoudrizwan.claude-dev): UA = "Cline/x.x.x"
	{patterns: []string{"cline/", "claude-dev"}, name: "cline"},
	// Aider: UA 可能包含 "aider"
	{patterns: []string{"aider"}, name: "aider"},
	// OpenAI Codex CLI: UA 可能包含 "codex"
	{patterns: []string{"codex"}, name: "codex"},
	// Continue.dev: 目前 UA = "node-fetch"，但未来可能改为 "continue"
	{patterns: []string{"continue.dev", "continuedev"}, name: "continue"},

	// === 通过关键字匹配的客户端 ===
	// Cursor IDE
	{patterns: []string{"cursor"}, name: "cursor"},
	// Windsurf / Codeium
	{patterns: []string{"windsurf", "codeium"}, name: "windsurf"},
	// GitHub Copilot
	{patterns: []string{"copilot"}, name: "copilot"},
	// Amazon Q Developer
	{patterns: []string{"amazon-q", "q-developer", "amazon q"}, name: "amazon-q"},
	// Augment
	{patterns: []string{"augment"}, name: "augment"},
	// Amp (短词，需边界匹配)
	{patterns: []string{"amp/"}, name: "amp"},
	// auto-coder
	{patterns: []string{"auto-coder", "autocoder"}, name: "auto-coder"},
	// CodeBuddy
	{patterns: []string{"codebuddy", "code-buddy"}, name: "codebuddy"},
	// Codebuff
	{patterns: []string{"codebuff", "code-buff"}, name: "codebuff"},
	// CodeGPT
	{patterns: []string{"codegpt", "code-gpt"}, name: "codegpt"},
	// Crush (短词，需边界匹配)
	{patterns: []string{"crush/"}, name: "crush"},
	// Factory Droid / Factory CLI
	{patterns: []string{"factory-droid", "factory/droid", "factory-cli", "factory/cli"}, name: "factory-droid"},
	// Gemini CLI
	{patterns: []string{"gemini-cli", "gemini/cli"}, name: "gemini-cli"},
	// Gemini Code Assist
	{patterns: []string{"gemini-code-assist", "gemini code assist"}, name: "gemini-code-assist"},
	// Goose (短词，需边界匹配)
	{patterns: []string{"goose/"}, name: "goose"},
	// Jules
	{patterns: []string{"jules"}, name: "jules"},
	// Junie
	{patterns: []string{"junie"}, name: "junie"},
	// Kilo Code
	{patterns: []string{"kilo-code", "kilocode"}, name: "kilo-code"},
	// Kiro (短词，需边界匹配)
	{patterns: []string{"kiro/"}, name: "kiro"},
	// OpenCode
	{patterns: []string{"opencode"}, name: "opencode"},
	// OpenHands
	{patterns: []string{"openhands", "open-hands"}, name: "openhands"},
	// Qoder
	{patterns: []string{"qoder"}, name: "qoder"},
	// Qwen Code
	{patterns: []string{"qwen-code", "qwen code"}, name: "qwen-code"},
	// Replit
	{patterns: []string{"replit"}, name: "replit"},
	// RovoDev
	{patterns: []string{"rovidev", "rovo-dev"}, name: "rovidev"},
	// Tabnine
	{patterns: []string{"tabnine"}, name: "tabnine"},
	// Trae (短词，需边界匹配)
	{patterns: []string{"trae/"}, name: "trae"},
	// Warp (短词，需边界匹配)
	{patterns: []string{"warp/"}, name: "warp"},
	// Zed (短词，需边界匹配)
	{patterns: []string{"zed/"}, name: "zed"},
	// 百度文心快码 (Baidu Comate)
	{patterns: []string{"baidu-comate", "comate/", "文心快码"}, name: "baidu-comate"},
	// 通义灵码 (Tongyi Lingma)
	{patterns: []string{"tongyi-lingma", "lingma/", "灵码"}, name: "tongyi-lingma"},

	// === 通用 SDK（优先级最低，放在最后） ===
	// Anthropic TypeScript SDK
	{patterns: []string{"anthropic-typescript", "anthropic/node"}, name: "anthropic-ts"},
	// OpenAI Python SDK: UA = "OpenAI/Python x.x.x"
	{patterns: []string{"openai/python"}, name: "openai-python"},
	// OpenAI Node SDK: UA = "OpenAI/JS x.x.x"
	{patterns: []string{"openai/js"}, name: "openai-js"},
}

// wordBoundaryChars 用于边界匹配的分隔符集合
// UA 字符串中这些字符被视为单词边界
const wordBoundaryChars = "-/._~+!$&'()*+,;= @\t\n\r"

// isWordBoundary 检查字符是否为单词边界
func isWordBoundary(c byte) bool {
	return c < 'a' || c > 'z' // 非小写字母即为边界（UA 已转为小写）
}

// wordBoundaryContains 检查 pattern 是否在 ua 中以单词边界形式出现
// 即 pattern 前后必须是非字母字符（或字符串边界）
func wordBoundaryContains(ua, pattern string) bool {
	plen := len(pattern)
	start := 0
	for {
		idx := strings.Index(ua[start:], pattern)
		if idx < 0 {
			return false
		}
		idx += start // 转换为 ua 中的绝对位置
		// 检查前边界
		beforeOK := idx == 0 || isWordBoundary(ua[idx-1])
		// 检查后边界
		afterIdx := idx + plen
		afterOK := afterIdx == len(ua) || isWordBoundary(ua[afterIdx])
		if beforeOK && afterOK {
			return true
		}
		start = idx + 1
	}
}

// DetectClient 从 User-Agent 字符串识别客户端
// 返回标准化客户端名称，未识别返回空字符串
func DetectClient(ua string) string {
	if ua == "" {
		return ""
	}
	lowerUA := strings.ToLower(ua)
	for _, rule := range clientRules {
		for _, pattern := range rule.patterns {
			if wordBoundaryContains(lowerUA, pattern) {
				return rule.name
			}
		}
	}
	return ""
}
