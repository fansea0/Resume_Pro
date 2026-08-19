package prompts

import (
	"strings"
	"testing"
	"text/template"
)

func TestEmbeddedPromptTemplatesContainRequiredGuidance(t *testing.T) {
	templates := map[string][]string{
		"system_common.tmpl": {
			"不得编造事实、数字、日期、公司名、学校名、项目名。",
			"不得删除用户已有的关键事实，只能改写表达。",
			"用户没写的数字不要替用户填",
			"严格按返回的 JSON schema 输出，不要有多余字段、不要 markdown 包裹、不要解释文字。",
		},
		"rewriting_strategy.tmpl": {"用户没写的数字不要编", "不编事实的前提下"},
		"system_generate.tmpl":    {"不得编造数字、日期、雇主名。", "所有 section 按 JD 侧重点排序。"},
		"section_summary.tmpl":    {"60-150 字", "定位 + 核心优势 + 目标岗位匹配"},
		"section_work.tmpl":       {"{{.Title}}", "工作/实习经历", "不可改字段请原样保留", "动作 + 对象 + 结果"},
		"section_project.tmpl":    {"不可改字段请原样保留", "技术选型理由 / 量化结果 / 个人贡献"},
		"target_job_context.tmpl": {"{{- if .Company }}", "{{- if .Position }}", "{{.Requirements}}", "不要把 JD 内容写进简历原文。"},
	}

	for name, requiredSnippets := range templates {
		t.Run(name, func(t *testing.T) {
			content := MustRead(name)
			for _, snippet := range requiredSnippets {
				if !strings.Contains(content, snippet) {
					t.Fatalf("%s does not contain required guidance %q", name, snippet)
				}
			}
			if _, err := template.New(name).Parse(content); err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
		})
	}
}

func TestTargetJobContextConditionallyRendersOptionalFields(t *testing.T) {
	templateContent := template.Must(template.New("target_job_context.tmpl").Parse(MustRead("target_job_context.tmpl")))
	var output strings.Builder
	if err := templateContent.Execute(&output, struct {
		Company      string
		Position     string
		Requirements string
	}{Requirements: "Go 服务端开发"}); err != nil {
		t.Fatal(err)
	}

	got := output.String()
	if strings.Contains(got, "公司：") || strings.Contains(got, "职位：") {
		t.Fatalf("optional fields rendered without values: %q", got)
	}
	if !strings.Contains(got, "岗位要求（JD）：\nGo 服务端开发") {
		t.Fatalf("requirements missing from target job context: %q", got)
	}
}

func TestMustReadPanicsForMissingTemplate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRead did not panic for a missing template")
		}
	}()
	MustRead("missing.tmpl")
}
