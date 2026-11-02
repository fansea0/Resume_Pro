package interview

import "testing"

func TestComputeOverallGrade(t *testing.T) {
	cases := []struct {
		name   string
		grades []Grade
		want   Grade
	}{
		{"3S9B -> B", []Grade{GradeS, GradeS, GradeS, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeB}, GradeB},
		{"5S7A -> A", []Grade{GradeS, GradeS, GradeS, GradeS, GradeS, GradeA, GradeA, GradeA, GradeA, GradeA, GradeA, GradeA}, GradeA},
		{"8S4A -> S", []Grade{GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeA, GradeA, GradeA, GradeA}, GradeS},
		{"6S6A -> A (even median takes lower)", []Grade{GradeS, GradeS, GradeS, GradeS, GradeS, GradeS, GradeA, GradeA, GradeA, GradeA, GradeA, GradeA}, GradeA},
		{"all A -> A", []Grade{GradeA, GradeA, GradeA, GradeA, GradeA, GradeA}, GradeA},
		{"3S9B+ -> B+", []Grade{GradeS, GradeS, GradeS, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus}, GradeBPlus},
		{"single B", []Grade{GradeB}, GradeB},
		{"empty -> C", nil, GradeC},
		// 偶数 N=12，下中位 orders[(12-1)/2]=orders[5]=B → 基础档 B；
		// bPlusOrAbove=6，6*2>=12 命中 B+ 档比例门槛，故总评为 B+（spec《评分与总评算法》比例门槛表）。
		{"6B6B+ -> B+ (even lower median base B, B+ tier reached by half)", []Grade{GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus, GradeBPlus}, GradeBPlus},
		// 偶数 N=12，中间两档跨 C|B：下中位 orders[5]=C → 基础档 C；
		// 上中位 orders[6]=B 会得到 B，本用例真正钉住「偶数取下中位」。
		{"6B6C -> C (even median takes lower, straddles C|B)", []Grade{GradeB, GradeB, GradeB, GradeB, GradeB, GradeB, GradeC, GradeC, GradeC, GradeC, GradeC, GradeC}, GradeC},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeOverallGrade(tc.grades); got != tc.want {
				t.Errorf("ComputeOverallGrade(%v) = %v, want %v", tc.grades, got, tc.want)
			}
		})
	}
}

func TestClampGrade(t *testing.T) {
	if got := ClampGrade(GradeB, GradeBPlus); got != GradeBPlus {
		t.Errorf("ClampGrade(B, B+) = %v, want B+", got)
	}
	if got := ClampGrade(GradeA, GradeB); got != GradeA {
		t.Errorf("ClampGrade(A, B) = %v, want A", got)
	}
	if got := ClampGrade(GradeC, GradeC); got != GradeC {
		t.Errorf("ClampGrade(C, C) = %v, want C", got)
	}
}

// TestMedianOrder 直接钉住中位数下标：奇数取正中间，偶数取下中位（orders[N/2-1]）。
// 直接测内部辅助函数，避免 ComputeOverallGrade 的比例门槛把中位数差异掩盖掉。
func TestMedianOrder(t *testing.T) {
	cases := []struct {
		name   string
		orders []int
		want   int
	}{
		{"odd 13 -> exact middle index 6", []int{0, 1, 1, 2, 2, 2, 2, 3, 3, 4, 4, 5, 5}, 2},
		{"even 12 -> lower middle index 5", []int{0, 0, 0, 0, 0, 0, 2, 2, 2, 2, 2, 2}, 0},
		{"even 2 -> lower middle index 0", []int{2, 5}, 2},
		{"single -> index 0", []int{3}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := medianOrder(tc.orders); got != tc.want {
				t.Errorf("medianOrder(%v) = %v, want %v", tc.orders, got, tc.want)
			}
		})
	}
}

func TestGradeOrderAndValidGrade(t *testing.T) {
	wantOrder := map[Grade]int{
		GradeC:      0,
		GradeBMinus: 1,
		GradeB:      2,
		GradeBPlus:  3,
		GradeA:      4,
		GradeS:      5,
	}
	for g, want := range wantOrder {
		if got := GradeOrder(g); got != want {
			t.Errorf("GradeOrder(%v) = %d, want %d", g, got, want)
		}
		if got := GradeFromOrder(want); got != g {
			t.Errorf("GradeFromOrder(%d) = %v, want %v", want, got, g)
		}
		if !ValidGrade(string(g)) {
			t.Errorf("ValidGrade(%q) = false, want true", string(g))
		}
	}
	if ValidGrade("X") {
		t.Error("ValidGrade(\"X\") = true, want false")
	}
	if ValidGrade("") {
		t.Error("ValidGrade(\"\") = true, want false")
	}
	if GradeOrder("X") != 0 {
		t.Errorf("GradeOrder(\"X\") = %d, want 0", GradeOrder("X"))
	}
}

func TestIsGradeAtLeast(t *testing.T) {
	if !IsGradeAtLeast(GradeS, GradeA) {
		t.Error("IsGradeAtLeast(S, A) = false, want true")
	}
	if IsGradeAtLeast(GradeB, GradeBPlus) {
		t.Error("IsGradeAtLeast(B, B+) = true, want false")
	}
	if !IsGradeAtLeast(GradeBPlus, GradeBPlus) {
		t.Error("IsGradeAtLeast(B+, B+) = false, want true")
	}
}

func TestInterviewConfigResolved(t *testing.T) {
	if got := DefaultInterviewConfig.Resolved().FollowUpBudget; got != 6 {
		t.Errorf("DefaultInterviewConfig.Resolved().FollowUpBudget = %d, want 6", got)
	}
	if got := (InterviewConfig{QuestionCount: 5, FollowUpBudget: -1}).Resolved().FollowUpBudget; got != 2 {
		t.Errorf("Resolved() FollowUpBudget = %d, want 2", got)
	}
	if got := (InterviewConfig{QuestionCount: 5, FollowUpBudget: 3}).Resolved().FollowUpBudget; got != 3 {
		t.Errorf("Resolved() FollowUpBudget = %d, want 3", got)
	}
	// Resolved 不得改动全局默认配置
	if DefaultInterviewConfig.FollowUpBudget != -1 {
		t.Errorf("DefaultInterviewConfig.FollowUpBudget mutated to %d, want -1", DefaultInterviewConfig.FollowUpBudget)
	}
}
