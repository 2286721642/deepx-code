package agent

import "testing"

func TestUsageInfoNormalize(t *testing.T) {
	// mimo / OpenAI 标准:命中在 prompt_tokens_details.cached_tokens,需回填。
	u := UsageInfo{PromptTokens: 1000}
	u.PromptTokensDetails.CachedTokens = 800
	u.normalize()
	if u.PromptCacheHitTokens != 800 {
		t.Errorf("hit = %d, want 800 (回填自 cached_tokens)", u.PromptCacheHitTokens)
	}
	if u.PromptCacheMissTokens != 200 {
		t.Errorf("miss = %d, want 200 (prompt - cached)", u.PromptCacheMissTokens)
	}

	// DeepSeek:已有专有字段,normalize 不覆盖。
	d := UsageInfo{PromptTokens: 1000, PromptCacheHitTokens: 700, PromptCacheMissTokens: 300}
	d.normalize()
	if d.PromptCacheHitTokens != 700 || d.PromptCacheMissTokens != 300 {
		t.Errorf("deepseek 字段被改动: hit=%d miss=%d, want 700/300", d.PromptCacheHitTokens, d.PromptCacheMissTokens)
	}

	// 无缓存:全 0,保持 0(不产生伪命中)。
	z := UsageInfo{PromptTokens: 500}
	z.normalize()
	if z.PromptCacheHitTokens != 0 {
		t.Errorf("无缓存时 hit = %d, want 0", z.PromptCacheHitTokens)
	}
}
