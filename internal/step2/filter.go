package step2

import (
	"context"
	"math/rand"
	"strings"

	"llm-wiki/pkg/llm"
)

// Filter decides whether a document passes the filter
type Filter struct {
	mode      string
	keyword   KeywordFilter
	llmCfg    LLMJudgmentConfig
	llmClient *llm.Client
}

// KeywordFilter matches documents by tag/keyword
type KeywordFilter struct {
	MatchAny bool
	Tags     []string
}

type LLMJudgmentConfig struct {
	Model         string
	SampleRate    float64
	MinConfidence float64
}

// NewFilter creates a filter from config
func NewFilter(mode string, kw KeywordFilter, llmCfg LLMJudgmentConfig) *Filter {
	return &Filter{mode: mode, keyword: kw, llmCfg: llmCfg}
}

// SetLLMClient injects the LLM client (needed for judgment mode)
func (f *Filter) SetLLMClient(client *llm.Client) {
	f.llmClient = client
}

// Decision represents a filter decision
type Decision int

const (
	Pass Decision = iota
	Reject
	Judging
)

func (d Decision) String() string {
	switch d {
	case Pass:
		return "pass"
	case Reject:
		return "reject"
	case Judging:
		return "judging"
	}
	return "unknown"
}

// Decide returns the filter decision for the given tags
func (f *Filter) Decide(ctx context.Context, tags []string) (Decision, error) {
	// keyword-based pre-filter always runs first
	kwResult := f.keywordDecide(tags)
	if kwResult == Reject {
		return Reject, nil
	}

	if f.mode != "llm_judgment" || f.llmClient == nil {
		return Pass, nil
	}

	return f.judgeByLLM(ctx, tags)
}

// keywordDecide implements keyword-based filtering
func (f *Filter) keywordDecide(tags []string) Decision {
	if len(tags) == 0 {
		return Reject
	}
	matched := 0
	for _, filterTag := range f.keyword.Tags {
		for _, docTag := range tags {
			if strings.Contains(strings.ToLower(docTag), strings.ToLower(filterTag)) {
				if f.keyword.MatchAny {
					return Pass
				}
				matched++
				break
			}
		}
	}
	if f.keyword.MatchAny {
		return Reject
	}
	if matched == len(f.keyword.Tags) {
		return Pass
	}
	return Reject
}

// judgeByLLM implements A (random sampling) + B (confidence gate) for LLM judgment
func (f *Filter) judgeByLLM(ctx context.Context, tags []string) (Decision, error) {
	// A: Random sampling — sample_rate fraction pass without LLM call
	if f.llmCfg.SampleRate > 0 && rand.Float64() < f.llmCfg.SampleRate {
		return Pass, nil
	}

	// B: Confidence gate — keyword matched but compute confidence
	confidence := f.computeConfidence(tags)
	if confidence >= f.llmCfg.MinConfidence {
		return Pass, nil
	}

	// Below threshold — call LLM for judgment
	return f.callLLMJudgment(ctx, tags)
}

func (f *Filter) computeConfidence(tags []string) float64 {
	if len(f.keyword.Tags) == 0 {
		return 0
	}
	matched := 0
	for _, filterTag := range f.keyword.Tags {
		for _, docTag := range tags {
			if strings.Contains(strings.ToLower(docTag), strings.ToLower(filterTag)) {
				matched++
				break
			}
		}
	}
	return float64(matched) / float64(len(f.keyword.Tags))
}

func (f *Filter) callLLMJudgment(ctx context.Context, tags []string) (Decision, error) {
	if f.llmClient == nil {
		return Reject, nil
	}

	prompt := `判断以下文档是否与用户关注的主题相关。
用户关注的主题: ` + strings.Join(f.keyword.Tags, ", ") + `
文档Tags: ` + strings.Join(tags, ", ") + `

如果文档内容与以上任一主题相关，返回 "YES"，否则返回 "NO"。只返回一个词。`

	resp, err := f.llmClient.Complete(ctx, []llm.ChatMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return Reject, err
	}

	resp = strings.TrimSpace(strings.ToUpper(resp))
	if strings.HasPrefix(resp, "YES") {
		return Pass, nil
	}
	return Reject, nil
}

// ExtractTags extracts tags from a markdown document's frontmatter or content
func ExtractTags(content string) []string {
	var tags []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "tags:") {
			tagStr := strings.TrimPrefix(line, "tags:")
			tagStr = strings.Trim(tagStr, " []-")
			for _, t := range strings.Split(tagStr, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tags = append(tags, t)
				}
			}
		}
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "# ") {
			tag := strings.TrimPrefix(line, "#")
			tag = strings.Split(tag, " ")[0]
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// ParseFrontmatter extracts title from markdown frontmatter
func ParseFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "title:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		}
	}
	return ""
}
