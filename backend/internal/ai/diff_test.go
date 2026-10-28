package ai

import "testing"

func TestDiffTextProducesSingleReplacementHunk(t *testing.T) {
	hunks := diffText("负责产品交付", "主导产品交付")

	if len(hunks) != 1 {
		t.Fatalf("diffText returned %d hunks, want 1: %#v", len(hunks), hunks)
	}
	hunk := hunks[0]
	if hunk.Original != "负责" || hunk.Revised != "主导" {
		t.Fatalf("hunk = %q -> %q, want 负责 -> 主导", hunk.Original, hunk.Revised)
	}
	if hunk.OriginalStart != 0 || hunk.OriginalEnd != 2 {
		t.Fatalf("hunk offsets = [%d:%d], want [0:2]", hunk.OriginalStart, hunk.OriginalEnd)
	}
}

func TestDiffTextCollapsesFullRewriteIntoOneHunk(t *testing.T) {
	hunks := diffText("old", "valid single-call optimization")

	if len(hunks) != 1 {
		t.Fatalf("diffText returned %d hunks, want 1: %#v", len(hunks), hunks)
	}
	if hunks[0].Original != "old" || hunks[0].Revised != "valid single-call optimization" {
		t.Fatalf("hunk = %q -> %q, want the whole text replaced", hunks[0].Original, hunks[0].Revised)
	}
}

func TestDiffTextKeepsSeparateHunksForDistantChanges(t *testing.T) {
	original := "- 负责交付，覆盖核心链路并支撑日常运营\n- 负责流程建设，同时沉淀方法论"
	revised := "- 主导交付，覆盖核心链路并支撑日常运营\n- 负责流程建设，同时沉淀方法论"

	hunks := diffText(original, revised)

	if len(hunks) != 1 {
		t.Fatalf("diffText returned %d hunks, want 1: %#v", len(hunks), hunks)
	}
	if hunks[0].Original != "负责" {
		t.Fatalf("hunk original = %q, want 负责", hunks[0].Original)
	}
}

func TestDiffTextReportsPureInsertion(t *testing.T) {
	hunks := diffText("负责产品交付", "负责产品交付并新增了服务")

	if len(hunks) != 1 {
		t.Fatalf("diffText returned %d hunks, want 1: %#v", len(hunks), hunks)
	}
	if !hunks[0].isInsertion() {
		t.Fatalf("hunk = %#v, want a pure insertion", hunks[0])
	}
}

func TestDiffTextUsesRuneOffsets(t *testing.T) {
	hunks := diffText("负责🚀交付", "主导🚀交付")

	if len(hunks) != 1 {
		t.Fatalf("diffText returned %d hunks, want 1: %#v", len(hunks), hunks)
	}
	if hunks[0].OriginalStart != 0 || hunks[0].OriginalEnd != 2 {
		t.Fatalf("hunk offsets = [%d:%d], want [0:2] in runes", hunks[0].OriginalStart, hunks[0].OriginalEnd)
	}
	if hunks[0].Original != "负责" {
		t.Fatalf("hunk original = %q, want 负责", hunks[0].Original)
	}
}

func TestDiffTextReturnsNoHunksForIdenticalText(t *testing.T) {
	hunks := diffText("负责产品交付", "负责产品交付")

	if len(hunks) != 0 {
		t.Fatalf("diffText returned %d hunks, want 0: %#v", len(hunks), hunks)
	}
}
