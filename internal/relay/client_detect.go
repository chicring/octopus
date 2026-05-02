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
// 所有匹配均使用 wordBoundaryContains，确保前后为非字母字符或字符串边界
// UA 格式通常为 "品牌名/版本号"，如 "cline/3.5.0"、"roo-cline/3.17.5"
var clientRules = []clientRule{
	// === 已确认 UA 格式的客户端 ===
	// Claude Code: UA = "claude-code/x.x.x" 或 "claude-cli/x.x.x"
	{patterns: []string{"claude-code", "claude-cli"}, name: "claude-code"},
	// Roo Code / Roo Cline: UA = "RooCode/x.x.x" 或 "roo-cline/x.x.x" 或 "roo-code/x.x.x"
	{patterns: []string{"roocode", "roo-cline", "roo-code"}, name: "roo-code"},
	// Cline: UA = "Cline/x.x.x"
	{patterns: []string{"cline"}, name: "cline"},
	// Aider: UA = "aider/x.x.x"
	{patterns: []string{"aider"}, name: "aider"},
	// OpenAI Codex CLI: UA = "codex/x.x.x"
	{patterns: []string{"codex"}, name: "codex"},
	// Continue.dev
	{patterns: []string{"continue.dev", "continuedev"}, name: "continue"},

	// === 通过品牌名匹配的客户端 ===
	// Cursor IDE: UA = "Cursor/x.x.x"
	{patterns: []string{"cursor"}, name: "cursor"},
	// Windsurf / Codeium: UA = "Windsurf/x.x.x" 或 "Codeium/x.x.x"
	{patterns: []string{"windsurf", "codeium"}, name: "windsurf"},
	// GitHub Copilot: UA = "copilot/x.x.x" 或 "CopilotAgent/x.x.x"
	{patterns: []string{"copilot"}, name: "copilot"},
	// Amazon Q Developer: UA = "amazon-q/x.x.x" 或 "q-developer/x.x.x"
	{patterns: []string{"amazon-q", "q-developer"}, name: "amazon-q"},
	// Augment: UA = "augment/x.x.x"
	{patterns: []string{"augment"}, name: "augment"},
	// Amp: UA = "amp/x.x.x" (短词，边界匹配防误匹配)
	{patterns: []string{"amp"}, name: "amp"},
	// auto-coder: UA = "auto-coder/x.x.x"
	{patterns: []string{"auto-coder", "autocoder"}, name: "auto-coder"},
	// CodeBuddy
	{patterns: []string{"codebuddy", "code-buddy"}, name: "codebuddy"},
	// Codebuff
	{patterns: []string{"codebuff", "code-buff"}, name: "codebuff"},
	// CodeGPT
	{patterns: []string{"codegpt", "code-gpt"}, name: "codegpt"},
	// Crush (短词，边界匹配)
	{patterns: []string{"crush"}, name: "crush"},
	// Factory Droid / Factory CLI: UA = "factory-cli/x.x.x" 或 "factory/droid"
	{patterns: []string{"factory-droid", "factory-cli", "factory/droid"}, name: "factory-droid"},
	// Gemini CLI: UA = "gemini-cli/x.x.x" 或 "gemini/cli"
	{patterns: []string{"gemini-cli", "gemini/cli"}, name: "gemini-cli"},
	// Gemini Code Assist
	{patterns: []string{"gemini-code-assist"}, name: "gemini-code-assist"},
	// Goose (短词，边界匹配): UA = "goose/x.x.x"
	{patterns: []string{"goose"}, name: "goose"},
	// Jules: UA = "jules/x.x.x"
	{patterns: []string{"jules"}, name: "jules"},
	// Junie: UA = "junie/x.x.x"
	{patterns: []string{"junie"}, name: "junie"},
	// Kilo Code: UA = "kilocode/x.x.x"
	{patterns: []string{"kilo-code", "kilocode"}, name: "kilo-code"},
	// Kiro (短词，边界匹配): UA = "kiro/x.x.x"
	{patterns: []string{"kiro"}, name: "kiro"},
	// OpenCode: UA = "opencode/x.x.x"
	{patterns: []string{"opencode"}, name: "opencode"},
	// OpenHands: UA = "openhands/x.x.x"
	{patterns: []string{"openhands", "open-hands"}, name: "openhands"},
	// Qoder: UA = "qoder/x.x.x"
	{patterns: []string{"qoder"}, name: "qoder"},
	// Qwen Code: UA = "qwen-code/x.x.x"
	{patterns: []string{"qwen-code"}, name: "qwen-code"},
	// Replit: UA = "replit/x.x.x"
	{patterns: []string{"replit"}, name: "replit"},
	// RovoDev
	{patterns: []string{"rovidev", "rovo-dev"}, name: "rovidev"},
	// Tabnine: UA = "tabnine/x.x.x"
	{patterns: []string{"tabnine"}, name: "tabnine"},
	// Trae (短词，边界匹配): UA = "trae/x.x.x"
	{patterns: []string{"trae"}, name: "trae"},
	// Warp (短词，边界匹配): UA = "warp/x.x.x"
	{patterns: []string{"warp"}, name: "warp"},
	// Zed (短词，边界匹配): UA = "zed/x.x.x"
	{patterns: []string{"zed"}, name: "zed"},
	// 百度文心快码 (Baidu Comate): UA = "comate/x.x.x"
	{patterns: []string{"baidu-comate", "comate"}, name: "baidu-comate"},
	// 通义灵码 (Tongyi Lingma): UA = "lingma/x.x.x"
	{patterns: []string{"tongyi-lingma", "lingma"}, name: "tongyi-lingma"},

	// === 通用 SDK（优先级最低，放在最后） ===
	// Anthropic TypeScript SDK: UA = "anthropic-typescript/x.x.x"
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
